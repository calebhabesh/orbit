#!/usr/bin/env python3
"""Linux PTY campaign for the real T09 binary; only fresh marked child state.

Run: make test-terminal-pty
Optional evidence: --output /new/empty/directory
"""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pty
import re
import select
import shutil
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time
import uuid

sys.path.insert(0, str(Path(__file__).resolve().parent / "validation"))
from host_agent import beneath, identity, validated_root
from terminal_vt import Screen

# Hermetic: never select the packaged hosted profile, so no daemon started here
# contacts the operated service (W14). Child processes inherit this.
os.environ.setdefault('ORBIT_DISABLE_PACKAGED_PROFILE', '1')



def orbit_argv(args):
    """Map pre-2.0 'filesync' argv onto the single orbit binary."""
    args = list(args)
    if args and args[0] == "orbit":
        return args[1:]
    return ["engine", *args]


class Campaign:
    def __init__(self, root, binary, output, bare=False):
        self.bare = bare
        self.root = root.resolve()
        self.token = uuid.uuid4().hex
        marker = root / ".orbit-disposable"
        marker.write_text(self.token)
        marker.chmod(0o600)
        self.binary = beneath(root, "orbit")
        if Path(binary).resolve() != self.binary.resolve():
            shutil.copyfile(binary, self.binary)
        self.binary.chmod(0o700)
        self.state = beneath(root, "state")
        self.data = beneath(root, "Notes界")
        self.data.mkdir(mode=0o700)
        self.children = {}
        self.output = output
        self.results = []
        self.raw = {}

    def checked(self):
        return validated_root({"root": str(self.root), "token": self.token})

    def run(self, *args):
        self.checked()
        proc = subprocess.run([str(self.binary), *orbit_argv(args)], stdin=subprocess.DEVNULL,
                              capture_output=True, timeout=15, cwd=self.data)
        if proc.returncode:
            raise AssertionError(f"CLI {args[0]} failed ({proc.returncode}): " + (proc.stdout+proc.stderr).decode(errors="replace"))
        return proc.stdout

    def spawn(self, args, **kwargs):
        self.checked()
        p = subprocess.Popen([str(self.binary), *orbit_argv(args)], **kwargs)
        ticks, _ = identity(p.pid)
        self.children[p.pid] = (p, ticks)
        return p

    def send_signal(self, p, sig):
        self.checked()
        if p.poll() is not None:
            return
        ticks, _ = identity(p.pid)
        assert ticks == self.children[p.pid][1], "changed process birth identity"
        argv = Path(f"/proc/{p.pid}/cmdline").read_bytes().split(b"\0")
        assert os.fsencode(self.binary) in argv and os.fsencode(self.state) in argv, "changed child command/state"
        assert Path(f"/proc/{p.pid}/exe").resolve() == self.binary, "changed child executable"
        beneath(self.checked(), "state")
        p.send_signal(sig)

    def start_daemon(self):
        p = self.spawn(["serve", "--state", str(self.state), "--peer-listen", "127.0.0.1:0",
                        "--control-listen", "127.0.0.1:0", "--sync-interval", "1s", "--no-watch"],
                       stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if p.poll() is not None:
                raise AssertionError("daemon exited before readiness")
            if (self.state / "control.addr").exists():
                status = json.loads(self.run("orbit", "status", "--state", str(self.state), "--json"))
                if status["service"]["running"]:
                    return p
            time.sleep(0.05)
        raise AssertionError("daemon readiness timeout")

    def record(self, name, transcript, assertions):
        # Remove ANSI/OSC queries and render instructions; fixture paths only.
        clean = re.sub(rb"\x1b\][^\x07]*(?:\x07|\x1b\\)", b"", transcript)
        clean = re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", clean)
        clean = clean.decode("utf-8", errors="replace").replace(str(self.root), "<disposable>")
        clean = "".join(c if c in "\n\r\t" or ord(c) >= 32 else f"\\x{ord(c):02x}" for c in clean)
        self.raw[name] = clean[-65536:]
        self.results.append({"scenario": name, "assertions": assertions, "result": "passed"})

    def session(self, name, daemon, exit_key=b"q", size=(80,24), tool_mode=None, loss=False):
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", size[1], size[0], 0, 0))
        before = copy.deepcopy(termios.tcgetattr(slave))
        args = ["orbit", *([] if self.bare else ["tui"]), "--state", str(self.state), "--no-color"]
        scratch = beneath(self.root, "tool result.txt")
        if tool_mode:
            scratch.write_text("before tool")
            helper = beneath(self.root, "tool.py")
            helper.write_text("""import pathlib,sys,termios
a=termios.tcgetattr(0)
assert a[3]&termios.ICANON and a[3]&termios.ECHO
print('TOOL_READY',flush=True)
answer=input()
if sys.argv[2]=='fail': sys.exit(7)
pathlib.Path(sys.argv[1]).write_text('tool edited: '+answer)
print('TOOL_DONE',flush=True)
""")
            args += ["--tool", f"python3 '{helper}'", "--tool-file", str(scratch)]
            # The file path always arrives as the adapter's last argument.
            helper.write_text(helper.read_text().replace("sys.argv[2]=='fail'", repr(tool_mode)+"=='fail'"))
        env = dict(os.environ, TERM="xterm-256color", NO_COLOR="1")
        p = self.spawn(args, stdin=slave, stdout=slave, stderr=slave, env=env)
        transcript = bytearray()
        screen = Screen(*size)

        def read_until(needle, timeout=10):
            start = len(transcript)
            deadline = time.monotonic()+timeout
            while time.monotonic()<deadline:
                if needle in transcript[start:] or (len(transcript)>start and needle.decode() in screen.text()):
                    return
                if select.select([master], [], [], 0.05)[0]:
                    try:
                        data = os.read(master, 65536)
                    except OSError:
                        data = b""
                    transcript.extend(data)
                    screen.feed(data)
                    assert len(transcript)<2<<20, "unbounded terminal output"
                if p.poll() is not None:
                    break
            # A render may retain part of a prior frame; include earlier data only
            # for the initial screen, never for transition assertions.
            raise AssertionError(f"{name}: missing {needle!r}; tail={bytes(transcript[-1500:])!r}")

        def wait_exit():
            # Drain output during shutdown as a terminal emulator would.
            deadline = time.monotonic()+10
            while p.poll() is None and time.monotonic()<deadline:
                if select.select([master],[],[],0.05)[0]:
                    data = os.read(master,65536)
                    transcript.extend(data)
                    screen.feed(data)
            if p.poll() is None:
                # Capture a real Go stack on a validated child, preserving the
                # failure instead of leaving an orphan after a timeout.
                self.send_signal(p, signal.SIGQUIT)
                deadline = time.monotonic()+3
                while time.monotonic()<deadline:
                    if select.select([master],[],[],0.05)[0]:
                        transcript.extend(os.read(master,65536))
                    if p.poll() is not None:
                        break
                if p.poll() is None:
                    self.send_signal(p, signal.SIGKILL)
                p.wait(timeout=5)
                raise AssertionError(f"{name}: shutdown hung; stack={bytes(transcript[-24000:]).decode(errors='replace')}")
            return p.wait()

        def read_visible(needle, timeout=10):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if needle in screen.text():
                    return
                if select.select([master], [], [], 0.05)[0]:
                    data = os.read(master, 65536)
                    transcript.extend(data)
                    screen.feed(data)
                    assert len(transcript) < 2 << 20, "unbounded terminal output"
                if p.poll() is not None:
                    break
            raise AssertionError(f"{name}: visible {needle!r} absent; frame={screen.text()!r}")

        try:
            read_until("Notes界".encode())
            assert b"\x1b[?1049h" in transcript, "not an actual alternate-screen client"
            assert b"\x1b[?2004h" in transcript, "paste mode not enabled"
            assert not re.search(rb"\x1b\[[0-9;]*m", transcript), "SGR leaked into colorless output"
            os.write(master, b"?")
            read_until(b"Keyboard help")
            os.write(master, b"\x1b")
            time.sleep(0.12)
            os.write(master, b"/\x1b[200~jkq?\xe7\x95\x8c\x1b[201~")
            read_until("jkq?界".encode())
            assert p.poll() is None, "q in search quit the client"
            os.write(master, b"\x1b")
            time.sleep(0.12)
            os.write(master, b"\x1b")  # Clear filter in navigation.
            time.sleep(0.12)
            os.write(master, b"f")
            read_until(b"[Folders]" if size[0]>=60 else b"Folders |")
            os.write(master, b"\x1b[B\x1b[A\t")
            read_until(b"Type to filter")
            os.write(master, b"\r")
            read_visible("> Notes界")
            os.write(master, b"\r")
            read_until(b"Inspect")
            os.write(master, b"\x1b")
            time.sleep(0.12)
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 16, 40, 0, 0))
            screen.resize(40,16)
            self.send_signal(p, signal.SIGWINCH)
            read_until(b"Folders |")
            if tool_mode:
                os.write(master, b"e")
                read_until(b"TOOL_READY")
                during = termios.tcgetattr(slave)
                assert during[3]&termios.ICANON and during[3]&termios.ECHO, "tool did not receive canonical terminal"
                if tool_mode=="cancel":
                    self.send_signal(p, signal.SIGTERM)
                    wait_exit()
                    assert scratch.read_text()=="before tool", "canceled tool changed result"
                else:
                    os.write(master, b"ordinary editor input\n")
                    read_until(b"Tool failed" if tool_mode=="fail" else b"Tool returned")
                    if tool_mode!="fail":
                        assert scratch.read_text()=="tool edited: ordinary editor input", "tool bytes not edited"
                    else:
                        assert scratch.read_text()=="before tool", "failed tool changed result"
                    resumed = termios.tcgetattr(slave)
                    assert not resumed[3]&termios.ICANON, "client did not reacquire raw input"
            if loss:
                # Keep the lock held but make owner HTTP unavailable. This proves
                # a failed live request cannot fall back to direct SQLite ownership.
                self.send_signal(daemon, signal.SIGSTOP)
                try:
                    os.write(master, b"r")
                    read_until(b"Control unavailable", timeout=12)
                finally:
                    self.send_signal(daemon, signal.SIGCONT)
                os.write(master, b"r")
                read_until(b"Notes", timeout=10)
                deadline = time.monotonic()+10
                while "Control unavailable" in screen.text() and time.monotonic()<deadline:
                    if select.select([master],[],[],0.1)[0]:
                        data = os.read(master,65536)
                        transcript.extend(data)
                        screen.feed(data)
                assert "Control unavailable" not in screen.text(), "reconnected response did not replace error"
            if exit_key==b"SIGTERM":
                self.send_signal(p, signal.SIGTERM)
            else:
                os.write(master, exit_key)
            wait_exit()
            # Drain final terminal restoration sequences.
            while select.select([master],[],[],0.1)[0]:
                transcript.extend(os.read(master,65536))
            after = termios.tcgetattr(slave)
            assert after==before, "terminal flags/control chars were not restored exactly"
            assert b"\x1b[?1049l" in transcript, "alternate screen was not restored"
            assert b"\x1b[?2004l" in transcript, "paste mode was not restored"
            expected = (0,1) if exit_key==b"SIGTERM" else (0,)
            assert p.returncode in expected, "unexpected client exit"
            assert daemon.poll() is None, "client exit stopped daemon"
            self.record(name, bytes(transcript), ["keyboard/search/paste/resize", "no SGR", "exact termios restoration", "daemon alive", *( ["direct argv tool canonical/raw handoff and actual bytes"] if tool_mode else []), *( ["locked unavailable daemon, retry and reconnection"] if loss else [])])
        finally:
            if p.poll() is None:
                self.send_signal(p, signal.SIGTERM)
                wait_exit()
            os.close(master)
            os.close(slave)

    def execute(self):
        self.run("init", "--state", str(self.state))
        identity_text = self.run("identity", "--state", str(self.state)).decode()
        device = re.search(r"device=([a-f0-9]{64})", identity_text)[1]
        pin = re.search(r"key-pin=([a-f0-9]{64})", identity_text)[1]
        folder = "11"*32
        self.run("register", "--state", str(self.state), "--folder", folder, "--root", str(self.data))
        membership = beneath(self.root, "membership.json")
        membership.write_text(json.dumps({"membership": {"folder": folder, "revision": 1, "prior_digest": "00"*32, "active": [{"device":device,"key_pin":pin}], "retired": []}}))
        membership.chmod(0o600)
        self.run("membership", "import", "--state", str(self.state), "--folder", folder, "--file", str(membership), "--approve")
        initial = b"protected before TUI"
        (self.data / "notes.txt").write_bytes(initial)
        self.run("scan", "--state", str(self.state), "--folder", folder)
        daemon = self.start_daemon()
        try:
            self.session("80x24-quit-tool", daemon, tool_mode="success")
            self.session("40x16-ctrl-c-tool-failure", daemon, size=(40,16), tool_mode="fail", exit_key=b"\x03")
            self.session("80x24-daemon-reconnect", daemon, loss=True)
            self.session("80x24-sigterm", daemon, exit_key=b"SIGTERM")
            self.session("80x24-tool-sigterm", daemon, tool_mode="cancel", exit_key=b"SIGTERM")
            piped = self.run("orbit", "tui", "--state", str(self.state))
            assert b"\x1b" not in piped and b"Daemon" in piped, "stdout pipe rendered a TUI"
            json_pipe = self.run("orbit", "tui", "--state", str(self.state), "--json")
            assert b"\x1b" not in json_pipe and json.loads(json_pipe)["service"]["running"], "JSON rendered a terminal screen"
            master, slave = pty.openpty()
            try:
                tty_input_pipe = subprocess.run([str(self.binary), "tui", "--state", str(self.state)], stdin=slave, capture_output=True, timeout=10)
                assert tty_input_pipe.returncode==0 and b"\x1b" not in tty_input_pipe.stdout, "TTY input with piped output rendered TUI"
                piped_input_tty = subprocess.run([str(self.binary), "tui", "--state", str(self.state)], stdin=subprocess.DEVNULL, stdout=slave, stderr=subprocess.PIPE, timeout=10)
                assert piped_input_tty.returncode==0, "piped input with TTY output hung"
                assert b"\x1b" not in os.read(master,65536), "piped input emitted terminal escapes"
            finally:
                os.close(master)
                os.close(slave)
            assert (self.data / "notes.txt").read_bytes()==initial, "TUI navigation mutated working bytes"
            edited = b"captured after every TUI client exited"
            (self.data / "notes.txt").write_bytes(edited)
            deadline = time.monotonic()+10
            captured = False
            while time.monotonic()<deadline:
                history = json.loads(self.run("orbit", "history", "notes.txt", "--state", str(self.state), "--folder", folder, "--json"))
                if any(v["digest"]==hashlib.sha256(edited).hexdigest() for v in history["versions"]):
                    captured = True
                    break
                time.sleep(0.1)
            assert captured, "daemon did not capture real edit after clients exited"
            assert json.loads(self.run("orbit", "status", "--state", str(self.state), "--json"))["service"]["running"], "daemon not running"
            self.results.append({"scenario":"pipe-and-continued-capture", "result":"passed", "assertions":["no terminal escapes in pipe", "protected working bytes intact", "actual new digest in captured history", "same device identity"]})
        finally:
            self.send_signal(daemon, signal.SIGTERM)
            daemon.wait(timeout=10)
        assert device in self.run("identity", "--state", str(self.state)).decode()
        if self.output:
            out = Path(self.output)
            out.mkdir(mode=0o700, parents=True, exist_ok=True)
            assert not any(out.iterdir()), "evidence output must be empty"
            for name, text in self.raw.items():
                (out / (name+".txt")).write_text(text)
            (out / "results.json").write_text(json.dumps(self.results, indent=2)+"\n")
        print(json.dumps(self.results, indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", default="bin/orbit")
    parser.add_argument("--output")
    parser.add_argument("--bare", action="store_true", help="exercise ordinary Orbit entry")
    args = parser.parse_args()
    binary = Path(args.binary).resolve(strict=True)
    root = Path(tempfile.mkdtemp(prefix="orbit-t09-pty-")).resolve()
    root.chmod(0o700)
    c = Campaign(root, binary, args.output, args.bare)
    try:
        c.execute()
    finally:
        for p, _ in c.children.values():
            if p.poll() is None:
                c.send_signal(p, signal.SIGCONT)
                c.send_signal(p, signal.SIGTERM)
                p.wait(timeout=10)
        c.checked()
        shutil.rmtree(root)


if __name__ == "__main__":
    main()
