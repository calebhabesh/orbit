#!/usr/bin/env python3
"""Gracefully upgrade only the marked, prepared pilot's three user services."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import re
import time

from harness import Node


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--setup", required=True, type=Path)
    parser.add_argument("--packages", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    if args.output.exists():
        parser.error("preserve existing upgrade evidence")
    setup = json.loads(args.setup.read_text())
    packages = json.loads(args.packages.read_text())
    report = {"source_commit": packages["source_commit"], "started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
              "type": "ordinary service upgrade; no fault injection or owner-use claim", "hosts": [], "success": False}
    try:
        for saved in setup["hosts"]:
            node = Node(saved["host"], saved["role"], "pilot")
            node.root = saved["root"]
            unit = saved["installation"]["unit"]
            node.token = re.fullmatch(r"orbit-pilot-([a-f0-9]{32})\.service", unit)[1]
            # Every following worker operation verifies this exact root/marker.
            before = node.call("service-check")
            old_binary = base64.b64decode(node.call("read", path="orbit")["data"])
            old_unit = base64.b64decode(node.call("read", path=unit)["data"]).decode()
            listen = re.search(r"--peer-listen=([^\s]+)", old_unit)[1]
            arch = "arm64" if saved["inventory"]["arch"] == "aarch64" else "amd64"
            new_binary = Path("bin/orbit-linux-arm64" if arch == "arm64" else "bin/orbit").read_bytes()
            expected_hash = packages["artifacts"][arch]["binary_sha256"]
            if hashlib.sha256(new_binary).hexdigest() != expected_hash:
                raise RuntimeError("binary does not match verified package")
            entry = {"host": node.host, "root": node.root, "before": before, "binary_sha256": expected_hash}
            report["hosts"].append(entry)
            node.cli("maintenance", "preflight")
            node.call("service-uninstall")
            try:
                node.cli("maintenance", "backup", "--out", node.root + "/pre-upgrade-" + expected_hash[:12] + ".sqlite")
                node.put("orbit", new_binary, mode=0o700)
                node.call("service-install", template=Path("packaging/systemd/orbit.service").read_text(),
                          peer_listen=listen, profile="pi" if node.role == "pi" else "laptop")
                time.sleep(0.4)
                entry["after"] = node.call("service-check")
                entry["integrity"] = node.call("integrity")
                entry["version"] = node.cli("version").strip()
                if entry["integrity"]["sqlite"] != "ok":
                    raise RuntimeError("upgraded database integrity failed")
            except Exception:
                # Stop only this owned unit before restoring its original binary.
                try:
                    node.call("service-uninstall")
                except RuntimeError:
                    pass
                node.put("orbit", old_binary, mode=0o700)
                node.call("service-install", template=old_unit, peer_listen=None)
                raise
            entry["success"] = True
            print("PASS ordinary pilot upgrade: " + node.host, flush=True)
        report["success"] = True
    finally:
        args.output.write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
