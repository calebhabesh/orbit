#!/usr/bin/env python3
"""Commit an isolated copy of current source, then reproduce from a clean clone.

The original index, branch and working tree are untouched. Active evidence under
--output is excluded from the snapshot; its source manifest records every input.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import uuid

from harness import prepare_output


def run(kernel, output):
    source = Path.cwd().resolve()
    output = output.resolve()
    prepare_output(output)
    parent = Path(tempfile.mkdtemp(prefix="orbit-t13-snapshot-")).resolve()
    (parent / ".orbit-disposable").write_text(uuid.uuid4().hex)
    snapshot = parent / "source"
    snapshot.mkdir(mode=0o700)
    inputs = subprocess.check_output(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"]).split(b"\0")
    manifest = {"original_head": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
                "original_status": subprocess.check_output(["git", "status", "--porcelain"], text=True),
                "snapshot": str(snapshot), "files": {}, "started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    for name in sorted(set(inputs)):
        if not name: continue
        relative = Path(name.decode())
        path = source / relative
        if output == path or output in path.parents or not path.exists(): continue
        target = snapshot / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        if path.is_symlink():
            target.symlink_to(path.readlink())
            manifest["files"][str(relative)] = {"symlink": str(path.readlink())}
        else:
            shutil.copy2(path, target)
            manifest["files"][str(relative)] = {"sha256": hashlib.sha256(path.read_bytes()).hexdigest()}
    subprocess.run(["git", "init", "--quiet", str(snapshot)], check=True)
    subprocess.run(["git", "-C", str(snapshot), "add", "."], check=True)
    subprocess.run(["git", "-C", str(snapshot), "-c", "user.name=Orbit validation", "-c", "user.email=validation@localhost",
                    "commit", "--quiet", "-m", "Isolated T13 validation snapshot"], check=True)
    manifest["snapshot_commit"] = subprocess.check_output(["git", "-C", str(snapshot), "rev-parse", "HEAD"], text=True).strip()
    manifest["snapshot_tree"] = subprocess.check_output(["git", "-C", str(snapshot), "rev-parse", "HEAD^{tree}"], text=True).strip()
    (output / "snapshot-manifest.json").write_text(json.dumps(manifest, indent=2)+"\n")
    print("Isolated source snapshot: " + str(snapshot), flush=True)
    command = ["python3", "scripts/validation/reproduce_release.py", "--source", manifest["snapshot_commit"],
               "--kernel", str(kernel.resolve()), "--output", str(output / "clean-release")]
    result = subprocess.run(command, cwd=snapshot)
    if result.returncode: raise RuntimeError("clean reproduction failed; inspect retained logs")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kernel", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    run(args.kernel, args.output)
