#!/usr/bin/env python3
"""E03 keyboard and paste conventions in a real PTY, two disposable daemons.

Drives the setup form with arrows only, Tab only and Enter only; pastes the
invitation bracketed and unbracketed, each after a failed attempt, and checks
the field holds exactly the new code; approves with arrow keys only.

Run: make test-terminal-keys-pty
"""
import argparse
import base64
import json
import os
import shutil as _shutil
from pathlib import Path
import shutil
import signal
import tempfile
import time

from terminal_onboarding_pty_test import Peer, UI, invite

DOWN, ENTER, TAB, RIGHT = b"\x1b[B", b"\r", b"\t", b"\x1b[C"


def code_of(inv):
    return "orbit-invitation:v2:" + base64.urlsafe_b64encode(json.dumps(inv).encode()).decode().rstrip("=")


def fill(ui, nav, label, root, confirm=True):
    """Type the three named fields, moving with nav; leave the rest default."""
    # A join form hides Orbit name when the invitation carries one.
    values = (label, "Notes", str(root)) if "Orbit name:" in ui.screen.text() else (label, str(root))
    for value in values:
        ui.replace(value)
        ui.send(nav)
    # Startup selector: typing is ignored, so a stray letter changes nothing.
    ui.send(b"z")
    ui.wait("Startup: ‹ manual ›")
    if not confirm:
        return
    # Fields are name, folder, root, startup, data budget, connection: two more
    # moves reach the last field, where Enter confirms. Enter-only presses
    # Enter for those moves too.
    if nav != ENTER:
        ui.send(nav + nav)
    ui.submit("Confirm adoption", presses=3)


def keyboard_variants(ua, a):
    seen = []
    for name, nav in (("arrows", DOWN), ("tab", TAB), ("enter", ENTER)):
        ua.send(b"c"); ua.wait("Connection choices:")
        root = a.root / f"root-{name}"; root.mkdir(mode=0o700)
        fill(ua, nav, f"Laptop-{name}", root)
        ua.wait(f"root-{name}")
        seen.append(name)
        ua.back(); ua.wait("Connection choices:"); ua.back(); ua.back()
        ua.wait("Create or join")
    return seen


def paste_variants(ub, inv):
    code = code_of(inv)
    expired = code_of(dict(inv, expires_at="2000-01-01T00:00:00Z"))
    # Bracketed: a failed paste, then a second bracketed paste replaces it.
    ub.send(b"j"); ub.wait("Join invitation")
    ub.send(b"\x1b[200~" + expired.encode() + b"\x1b[201~"); ub.send(ENTER); ub.wait("INVITATION_EXPIRED")
    ub.send(b"\x1b[200~" + code.encode() + b"\x1b[201~")
    ub.wait(f"{len(code):,} characters received")
    ub.send(ENTER); ub.wait("Connection choices:")
    ub.back(); ub.wait("Join invitation"); ub.back(); ub.wait("[Overview]")
    # Unbracketed: typed characters, a failed attempt, then a typed re-paste
    # replaces the kept value instead of appending to it.
    ub.send(b"J"); ub.wait("Join invitation")
    for chunk in range(0, len(expired), 256):
        ub.send(expired[chunk:chunk + 256].encode())
    ub.send(ENTER); ub.wait("INVITATION_EXPIRED")
    for chunk in range(0, len(code), 256):
        ub.send(code[chunk:chunk + 256].encode())
    ub.wait(f"{len(code):,} characters received")
    ub.send(ENTER); ub.wait("Connection choices:")
    return ["bracketed replace after failure", "unbracketed replace after failure"]


def open_invitation(ui):
    ui.send(b"a"); ui.wait("Select folder"); ui.wait("> Notes"); ui.send(ENTER)
    ui.wait("characters and stays hidden")


def invitation_out_checks(ui, a, columns):
    """E05: reveal one exact line, OSC 52 copy, private default save."""
    open_invitation(ui)
    start = len(ui.raw)
    ui.send(b"v"); ui.wait("Press Enter to return")
    raw = bytes(ui.raw[start:])
    # Cooked output may translate line ends; the code itself has none.
    after = raw[raw.index(b"wraps it.") + len(b"wraps it."):].lstrip(b"\r\n")
    code = after[:min(i for i in (after.find(b"\r"), after.find(b"\n")) if i >= 0)]
    assert b"\r" not in code and b"\n" not in code and b"\x1b" not in code, "revealed code is not one clean line"
    assert code.startswith(b"orbit-invitation:v"), "revealed text is not an invitation code"
    ui.send(ENTER); ui.wait("characters and stays hidden")
    assert b"\x1b[3J" in bytes(ui.raw[start:]), "screen and scrollback not cleared after reveal"
    start = len(ui.raw)
    ui.send(b"c"); ui.wait("to the clipboard (OSC 52)")
    copied = bytes(ui.raw[start:])
    osc = copied[copied.index(b"\x1b]52;c;") + 7:]
    payload = osc[:min(i for i in (osc.find(b"\x07"), osc.find(b"\x1b\\")) if i >= 0)]
    assert base64.b64decode(payload) == code, "OSC 52 payload is not the exact code"
    assert len(payload) <= 4 * ((16 << 10) + 2) // 3 + 4, "OSC 52 payload unbounded"
    ui.send(b"s"); ui.wait("Private output file")
    ui.send(ENTER); ui.wait("Saved: ")
    saved = max((p for p in a.state.iterdir() if p.name.startswith("invitation-") and p.suffix == ".json"), key=lambda p: p.stat().st_mtime_ns)
    assert saved.stat().st_mode & 0o777 == 0o600 and a.state.stat().st_mode & 0o777 == 0o700, "saved file not private"
    decoded = json.loads(base64.urlsafe_b64decode(code.split(b":", 2)[2] + b"=" * (-len(code.split(b":", 2)[2]) % 4)))
    file_inv = json.loads(saved.read_text())
    assert decoded["capability"] == file_inv["capability"] and decoded["folder"] == file_inv["folder"], "reveal and saved file disagree"
    ui.back(); ui.wait("[Overview]")
    return saved, f"{columns} columns: exact one-line reveal, OSC 52 payload, private save"


def join_with_file(ub, b, saved):
    """The join prompt accepts a file path (the SSH fallback)."""
    target = b.root / "orbit-invitation.json"
    _shutil.copyfile(saved, target); os.chmod(target, 0o600)
    ub.send(b"J"); ub.wait("Join invitation"); ub.wait("private file path")
    ub.replace(str(target)); ub.send(ENTER); ub.wait("Connection choices:")
    ub.back(); ub.wait("Join invitation"); ub.back(); ub.wait("[Overview]")
    target.unlink()


def approve_with_arrows(ua, a, request):
    ua.send(b"w"); ua.wait("Enrollment requests")
    requests = a.query("requests", limit="20")["requests"]
    index = next(i for i, p in enumerate(requests) if p["id"] == request)
    ua.send(DOWN * index + ENTER); ua.wait("Exact request approval")
    ua.send(b"a"); ua.wait("Exact request approve completed")


def run(binary, output):
    peers, active, daemons = [], [], []
    try:
        for _ in range(2):
            root = Path(tempfile.mkdtemp(prefix="orbit-e03-pty-")).resolve(); root.chmod(0o700)
            p = Peer(root, binary, None); p.initialize(); peers.append(p); daemons.append(p.start_daemon())
        a, b = peers
        ua = UI(a, "e03-create-keys", size=(100, 32)); active.append(ua); ua.wait("Join an existing Orbit [j]")
        variants = keyboard_variants(ua, a)
        # Create for real with Enter only.
        ua.send(b"c"); ua.wait("Connection choices:")
        fill(ua, ENTER, "Laptop", a.data)
        ua.send(ENTER); ua.wait("Locally ready", timeout=25)
        ua.back(); ua.wait("[Overview]")
        reveals = []
        for columns, rows in ((80, 24), (200, 50)):
            ur = UI(a, f"e05-reveal-{columns}", size=(columns, rows)); active.append(ur); ur.overview()
            saved, note = invitation_out_checks(ur, a, columns); reveals.append(note)
            ur.finish(); active.remove(ur)
        inv = invite(ua, a, a.root / "invite.json")
        ub = UI(b, "e03-join-paste", size=(100, 32)); active.append(ub); ub.wait("Join an existing Orbit [j]")
        pastes = paste_variants(ub, inv)
        ub.back(); ub.wait("Join invitation"); ub.back(); ub.wait("[Overview]")
        join_with_file(ub, b, saved); pastes.append("join prompt accepts a saved invitation file path")
        ub.send(b"J"); ub.wait("Join invitation")
        ub.replace(code_of(inv)); ub.send(ENTER); ub.wait("Connection choices:")
        fill(ub, ENTER, "Pi", b.data)
        ub.send(ENTER); ub.wait("Waiting for approval", timeout=25)
        request = b.query("setups", limit="20")["items"][0]["id"]
        request = b.query("operation", id=request)["join"]["request"]
        approve_with_arrows(ua, a, request)
        ub.wait("Locally ready", timeout=40)
        assert inv["capability"].encode() not in ub.raw, "capability shown"
        results = [dict(scenario="e03-keyboard-paste", result="passed",
                        assertions=[f"setup form via {v} only" for v in variants] + pastes + reveals + ["approval with arrow keys only", "selector ignores typing"])]
        if output:
            out = Path(output); out.mkdir(mode=0o700, parents=True, exist_ok=True)
            (out / "results.json").write_text(json.dumps(results, indent=2) + "\n")
        print(json.dumps(results, indent=2))
    finally:
        for ui in active: ui.finish()
        for p in peers:
            for proc, _ in p.children.values():
                if proc.poll() is None: p.send_signal(proc, signal.SIGTERM); proc.wait(timeout=12)
            p.checked(); shutil.rmtree(p.root)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", default="bin/orbit"); parser.add_argument("--output")
    args = parser.parse_args(); run(Path(args.binary).resolve(strict=True), args.output)


if __name__ == "__main__":
    main()
