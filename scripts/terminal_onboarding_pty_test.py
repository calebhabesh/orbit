#!/usr/bin/env python3
"""Actual two-daemon keyboard onboarding under new private disposable roots."""
import argparse
import base64
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
import socket
import struct
import subprocess
import tempfile
import termios
import time
import urllib.request

from terminal_pty_test import Campaign
from terminal_vt import Screen

# Hermetic: never select the packaged hosted profile, so no daemon started here
# contacts the operated service (W14). Child processes inherit this.
os.environ.setdefault('ORBIT_DISABLE_PACKAGED_PROFILE', '1')


class Peer(Campaign):
    def query(self, kind, **fields):
        self.checked()
        endpoint = (self.state / "control.addr").read_text().strip()
        token = (self.state / "control.token").read_text().strip()
        request = urllib.request.Request("http://" + endpoint.removeprefix("http://") + "/control/terminal/v1/query",
                                         data=json.dumps(dict(version="1", kind=kind, **fields)).encode(),
                                         headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request, timeout=8) as response:
            assert response.headers["X-Orbit-Device"] == json.loads((self.state / "config.json").read_text())["device_id"]
            return json.load(response)

    def initialize(self):
        self.run("init", "--state", str(self.state))
        ip = socket.gethostbyname(socket.gethostname())
        if ip.startswith("127."):
            # Some Linux hosts map their hostname to 127.0.1.1. A UDP route
            # lookup chooses an existing interface without sending a packet.
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as route:
                route.connect(("192.0.2.1", 9))
                ip = route.getsockname()[0]
        assert not ip.startswith("127."), "nonloopback interface required"
        sockets = []
        for _ in range(2):
            sock = socket.socket(); sock.bind((ip, 0)); sockets.append(sock)
        peer, enroll = [f"{ip}:{sock.getsockname()[1]}" for sock in sockets]
        settings = dict(data_budget="4000000000", metadata_budget="268435456", reserve_bytes="16777216",
                        retention_seconds="0", concurrency="4", bandwidth_bytes_per_second="0",
                        peer_listen=peer, enrollment_listen=enroll, advertised_peer=peer, advertised_enrollment=enroll, startup="manual")
        file = self.state / "runtime.json"; file.write_text(json.dumps(settings)); file.chmod(0o600)
        for sock in sockets: sock.close()
        self.device = json.loads((self.state / "config.json").read_text())["device_id"]

    def start_daemon(self):
        # Missing system tooling is an explicit fixture: no host unit/lingering
        # configuration can be changed by the startup-error scenario.
        env = dict(os.environ, PATH=str(self.root / "empty-path"))
        p = self.spawn(["serve", "--state", str(self.state), "--control-listen", "127.0.0.1:0",
                        "--sync-interval", "1s", "--no-watch"], stdin=subprocess.DEVNULL,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env=env)
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            if p.poll() is not None: raise AssertionError("fixture daemon exited")
            try:
                self.query("capabilities"); return p
            except (OSError, urllib.error.URLError): time.sleep(.05)
        raise AssertionError("fixture daemon readiness timeout")

    def stop(self, p):
        self.send_signal(p, signal.SIGTERM); p.wait(timeout=12)


class UI:
    def __init__(self, peer, name, size=(80, 24), extra=None):
        self.peer, self.name = peer, name
        self.master, self.slave = pty.openpty()
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack("HHHH", size[1], size[0], 0, 0))
        self.before = copy.deepcopy(termios.tcgetattr(self.slave))
        self.screen = Screen(*size); self.raw = bytearray(); self.frames = []
        self.p = peer.spawn(["orbit", "tui", "--state", str(peer.state), "--no-color"] + (extra or []),
                            stdin=self.slave, stdout=self.slave, stderr=self.slave,
                            env=dict(os.environ, TERM="xterm-256color", NO_COLOR="1"))

    def pump(self, timeout=.02):
        if select.select([self.master], [], [], timeout)[0]:
            data = os.read(self.master, 65536); self.raw.extend(data); self.screen.feed(data)
            assert len(self.raw) < 4 << 20, "unbounded terminal output"

    def send(self, keys):
        os.write(self.master, keys); self.pump(.04)

    def wait(self, text, timeout=12):
        until = time.monotonic() + timeout
        while time.monotonic() < until:
            self.pump(.05)
            if text in self.screen.text():
                self.frames.append(self.screen.text()); return
            if self.p.poll() is not None: break
        raise AssertionError(f"{self.name}: visible {text!r} absent; frame={self.screen.text()!r}; rawtail={bytes(self.raw[-3500:])!r}")

    def replace(self, text):
        # End, delete-before-cursor, and one bounded bracketed paste.
        self.send(b"\x05\x15\x1b[200~" + text.encode() + b"\x1b[201~")

    def choose(self, value):
        # Startup is a selector (E03): right arrow until the choice shows.
        for _ in range(4):
            self.pump(.1)
            if f"Startup: ‹ {value} ›" in self.screen.text(): return
            self.send(b"\x1b[C")
        self.wait(f"Startup: ‹ {value} ›")

    def submit(self, text, presses=6):
        # Enter advances field by field and confirms on the last (E03).
        for _ in range(presses):
            self.send(b"\r"); time.sleep(.2); self.pump(.1)
            if text in self.screen.text(): return
        self.wait(text)

    def form(self, label, root, startup="manual"):
        # A join form hides Orbit name when the invitation carries one.
        values = (label, "Notes", str(root)) if "Orbit name:" in self.screen.text() else (label, str(root))
        for value in values:
            self.replace(value); self.send(b"\t")
        self.choose(startup); self.send(b"\t")
        # Keep loaded finite/network settings; Enter advances to the last
        # field and confirms there (E03).
        self.submit("Confirm adoption", presses=3)

    def invitation(self, code):
        self.replace(code); self.send(b"\r"); self.wait("Connection choices:")

    def back(self):
        self.send(b"\x1b"); time.sleep(.1); self.pump()

    def overview(self):
        # E08 lands healthy, configured devices on Files. Wait for the
        # initial query before selecting Overview; 'o' opens a file there.
        end = time.monotonic() + 12
        while time.monotonic() < end:
            self.pump(.05)
            if "Search folder:" in self.screen.text() or "Search page:" in self.screen.text():
                self.send(b"1")
                self.wait("Overview")
                return
        raise AssertionError(f"{self.name}: initial view absent: {self.screen.text()!r}")

    def finish(self):
        if self.p.poll() is None: self.send(b"\x03")
        until = time.monotonic()+12
        while self.p.poll() is None and time.monotonic()<until: self.pump(.05)
        if self.p.poll() is None:
            self.peer.send_signal(self.p, signal.SIGTERM)
            while self.p.poll() is None and time.monotonic()<until+5: self.pump(.05)
        self.p.wait(timeout=2)
        self.pump(.01)
        assert termios.tcgetattr(self.slave)==self.before, "terminal state not restored"
        assert b"\x1b[?1049l" in self.raw and b"\x1b[?2004l" in self.raw, "terminal modes not restored"
        frames="\n--- frame ---\n".join(self.frames)
        # Screen frames contain hard wrapping and padded cells. Redact the exact
        # known private fixture root even when a line break falls inside it.
        pattern=r"[ \t\r\n]*".join(re.escape(c) for c in str(self.peer.root))
        frames=re.sub(pattern,"<disposable>",frames)
        self.peer.record(self.name, frames.encode(), ["real typed control", "keyboard PTY", "termios restored"])
        os.close(self.master); os.close(self.slave)


def wait_bytes(a, b, name, value, uis):
    until=time.monotonic()+65
    while time.monotonic()<until:
        for ui in uis: ui.pump(.02)
        if all((root/name).exists() and (root/name).read_bytes()==value for root in (a,b)): return
        time.sleep(.1)
    raise AssertionError("verified file transfer timed out: "+name)


def invite(ui, peer, path, device=None):
    ui.send(b"s" if device else b"a"); ui.wait("Select folder"); ui.wait("> Notes")
    ui.send(b"\r")
    if device:
        ui.wait("Select device")
        items=peer.query("devices",limit="20")["items"]
        ui.wait("> "+items[0]["name"])
        index=next(i for i,it in enumerate(items) if it["id"]==device)
        ui.send(b"j"*index+b"\r")
        # Sharing with a known device keeps its explicit review.
        ui.wait("Reviewed membership revision:"); ui.send(b"\r")
    ui.wait("Private invitation")
    short = re.search(r'Pairing code: ([0-9A-Z]{4}-[0-9A-Z]{4})', ui.screen.text())
    ui.pairing_code = short.group(1) if short else None
    ui.send(b"s"); ui.wait("save_invitation")
    ui.replace(str(path)); ui.send(b"\r"); ui.wait("Private invitation saved")
    invitation=json.loads(path.read_text())
    assert path.stat().st_mode & 0o077 == 0
    assert invitation["capability"].encode() not in ui.raw, "capability leaked into masked UI"
    ui.back()
    return invitation


def approve(ui, peer, expected_request):
    ui.send(b"w"); ui.wait("Enrollment requests")
    requests=peer.query("requests",limit="20")["requests"]
    ui.wait("> "+requests[0]["label"])
    index=next(i for i,p in enumerate(requests) if p["id"]==expected_request)
    ui.send(b"j"*index+b"\r"); ui.wait("Exact request approval")
    code=requests[index]["verification_code"]
    assert code in ui.screen.text(), "review did not show transcript verification"
    ui.send(b"a"); ui.wait("Exact request approve completed")
    return code


def run(binary, output):
    roots=[]; peers=[]; active=[]; daemons=[]
    try:
        for _ in range(2):
            root=Path(tempfile.mkdtemp(prefix="orbit-t10-pty-")).resolve();root.chmod(0o700);roots.append(root)
            c=Peer(root,binary,None);c.initialize();peers.append(c);daemons.append(c.start_daemon())
        a,b=peers
        (a.data/"owner.txt").write_bytes(b"owner preexisting bytes")
        (b.data/"local.txt").write_bytes(b"joining preexisting bytes")
        ua=UI(a,"create-nonempty-back-edit");active.append(ua);ua.wait("Join an existing Orbit [j]")
        ua.send(b"c");ua.wait("Connection choices:")
        # Invalid root retains draft and requires correction.
        ua.replace("Laptop");ua.send(b"\t");ua.replace("Notes");ua.send(b"\t");ua.replace("relative-root");ua.submit("folder such as ~/Documents")
        ua.replace(str(a.data));ua.submit("Confirm adoption");ua.wait("files=1")
        ua.back();ua.wait("Connection choices:");ua.send(b"\r");ua.wait("Confirm adoption");ua.send(b"\r");ua.wait("Locally ready",timeout=25)
        created=a.query("folders",limit="20")["items"];assert len(created)==1
        folder=created[0]["id"]
        ua.back();ua.wait("[Overview]")
        inv=invite(ua,a,a.root/"invite.json")
        code="orbit-invitation:v2:"+base64.urlsafe_b64encode(json.dumps(inv).encode()).decode().rstrip("=")
        ub=UI(b,"join-private-errors-delayed-approval");active.append(ub);ub.wait("Join an existing Orbit [j]");ub.send(b"j");ub.wait("Join invitation")
        expired=dict(inv,expires_at="2000-01-01T00:00:00Z")
        expired_code="orbit-invitation:v2:"+base64.urlsafe_b64encode(json.dumps(expired).encode()).decode().rstrip("=")
        ub.replace(expired_code);ub.send(b"\r");ub.wait("INVITATION_EXPIRED")
        wrong=dict(inv,key_pin="ff"*32)
        wrong_code="orbit-invitation:v2:"+base64.urlsafe_b64encode(json.dumps(wrong).encode()).decode().rstrip("=")
        ub.replace(wrong_code);ub.send(b"\r");ub.wait("IDENTITY_MISMATCH")
        ub.invitation("\n".join(code[i:i+40] for i in range(0,len(code),40)));ub.form("Pi",b.data);ub.send(b"\r");ub.wait("Waiting for approval",timeout=25)
        pending=b.query("setups",limit="20")["items"];assert len(pending)==1
        operation=pending[0]["id"];before=b.query("operation",id=operation)
        request=before["join"]["request"];assert not before["readiness"]["approved"]
        assert b"Locally ready" not in ub.raw and inv["capability"].encode() not in ub.raw
        ub.finish();active.remove(ub)
        # Pending approval survives client exit and a real daemon restart.
        b.stop(daemons[1]);daemons[1]=b.start_daemon()
        ub=UI(b,"resume-after-daemon-restart",size=(40,16));active.append(ub);ub.wait("Waiting for approval")
        after=b.query("operation",id=operation)
        assert after["join"]["request"]==request and after["join"]["attempt"]==before["join"]["attempt"]
        verification=approve(ua,a,request)
        assert verification==before["requests"][0]["verification_code"], "cross-device verification mismatch"
        wait_bytes(a.data,b.data,"owner.txt",b"owner preexisting bytes",active)
        wait_bytes(a.data,b.data,"local.txt",b"joining preexisting bytes",active)
        ub.wait("Locally ready",timeout=40)
        ub.back();ub.wait("Overview |")
        ua.back();ua.wait("[Overview]")
        # Local pause/resume uses actual root state, and relocation preserves bytes.
        ua.send(b"f");ua.wait("[Orbits]");ua.wait("> Notes");ua.send(b"\r");ua.wait("Inspect folder");ua.wait("Local pause=")
        ua.send(b"p");ua.wait("Confirm local pause");ua.send(b"\r");ua.wait("Local pause=true")
        assert a.query("folder_management",folder=folder)["folder_management"]["paused"]
        ua.send(b"p");ua.wait("Confirm local resume");ua.send(b"\r");ua.wait("Local pause=false")
        ua.send(b"x");ua.wait("Unregister preview");ua.wait("Working files are preserved");ua.back();ua.wait("[Orbits]")
        # Separate second folder and exact known-device invitation. Restart only
        # the fixture inviter to reset its real process-local admission bucket.
        a.stop(daemons[0]);daemons[0]=a.start_daemon()
        second_a=a.root/"second";second_a.mkdir(mode=0o700);(second_a/"second.txt").write_bytes(b"second-folder bytes")
        second_b=b.root/"second";second_b.mkdir(mode=0o700)
        ua.send(b"c");ua.wait("Connection choices:");ua.form("Laptop",second_a);ua.send(b"\r");ua.wait("Locally ready",timeout=25);ua.back();ua.wait("[Orbits]")
        # Select the second folder by its actual named-page position.
        items=a.query("folders",limit="20")["items"];second=next(it for it in items if it["root"]==str(second_a));idx=items.index(second)
        ua.send(b"k"*len(items)+b"j"*idx+b"\r");ua.wait("Inspect folder");ua.wait("Local pause=")
        # share shortcut on detail skips folder selection.
        ua.send(b"s");ua.wait("Select device");devices=a.query("devices",limit="20")["items"];ua.wait("> "+devices[0]["name"]);idx=next(i for i,it in enumerate(devices) if it["id"]==b.device)
        ua.send(b"j"*idx+b"\r");ua.wait("Reviewed membership revision:");ua.send(b"\r");ua.wait("Private invitation")
        transfer=a.root/"second-invite.json";ua.send(b"s");ua.wait("save_invitation");ua.replace(str(transfer));ua.send(b"\r");ua.wait("Private invitation saved");ua.back();ua.wait("[Orbits]")
        inv2=json.loads(transfer.read_text());assert inv2["folder"]==second["id"] and inv2["folder"]!=folder
        code2="orbit-invitation:v2:"+base64.urlsafe_b64encode(json.dumps(inv2).encode()).decode().rstrip("=")
        ub.send(b"J");ub.wait("Join invitation");ub.invitation(code2);ub.form("Pi",second_b);ub.send(b"\r");ub.wait("Waiting for approval",timeout=25)
        ops=b.query("setups",limit="20")["items"];op2=next(it["id"] for it in ops if it["root"]==str(second_b));join2=b.query("operation",id=op2)
        assert join2["join"]["request"]!=request and join2["join"]["attempt"]!=before["join"]["attempt"]
        approve(ua,a,join2["join"]["request"])
        wait_bytes(second_a,second_b,"second.txt",b"second-folder bytes",active);ub.wait("Locally ready",timeout=40)
        # Existing relocation control runs from a reviewed local source/destination.
        ua.back();ua.wait("[Orbits]")
        items=a.query("folders",limit="20")["items"];idx=next(i for i,it in enumerate(items) if it["id"]==folder)
        ua.send(b"k"*len(items)+b"j"*idx+b"\r");ua.wait("Inspect folder");ua.wait("Local pause=")
        relocated=a.root/"relocated-notes"
        ua.send(b"l");ua.wait("relocate_form");ua.replace(str(relocated));ua.send(b"\r");ua.wait("Confirm relocation");ua.send(b"\r");ua.wait("Local folder action completed: relocate",timeout=30)
        assert a.query("folder_management",folder=folder)["folder_management"]["root"]==str(relocated)
        a.data=relocated
        assert (a.data/"owner.txt").read_bytes()==b"owner preexisting bytes"
        # Actual startup error under an empty PATH does not change any host unit.
        ua.back();ua.wait("[Orbits]");ua.send(b"c");ua.wait("Connection choices:")
        blocked=a.root/"startup-block";ua.form("Laptop",blocked,startup="login");ua.send(b"\r");ua.wait("SYSTEMD_UNAVAILABLE",timeout=25)
        blocked_ops=a.query("setups",limit="20")["items"];assert any(it["root"]==str(blocked) for it in blocked_ops)
        assert not blocked.exists(), "startup failure created unreviewed/root bytes"
        for ui in list(active):ui.finish();active.remove(ui)
        (a.data/"after-ui.txt").write_bytes(b"daemon work after every client exit")
        wait_bytes(a.data,b.data,"after-ui.txt",b"daemon work after every client exit",[])
        history=a.query("history",folder=folder,path="after-ui.txt",limit="20")
        assert any(v["digest"]==hashlib.sha256(b"daemon work after every client exit").hexdigest() for v in history["versions"])
        for peer in peers:assert json.loads((peer.state/"config.json").read_text())["device_id"]==peer.device
        assert (a.data/"owner.txt").read_bytes()==b"owner preexisting bytes"
        assert (b.data/"local.txt").read_bytes()==b"joining preexisting bytes"
        results=[dict(scenario="two-process-onboarding",result="passed",assertions=[
            "nonempty measured review; validation/back/edit", "private expired and wrong-pin refusal", "exact transcript code and approval",
            "pending client exit/reopen/daemon restart", "actual verified bidirectional file bytes", "local pause/resume, actual relocation and unregister preview",
            "second folder distinct consent and attempt; device identities retained", "actual startup failure under missing-system-tool fixture", "terminal restoration"])]
        if output:
            out=Path(output);out.mkdir(mode=0o700,parents=True,exist_ok=True);assert not any(out.iterdir())
            for i,peer in enumerate(peers):
                for name,frame in peer.raw.items():
                    (out/f"{i}-{name}.txt").write_text(frame)
            (out/"results.json").write_text(json.dumps(results,indent=2)+"\n")
        print(json.dumps(results,indent=2))
    finally:
        for ui in active:ui.finish()
        for c in peers:
            for p,_ in c.children.values():
                if p.poll() is None:c.send_signal(p,signal.SIGTERM);p.wait(timeout=12)
            c.checked();shutil.rmtree(c.root)


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument("--binary",default="bin/orbit");parser.add_argument("--output")
    args=parser.parse_args();run(Path(args.binary).resolve(strict=True),args.output)

if __name__=="__main__":main()
