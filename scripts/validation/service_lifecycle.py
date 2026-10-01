#!/usr/bin/env python3
"""Install/restart/remove ONLY a unique user unit pointing into a marked root."""
import argparse
import hashlib
import json
from pathlib import Path
import time
import uuid

from harness import Node


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--hosts", nargs="+", default=["local", "rpi", "vps"])
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    template = Path("packaging/systemd/filesync.service").read_text()
    results = []
    try:
        for index, host in enumerate(args.hosts):
            node = Node(host, "service" + str(index))
            node.setup(uuid.uuid4().hex * 2)
            node.put("data/keep.txt", b"installation and uninstall preserve these bytes\n")
            node.scan()
            before_progress=node.call("progress")
            installed = False
            entry = {"host": host, "inventory": node.inventory, "root": node.root, "success": False,
                     "template_sha256": hashlib.sha256(template.encode()).hexdigest(), "binary_sha256": node.binary_hash}
            results.append(entry)
            try:
                entry["installation"] = node.call("service-install", template=template)
                installed = True
                time.sleep(0.3)
                entry["before"] = node.call("service-check")
                edited=b"ordinary edit while installed service is running\n"
                node.put("data/keep.txt",edited)
                deadline=time.monotonic()+20
                while time.monotonic()<deadline:
                    if node.call("progress")["ready_versions"]>before_progress["ready_versions"]:break
                    time.sleep(0.1)
                else:raise RuntimeError("service is alive but cannot capture ordinary edits")
                entry["ordinary_capture"]={"ready_versions_increased":True,"expected_sha256":hashlib.sha256(edited).hexdigest()}
                entry["after"] = node.call("service-restart")
                if entry["before"]["pid"] == entry["after"]["pid"]:
                    raise RuntimeError("restart did not change service process")
            finally:
                if installed:
                    entry["uninstall"] = node.call("service-uninstall")
            entry["content"] = node.call("hash", path="keep.txt")
            entry["integrity"] = node.call("integrity")
            if not all(entry["uninstall"].values()) or entry["integrity"]["sqlite"] != "ok" or entry["content"]["sha256"]!=entry["ordinary_capture"]["expected_sha256"]:
                raise RuntimeError("state preservation failed")
            entry["success"] = True
            print(f"PASS native user-service lifecycle: {host}", flush=True)
    finally:
        (args.output / "service-lifecycle.json").write_text(json.dumps({"results": results,
             "limitation": "Template with explicit private paths/ephemeral port overrides; does not install system-wide deb/rpm or enable lingering",
             "success": bool(results) and all(r["success"] for r in results)}, indent=2) + "\n")


if __name__ == "__main__":
    main()
