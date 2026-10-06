#!/usr/bin/env python3
"""T11 actual two-daemon everyday controls; fresh marked roots only."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import sys
import tempfile
import time

from terminal_onboarding_pty_test import Peer, UI, invite, approve, wait_bytes

# Hermetic: never select the packaged hosted profile, so no daemon started here
# contacts the operated service (W14). Child processes inherit this.
os.environ.setdefault('ORBIT_DISABLE_PACKAGED_PROFILE', '1')


def until(fn, uis=(), timeout=35):
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        for ui in uis: ui.pump(.01)
        value=fn()
        if value: return value
        time.sleep(.05)
    raise AssertionError("authoritative state timeout")


def folder_ui(ui):
    ui.back()
    ui.send(b"f");until(lambda: "[Folders]" in ui.screen.text() or "Folders |" in ui.screen.text(), (ui,));ui.wait("Notes")
    # Refresh may retain attention selection on overview, but Folders has one root.
    ui.send(b"\r");ui.wait("Inspect folder");ui.wait("Local pause=")


def conflicts(ui, peer, folder, path):
    ui.send(b"C");ui.wait("Orbit | Conflicts");ui.wait(path)
    items=peer.query("conflicts",folder=folder,limit="20")["attention"]
    idx=next(i for i,it in enumerate(items) if it["path"]==path)
    ui.send(b"k"*len(items)+b"j"*idx+b"\r");ui.wait("Review versions:");ui.wait("Exact heads=2")


def run(binary, output):
    peers=[];roots=[];active=[];daemons=[]
    try:
        for _ in range(2):
            root=Path(tempfile.mkdtemp(prefix="orbit-t11-pty-")).resolve();root.chmod(0o700);roots.append(root)
            p=Peer(root,binary,output);p.initialize();peers.append(p);daemons.append(p.start_daemon())
        a,b=peers
        for name in ("doc","select","copies"):(a.data/name).write_bytes(b"original bytes")
        helper=a.root/"editor.py"
        helper.write_text("""import pathlib,sys,termios
s=termios.tcgetattr(0)
assert s[3]&termios.ICANON and s[3]&termios.ECHO
print('EDIT_READY',flush=True)
value=input()
if value=='fail': sys.exit(7)
pathlib.Path(sys.argv[-1]).write_text(value)
print('EDIT_DONE',flush=True)
""")
        diff=a.root/"diff.py"
        diff.write_text("""import pathlib,sys,termios
assert termios.tcgetattr(0)[3]&termios.ICANON
assert len(sys.argv)==3
assert all(pathlib.Path(p).read_bytes() for p in sys.argv[1:])
print('DIFF_READY',flush=True)
input()
""")
        extra=["--editor",f"'{sys.executable}' '{helper}'","--diff",f"'{sys.executable}' '{diff}'"]
        ua=UI(a,"everyday-offline-conflict-editor",extra=extra);active.append(ua);ua.wait("Join an existing Orbit [j]")
        ua.send(b"c");ua.wait("Review setup inputs");ua.form("Laptop",a.data);ua.send(b"\r");ua.wait("Locally ready",timeout=25);ua.back();ua.wait("[Overview]")
        folder=a.query("folders",limit="20")["items"][0]["id"]
        inv=invite(ua,a,a.root/"invite.json")
        code="orbit-invitation:v2:"+base64.urlsafe_b64encode(json.dumps(inv).encode()).decode().rstrip("=")
        ub=UI(b,"paired-peer",size=(40,16));active.append(ub);ub.wait("Join an existing Orbit [j]");ub.send(b"j");ub.wait("Join invitation");ub.invitation(code);ub.form("Pi",b.data);ub.send(b"\r");ub.wait("Waiting for approval",timeout=25)
        op=b.query("setups",limit="20")["items"][0]["id"]
        request=b.query("operation",id=op)["join"]["request"]
        approve(ua,a,request)
        for name in ("doc","select","copies"):wait_bytes(a.data,b.data,name,b"original bytes",active)
        ub.wait("Locally ready",timeout=35);ub.finish();active.remove(ub)
        ua.back();ua.wait("[Overview]")
        b.stop(daemons[1]);daemons[1]=None
        for name in ("doc","select","copies"):
            (b.data/name).write_bytes(b"Pi offline bytes")
            (a.data/name).write_bytes(b"Laptop online bytes")
        b.run("scan","--state",str(b.state),"--folder",folder)
        until(lambda: all(a.query("history",folder=folder,path=n,limit="20")["versions"][0]["digest"]==hashlib.sha256(b"Laptop online bytes").hexdigest() for n in ("doc","select","copies")),active)
        daemons[1]=b.start_daemon()
        until(lambda: all(len(a.query("content_review",folder=folder,path=n)["content_review"]["heads"])==2 for n in ("doc","select","copies")),active)
        ua.wait("CONFLICT",timeout=20)
        folder_ui(ua);conflicts(ua,a,folder,"doc")
        frozen=a.query("content_review",folder=folder,path="doc")["content_review"]["heads"]
        # Actual external diff uses two admitted exact source paths, canonical tty.
        ua.send(b"d");ua.wait("DIFF_READY");ua.send(b"\r");ua.wait("Retained editor session");ua.wait("Tool returned")
        # Failed editor preserves the admitted result and returns terminal ownership.
        ua.send(b"e");ua.wait("EDIT_READY");ua.send(b"fail\r");ua.wait("Tool failed")
        # A new remote version arrives while the owner tool has the terminal.
        ua.send(b"e");ua.wait("EDIT_READY")
        b.stop(daemons[1]);daemons[1]=None
        (b.data/"doc").write_bytes(b"Pi newer offline bytes")
        b.run("scan","--state",str(b.state),"--folder",folder)
        daemons[1]=b.start_daemon()
        until(lambda: a.query("content_review",folder=folder,path="doc")["content_review"]["heads"]!=frozen,active)
        ua.send(b"old reviewed merge\r");ua.wait("Tool returned")
        ua.send(b"u");ua.wait("STALE_VIEW")
        assert (a.data/"doc").read_bytes()==b"Laptop online bytes", "stale editor replaced bytes"
        assert len(a.query("content_review",folder=folder,path="doc")["content_review"]["heads"])==2
        # Metadata can arrive before verified payloads on native hosts. Wait for
        # all exact sources before asking the UI for a fresh editor session.
        until(lambda: all(v["availability"] == "ready" for v in a.query("content_review",folder=folder,path="doc")["versions"]), active)
        ua.send(b"r");ua.wait("Retained editor session");ua.wait("fresh",timeout=10)
        ua.send(b"e");ua.wait("EDIT_READY");ua.send(b"fresh reviewed merge\r");ua.wait("Tool returned")
        ua.send(b"u");ua.wait("Confirm reviewed merge");ua.wait("Staged result=");ua.send(b"\r");ua.wait("Durable content operation");ua.wait("State: completed")
        wait_bytes(a.data,b.data,"doc",b"fresh reviewed merge",active)
        current=a.query("content_review",folder=folder,path="doc")["content_review"]["heads"];assert len(current)==1
        # Choose one exact version, keeping the competing history.
        folder_ui(ua);conflicts(ua,a,folder,"select")
        r=a.query("content_review",folder=folder,path="select");source=r["versions"][0]
        ua.send(b"\r");ua.wait("Confirm reviewed select");ua.send(b"\r");ua.wait("State: completed")
        expected=b"Laptop online bytes" if source["version"]["author"]==a.device else b"Pi offline bytes"
        wait_bytes(a.data,b.data,"select",expected,active)
        # Keep every file version in separately reviewed collision-free paths.
        folder_ui(ua);conflicts(ua,a,folder,"copies")
        versions=a.query("content_review",folder=folder,path="copies")["versions"]
        ua.send(b"K");ua.wait("Separate copy destinations")
        ua.replace("copies-laptop");ua.send(b"\t");ua.replace("copies-pi");ua.send(b"\r");ua.wait("Confirm reviewed keep_copies");ua.send(b"\r");ua.wait("State: completed")
        for i,name in enumerate(("copies-laptop","copies-pi")):
            expected=b"Laptop online bytes" if versions[i]["version"]["author"]==a.device else b"Pi offline bytes"
            wait_bytes(a.data,b.data,name,expected,active)
        # Deleted history remains actual known ancestry; restore at original path.
        (a.data/"doc").unlink()
        until(lambda:not (b.data/"doc").exists(),active)
        folder_ui(ua);ua.send(b"D");ua.wait("Deleted files");ua.wait("> doc");ua.send(b"\r");ua.wait("History: doc")
        versions=a.query("history",folder=folder,path="doc",limit="20")["versions"]
        index=next(i for i,v in enumerate(versions) if v["digest"]==hashlib.sha256(b"fresh reviewed merge").hexdigest())
        ua.send(b"j"*index+b"\r");ua.wait("Review versions:");ua.wait("Historical source:")
        ua.send(b"\r");ua.wait("Confirm reviewed restore");ua.send(b"\r");ua.wait("State: completed")
        wait_bytes(a.data,b.data,"doc",b"fresh reviewed merge",active)
        restored=a.query("content_review",folder=folder,path="doc")["content_review"]["heads"];assert len(restored)==1 and restored!=current
        # Path search and separate copy through the same exact source controls.
        folder_ui(ua);ua.send(b"h");ua.wait("Find path history");ua.send(b"/");ua.replace("doc");ua.send(b"\r");ua.wait("> doc");ua.send(b"\r");ua.wait("History: doc");ua.send(b"\r");ua.wait("Review versions:");ua.wait("Historical source:")
        ua.send(b"c");ua.wait("Separate copy destinations");ua.replace("doc-recovered");ua.send(b"\r");ua.wait("Review versions:");ua.send(b"\r");ua.wait("Confirm reviewed separate_copy");ua.send(b"\r");ua.wait("State: completed")
        wait_bytes(a.data,b.data,"doc-recovered",b"fresh reviewed merge",active)
        assert a.query("content_review",folder=folder,path="doc")["content_review"]["heads"]==restored
        # Storage/maintenance are read-only. Paused/root issues remain visible.
        folder_ui(ua);ua.send(b"b");ua.wait("Storage and retention");ua.wait("data budget=");ua.send(b"m");ua.wait("Cleanup candidates=")
        assert (a.data/"doc").read_bytes()==b"fresh reviewed merge"
        folder_ui(ua);ua.send(b"v");ua.wait("Qualified copy status");ua.wait("stored=")
        ua.finish();active.remove(ua)
        # Narrow colorless keyboard screens and authoritative unavailable root.
        ua=UI(a,"narrow-storage-root-recovery",size=(40,16),extra=extra);active.append(ua);ua.wait("Overview")
        folder_ui(ua);ua.send(b"b");ua.wait("Storage and retention");ua.wait("budget=");ua.send(b"m");ua.wait("Cleanup candidates=")
        folder_ui(ua);ua.send(b"p");ua.wait("Confirm local pause");ua.send(b"\r");ua.wait("Local pause=true")
        assert a.query("folder_management",folder=folder)["folder_management"]["paused"]
        ua.send(b"p");ua.wait("Confirm local resume");ua.send(b"\r");ua.wait("Local pause=false")
        # Rename whole disposable root away; no scan can mass-delete its contents.
        missing=a.root/"temporarily-missing";a.data.rename(missing)
        ua.back();until(lambda: "Search page" in ua.screen.text(), (ua,))
        ua.send(b"n");ua.wait("Attention");ua.wait("ROOT_UNAVAILABLE",timeout=20)
        assert a.query("status",folder=folder,limit="20")["readiness"]["root_available"] is False
        missing.rename(a.data)
        assert (a.data/"doc").read_bytes()==b"fresh reviewed merge"
        until(lambda: a.query("status",folder=folder,limit="20")["readiness"]["root_available"] is True, active)
        ua.finish();active.remove(ua)
        # Same daemons capture and transfer verified bytes after every client quits.
        (a.data/"after-quit").write_bytes(b"daemon continues")
        wait_bytes(a.data,b.data,"after-quit",b"daemon continues",())
        assert len(a.query("content_review",folder=folder,path="doc")["content_review"]["heads"])==1
        assert json.loads((a.state/"config.json").read_text())["device_id"]==a.device
        for peer in peers:
            if output:
                output.mkdir(parents=True,exist_ok=True)
                for name,text in peer.raw.items():(output/(name+".txt")).write_text(text)
        results = {"result":"passed","scenarios":["real offline/reconnect concurrent heads","canonical diff/editor failed exit","stale editor refusal and fresh review","exact select and keep copies","delete history restore and separate copy","storage preview narrow root block","terminal restoration and daemon transfer after quit"]}
        if output:
            (output/"results.json").write_text(json.dumps(results,indent=2)+"\n")
        print(json.dumps(results))
    finally:
        for ui in active:
            try:ui.finish()
            except Exception:pass
        for peer in peers:
            for child,_ in peer.children.values():
                if child.poll() is None:
                    peer.send_signal(child,signal.SIGTERM);child.wait(timeout=12)
            peer.checked();shutil.rmtree(peer.root)


if __name__=="__main__":
    parser=argparse.ArgumentParser();parser.add_argument("--binary",required=True,type=Path);parser.add_argument("--output",type=Path)
    args=parser.parse_args();run(args.binary.resolve(),args.output)
