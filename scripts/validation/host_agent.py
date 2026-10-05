#!/usr/bin/env python3
"""Private validation worker. All mutations remain in a fresh marked run root."""
import base64
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import resource
import socket
import threading
import signal
import sqlite3
import stat
import subprocess
import sys
import tempfile
import time
import urllib.request


def validated_root(req):
    root = Path(req["root"])
    if not root.is_absolute() or root.is_symlink() or root.resolve() != root:
        raise RuntimeError("run root must be canonical and cannot be symlinked")
    root_info = root.stat()
    if root_info.st_uid != os.getuid() or root_info.st_mode & 0o077:
        raise RuntimeError("run root must be private and owner-controlled")
    marker = root / (".filesync-pilot" if req.get("purpose") == "pilot" else ".filesync-disposable")
    info = marker.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_uid != os.getuid():
        raise RuntimeError("unsafe marker")
    if marker.read_text() != req["token"]:
        raise RuntimeError("disposable marker token mismatch")
    return root


def beneath(root, relative):
    path = root / relative
    if Path(relative).is_absolute() or ".." in Path(relative).parts or path == root:
        raise RuntimeError("target must remain strictly beneath run root")
    for part in [path, *path.parents]:
        if part == root:
            break
        if part.is_symlink():
            raise RuntimeError("symlink target refused")
    if path.exists() and path.is_file() and path.stat().st_nlink != 1:
        raise RuntimeError("hard-linked target refused")
    return path


def identity(pid):
    text = Path(f"/proc/{pid}/stat").read_text()
    fields = text[text.rfind(")") + 2:].split()
    return fields[19], fields[0]


def stop(root, name):
    record = json.loads(beneath(root, f"{name}.pid.json").read_text())
    if (root / ".filesync-pilot").exists() and record["kind"] == "sync":
        raise RuntimeError("fault interruption is forbidden in a personal pilot")
    pid = record["pid"]
    try:
        ticks, state = identity(pid)
    except FileNotFoundError:
        return
    if state == "Z":
        return
    argv = Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\0")
    if ticks != record["start_ticks"] or os.fsencode(root / "filesync") not in argv or not any(a in (os.fsencode(root / "state"), b"--state=" + os.fsencode(root / "state")) for a in argv):
        raise RuntimeError("refusing signal: process identity or state path changed")
    os.kill(pid, signal.SIGKILL if record["kind"] == "sync" else signal.SIGTERM)
    for _ in range(100):
        try:
            if identity(pid)[1] == "Z":
                return
        except FileNotFoundError:
            return
        time.sleep(0.05)
    raise RuntimeError("marked process did not stop")


def dispatch(req):
    action = req["action"]
    if action == "network-preflight":
        # Read-only prerequisite discovery is allowed before creating a run root.
        addresses = [str(ipaddress.IPv4Address(a)) for a in req["addresses"]]
        if any(ipaddress.ip_address(a).is_loopback or ipaddress.ip_address(a).is_unspecified
               or not ipaddress.ip_address(a).is_private and not ipaddress.ip_address(a) in ipaddress.ip_network("100.64.0.0/10")
               for a in addresses):
            raise RuntimeError("campaign addresses must be private nonloopback IPv4")
        routes = {a: json.loads(subprocess.check_output(["ip", "-j", "route", "get", a], text=True))
                  for a in addresses}
        tailscale = {"installed": False, "authenticated": False}
        if __import__("shutil").which("tailscale"):
            tailscale["installed"] = True
            result = subprocess.run(["tailscale", "status", "--json"], capture_output=True, text=True, timeout=10)
            if result.returncode == 0:
                status = json.loads(result.stdout)
                tailscale.update(state=status.get("BackendState"), addresses=status.get("TailscaleIPs", []),
                                 authenticated=status.get("BackendState") == "Running" and bool(status.get("Self", {}).get("Online")))
        return {"routes": routes, "tailscale": tailscale,
                "hostname": os.uname().nodename, "arch": os.uname().machine}
    if action == "create":
        if req.get("purpose") == "pilot":
            name = req.get("pilot_name", "FileSyncPilot-20261001")
            if not re.fullmatch(r"[A-Za-z0-9-]+", name):
                raise RuntimeError("unsafe pilot directory name")
            root = Path.home().resolve() / name
            root.mkdir(mode=0o700)  # Never overwrite/reuse existing user data.
            marker = root / ".filesync-pilot"
        else:
            root = Path(tempfile.mkdtemp(prefix="filesync-validation-", dir=Path.home())).resolve()
            marker = root / ".filesync-disposable"
        marker.write_text(req["token"])
        marker.chmod(0o600)
        (root / "data").mkdir(mode=0o700)
        return {"root": str(root)}
    if action == "inventory":
        return {"hostname": os.uname().nodename,
                "kernel": subprocess.check_output(["uname", "-sr"], text=True).strip(),
                "arch": subprocess.check_output(["uname", "-m"], text=True).strip(),
                "os": Path("/etc/os-release").read_text(),
                "filesystem": subprocess.check_output(["findmnt", "-no", "FSTYPE,OPTIONS", "-T", str(Path.home())], text=True).strip(),
                "cpu": subprocess.check_output(["lscpu", "-J"], text=True),
                "storage": subprocess.check_output(["df", "-B1", str(Path.home())], text=True),
                "route": subprocess.check_output(["ip", "route"], text=True)}
    root = validated_root(req)
    if action == "tcp-probe":
        address = str(ipaddress.IPv4Address(req["address"]))
        if ipaddress.ip_address(address).is_loopback or ipaddress.ip_address(address).is_unspecified:
            raise RuntimeError("native probe requires a nonloopback address")
        with socket.create_connection((address, int(req["port"])), timeout=3):
            return {"connected": True, "address": address, "port": int(req["port"])}
    if action == "terminal-pty":
        script = req["script"]
        if script not in ("terminal_pty_test.py", "terminal_onboarding_pty_test.py", "terminal_everyday_pty_test.py"):
            raise RuntimeError("unknown terminal campaign")
        args = [sys.executable, str(beneath(root, script)), "--binary", str(beneath(root, "filesync")),
                "--output", str(beneath(root, "pty-" + script.removesuffix(".py")))]
        if script == "terminal_pty_test.py":
            args.append("--bare")
        result = subprocess.run(args, capture_output=True, text=True, timeout=300)
        if result.returncode:
            raise RuntimeError("native PTY failed: " + (result.stdout + result.stderr)[-6000:])
        return {"returncode": result.returncode, "stdout": result.stdout}
    if action == "network":
        return {"addresses": json.loads(subprocess.check_output(["ip", "-j", "-4", "address"], text=True)),
                "linger": subprocess.run(["loginctl", "show-user", str(os.getuid()), "-p", "Linger"], capture_output=True, text=True).stdout.strip() or "unavailable",
                "tailscale_available": __import__("shutil").which("tailscale") is not None}
    if action == "terminal-query":
        state = beneath(root, "state")
        address = beneath(root, "state/control.addr").read_text().strip()
        host, port = address.rsplit(":", 1)
        if host != "127.0.0.1":
            raise RuntimeError("owner control must be loopback")
        token = beneath(root, "state/control.token").read_text().strip()
        request = urllib.request.Request("http://" + address + "/control/terminal/v1/query",
            data=json.dumps({"version": "1", **req["query"]}).encode(),
            headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request, timeout=8) as response:
            if response.headers["X-Orbit-Device"] != json.loads((state / "config.json").read_text())["device_id"]:
                raise RuntimeError("owner identity mismatch")
            return json.load(response)
    if action == "terminal-stop":
        # Launcher-created daemons also require exact private binary/state ownership.
        pidfile = beneath(root, "state/.agent.pid")
        if not pidfile.exists():
            return {"stopped": False}
        pid = int(pidfile.read_text().strip())
        try:
            ticks, process_state = identity(pid)
        except FileNotFoundError:
            return {"stopped": False}
        if process_state == "Z":
            return {"stopped": False}
        argv = Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\0")
        if os.fsencode(root / "filesync") not in argv or not any(
                a in (os.fsencode(root / "state"), b"--state=" + os.fsencode(root / "state")) for a in argv):
            raise RuntimeError("refusing signal: terminal daemon ownership mismatch")
        name = "terminal-" + str(pid)
        record = {"pid": pid, "start_ticks": ticks, "kind": "serve"}
        target = beneath(root, name + ".pid.json")
        if target.exists() and json.loads(target.read_text()) != record:
            raise RuntimeError("refusing signal: reused terminal PID")
        target.write_text(json.dumps(record))
        stop(root, name)
        return {"stopped": True}
    if action == "reserve-port":
        with socket.socket() as sock:
            sock.bind((req.get("address","127.0.0.1"),0))
            return {"port":sock.getsockname()[1]}
    if action == "put":
        target = beneath(root, req["path"])
        target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        target.write_bytes(base64.b64decode(req["data"]))
        target.chmod(req.get("mode", 0o600))
        return {}
    if action == "read":
        return {"data": base64.b64encode(beneath(root, req["path"]).read_bytes()).decode()}
    if action == "hash":
        h = hashlib.sha256()
        with beneath(root, "data/" + req["path"]).open("rb") as source:
            for data in iter(lambda: source.read(1024 * 1024), b""):
                h.update(data)
        return {"sha256": h.hexdigest()}
    if action == "run":
        proc = subprocess.Popen([str(root / "filesync"), *req["args"]], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        samples = {"sampled_peak_fds": 0, "sampled_peak_rss_kib": 0}
        finished = threading.Event()
        def sample():
            while not finished.is_set():
                try:
                    samples["sampled_peak_fds"] = max(samples["sampled_peak_fds"], len(list(Path(f"/proc/{proc.pid}/fd").iterdir())))
                    status = Path(f"/proc/{proc.pid}/status").read_text()
                    match = re.search(r"VmRSS:\s+(\d+)", status)
                    if match:
                        samples["sampled_peak_rss_kib"] = max(samples["sampled_peak_rss_kib"], int(match[1]))
                except (FileNotFoundError, ProcessLookupError):
                    pass
                finished.wait(0.02)
        sampler = threading.Thread(target=sample, daemon=True)
        sampler.start()
        try:
            stdout, stderr = proc.communicate(timeout=1800)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
            raise
        finally:
            finished.set()
            sampler.join(timeout=2)
        usage = resource.getrusage(resource.RUSAGE_CHILDREN)
        return {"returncode": proc.returncode, "stdout": stdout, "stderr": stderr,
                "resources": {**samples, "user_seconds": usage.ru_utime, "system_seconds": usage.ru_stime,
                              "peak_rss_kib": usage.ru_maxrss, "sampling_interval_seconds": 0.02}}
    if action == "start":
        if req.get("purpose") == "pilot" and req["kind"] == "sync":
            raise RuntimeError("fault workers are forbidden in a personal pilot")
        name = req["name"]
        if not re.fullmatch(r"[a-z0-9-]+", name):
            raise RuntimeError("unsafe worker name")
        if beneath(root, name + ".pid.json").exists():
            raise RuntimeError("worker name already used")
        worker = {**req, "action": "worker"}
        subprocess.Popen([sys.executable, str(root / "host_agent.py"), json.dumps(worker)],
                         stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        for _ in range(200):
            pidfile = beneath(root, name + ".pid.json")
            if pidfile.exists():
                return json.loads(pidfile.read_text())
            time.sleep(0.025)
        raise RuntimeError("worker start timeout")
    if action == "worker":
        name = req["name"]
        with beneath(root, name + ".log").open("wb") as log:
            proc = subprocess.Popen([str(root / "filesync"), *req["args"]], stdin=subprocess.DEVNULL, stdout=log, stderr=log)
            ticks, _ = identity(proc.pid)
            record = {"pid": proc.pid, "start_ticks": ticks, "kind": req["kind"]}
            temp = beneath(root, name + ".pid.tmp")
            temp.write_text(json.dumps(record))
            temp.rename(beneath(root, name + ".pid.json"))
            rc = proc.wait()
        beneath(root, name + ".done").write_text(str(rc))
        return {}
    if action == "poll":
        name = req["name"]
        log = beneath(root, name + ".log").read_text()
        done = beneath(root, name + ".done")
        port = re.search(r"peer-listener=127\.0\.0\.1:(\d+)", log)
        return {"log": log, "returncode": int(done.read_text()) if done.exists() else None,
                "port": int(port.group(1)) if port else None}
    if action == "stop":
        stop(root, req["name"])
        return {}
    if action == "verified-objects":
        # Inspect immutable durable objects without opening the owner's live DB.
        directory = beneath(root, "state/objects/sha256")
        count = 0
        for candidate in directory.rglob("*"):
            relative = candidate.relative_to(directory).as_posix().replace("/", "")
            if candidate.is_file() and re.fullmatch(r"[0-9a-f]{64}", relative):
                safe = beneath(root, candidate.relative_to(root))
                digest = hashlib.sha256()
                with safe.open("rb") as source:
                    for chunk in iter(lambda: source.read(1024 * 1024), b""):
                        digest.update(chunk)
                if digest.hexdigest() != relative:
                    raise RuntimeError("immutable object digest mismatch")
                count += 1
        return {"verified_objects": count}
    if action == "progress":
        with sqlite3.connect(f"file:{root}/state/metadata.sqlite?mode=ro", uri=True) as db:
            chunks = db.execute("SELECT COUNT(*) FROM transfer_chunks WHERE verified=1").fetchone()[0]
            ready = db.execute("SELECT COUNT(*) FROM versions WHERE content_state='ready'").fetchone()[0]
            return {"verified_chunks": chunks, "ready_versions": ready}
    if action == "work-summary":
        with sqlite3.connect(f"file:{root}/state/metadata.sqlite?mode=ro",uri=True) as db:
            return {"work":db.execute("SELECT task_kind,state,last_error,count(*) FROM durable_work_tasks GROUP BY task_kind,state,last_error").fetchall(),
                    "versions":db.execute("SELECT content_state,count(*) FROM versions GROUP BY content_state").fetchall()}
    if action == "integrity":
        with sqlite3.connect(f"file:{root}/state/metadata.sqlite?mode=ro", uri=True) as db:
            return {"sqlite": db.execute("PRAGMA integrity_check").fetchone()[0]}
    if action == "service-install":
        unit = ("filesync-pilot-" if req.get("purpose") == "pilot" else "filesync-validation-") + req["token"] + ".service"
        target = beneath(root, unit)
        text = req["template"].replace("/usr/bin/filesync", str(root / "filesync")).replace("/usr/bin/orbit", str(root / "filesync"))
        # Unique validation units must never create/remove the installed aliases.
        text = "\n".join(line for line in text.splitlines() if not line.startswith("Alias=")) + "\n"
        text = text.replace("%h/.local/state/filesync", str(root / "state"))
        text = text.replace("127.0.0.1:8080", "127.0.0.1:0")
        if req.get("peer_listen"):
            text = text.replace("--control-listen=127.0.0.1:0", "--control-listen=127.0.0.1:0 --peer-listen=" + req["peer_listen"] + " --sync-interval=2s --profile=" + req.get("profile","laptop"))
        # Prefixing custom roots/binaries is the documented user override.
        target.write_text(text)
        for command in [["link", str(target)], ["daemon-reload"], ["start", unit]]:
            subprocess.run(["systemctl", "--user", *command], check=True, capture_output=True, text=True)
        if req.get("purpose") == "pilot":
            subprocess.run(["systemctl","--user","enable",unit],check=True,capture_output=True,text=True)
        return {"unit": unit}
    if action in ["service-check", "service-restart", "service-uninstall"]:
        unit = ("filesync-pilot-" if req.get("purpose") == "pilot" else "filesync-validation-") + req["token"] + ".service"
        configured = subprocess.check_output(["systemctl", "--user", "show", unit, "--property=ExecStart", "--property=FragmentPath"], text=True)
        fragment = re.search(r"FragmentPath=(.*)", configured)
        if str(root / "filesync") not in configured or not fragment or Path(fragment[1]).resolve() != root / unit:
            raise RuntimeError("service ownership mismatch")
        if action == "service-restart":
            subprocess.run(["systemctl", "--user", "restart", unit], check=True, capture_output=True)
            time.sleep(0.3)
        elif action == "service-uninstall":
            subprocess.run(["systemctl", "--user", "stop", unit], check=True, capture_output=True)
            subprocess.run(["systemctl", "--user", "disable", unit], check=True, capture_output=True)
            beneath(root, unit).unlink()
            subprocess.run(["systemctl", "--user", "daemon-reload"], check=True, capture_output=True)
            return {"state_preserved": (root / "state/config.json").exists(), "root_preserved": (root / "data").exists()}
        info = subprocess.check_output(["systemctl", "--user", "show", unit, "--property=ActiveState", "--property=SubState", "--property=MainPID", "--property=ExecMainStatus"], text=True)
        if "ActiveState=active" not in info or "ExecMainStatus=0" not in info:
            raise RuntimeError("service unhealthy: " + info)
        pid = int(re.search(r"MainPID=(\d+)", info)[1])
        commandline = Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\0")
        if os.fsencode(root / "filesync") not in commandline:
            raise RuntimeError("unexpected service executable")
        import http.client
        sockets = {os.readlink(p)[8:-1] for p in Path(f"/proc/{pid}/fd").iterdir() if os.readlink(p).startswith("socket:[")}
        ports = []
        for line in Path(f"/proc/{pid}/net/tcp").read_text().splitlines()[1:]:
            values = line.split()
            if values[3] == "0A" and values[9] in sockets:
                ports.append(int(values[1].split(":")[1], 16))
        address = (root / "state/control.addr").read_text().strip()
        control_port = int(address.rsplit(":",1)[1])
        if not address.startswith("127.0.0.1:") or control_port not in ports:
            raise RuntimeError("expected owned loopback control socket")
        conn = http.client.HTTPConnection("127.0.0.1", control_port, timeout=5)
        conn.request("GET", "/")
        response = conn.getresponse()
        data = response.read()
        conn.close()
        if response.status != 200 or b'<div id="root"' not in data:
            raise RuntimeError("embedded UI unavailable")
        return {"systemd": info, "pid": pid, "control_address":address, "embedded_ui_status": response.status, "embedded_ui_sha256": hashlib.sha256(data).hexdigest()}
    raise RuntimeError("unknown action")


if __name__ == "__main__":
    request = json.loads(sys.argv[1]) if len(sys.argv) > 1 else json.load(sys.stdin)
    try:
        print(json.dumps(dispatch(request)))
    except Exception as error:
        print(json.dumps({"error": str(error)}))
        sys.exit(1)
