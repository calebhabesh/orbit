#!/usr/bin/env python3
"""Native package/real PTY and unique user-service campaign; no host reboots."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import tarfile
import time

from harness import prepare_output
from terminal_native import TerminalNode, equal_bytes, wait


def run(dist, hosts, output):
    prepare_output(output)
    report = {"started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "success": False, "hosts": [],
              "limitations": ["Unique scoped user units; installed aliases preserved", "No logout/login/boot or unattended claim", "Automated use, not personal adoption"]}
    try:
        for i, host in enumerate(hosts):
            node = TerminalNode(host, "host"+str(i))
            installed = False
            entry = {"host": host, "success": False}
            report["hosts"].append(entry)
            try:
                node.prepare(dist)
                entry.update(root=node.root, inventory=node.inventory, network=node.network, provenance=node.provenance)
                for name in ("terminal_pty_test.py", "terminal_onboarding_pty_test.py", "terminal_everyday_pty_test.py", "terminal_vt.py"):
                    node.put(name, (Path("scripts") / name).read_bytes())
                entry["pty"] = {}
                for name in ("terminal_pty_test.py", "terminal_onboarding_pty_test.py", "terminal_everyday_pty_test.py"):
                    start = time.monotonic()
                    entry["pty"][name] = {**node.call("terminal-pty", script=name), "seconds": time.monotonic()-start}
                    # Each delivered runner writes sanitized transcripts/results.
                    directory = "pty-" + name.removesuffix(".py")
                    raw = node.call("read", path=directory+"/results.json")["data"]
                    target = output / (node.role + "-" + directory)
                    target.mkdir(mode=0o700)
                    (target / "results.json").write_bytes(base64.b64decode(raw))
                protected = b"native unique service keeps ordinary bytes\n"
                node.put("data/protected", protected)
                setup = node.setup()
                node.call("terminal-stop")
                with tarfile.open(dist / node.provenance["package"], "r:gz") as archive:
                    template = archive.extractfile("systemd/orbit.service").read().decode()
                template = template.replace("--control-listen=127.0.0.1:8080", "--control-listen=127.0.0.1:0 --sync-interval=1s")
                entry["installation"] = node.call("service-install", template=template)
                installed = True
                entry["before"] = wait("service ready", lambda: node.call("service-check"), 15)
                status = json.loads(node.orbit("status", "--json"))
                if not status["service"]["running"]:
                    raise RuntimeError("CLI cannot use installed running service")
                entry["terminal_status"] = status
                edited = b"captured by scoped native service\n"
                node.put("data/service-edit", edited)
                wait("service actual capture", lambda: any(v["digest"] == hashlib.sha256(edited).hexdigest()
                    for v in node.query("history", folder=setup["join"]["folder"], path="service-edit")["versions"]))
                entry["capture"] = equal_bytes([node], "service-edit", b"captured by scoped native service\n")
                entry["after_restart"] = node.call("service-restart")
                if entry["before"]["pid"] == entry["after_restart"]["pid"]:
                    raise RuntimeError("restart did not change PID")
                entry["after_restart_status"] = json.loads(node.orbit("status", "--json"))
                entry["uninstall"] = node.call("service-uninstall")
                installed = False
                entry["after_uninstall_status"] = json.loads(node.orbit("status", "--json"))
                if entry["after_uninstall_status"]["service"]["running"]:
                    raise RuntimeError("service removal left its daemon running")
                entry["integrity"] = node.call("integrity")
                entry["protected_bytes"] = equal_bytes([node], "protected", protected)
                config = json.loads(base64.b64decode(node.call("read", path="state/config.json")["data"]))
                if config["device_id"] != node.device or entry["integrity"]["sqlite"] != "ok":
                    raise RuntimeError("service lifecycle changed identity or damaged database")
                entry["success"] = True
                print("PASS native packaged PTY and user-service: " + host, flush=True)
            finally:
                if installed:
                    node.call("service-uninstall")
                node.cleanup()
        report["success"] = all(x["success"] for x in report["hosts"])
    finally:
        report["ended_utc"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        (output / "terminal-hosts.json").write_text(json.dumps(report, indent=2)+"\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", type=Path, default=Path("dist"))
    parser.add_argument("--hosts", nargs="+", default=["laptop", "rpi", "vps"])
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    run(args.dist, args.hosts, args.output)
