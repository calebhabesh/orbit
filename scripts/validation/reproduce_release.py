#!/usr/bin/env python3
"""Check an explicit committed revision in a fresh checkout; retain all logs."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import time
import uuid


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", default="HEAD")
    parser.add_argument("--kernel", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        parser.error("output must be empty; preserve previous evidence")
    source = subprocess.check_output(["git", "rev-parse", args.source + "^{commit}"], text=True).strip()
    parent = Path(tempfile.mkdtemp(prefix="filesync-reproduction-"))
    marker = parent / ".filesync-disposable"
    marker.write_text(uuid.uuid4().hex)
    marker.chmod(0o600)
    checkout = parent / "checkout"
    subprocess.run(["git", "clone", "--quiet", "--no-hardlinks", str(Path.cwd()), str(checkout)], check=True)
    subprocess.run(["git", "-C", str(checkout), "checkout", "--quiet", "--detach", source], check=True)
    report = {"source_commit": source, "checkout": str(checkout), "started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
              "checks": [], "success": False, "cleanup": "fresh marked checkout retained; no existing environment reset"}

    def run(name, command, cwd=checkout):
        started = time.monotonic()
        with (output / (name + ".log")).open("w") as log:
            result = subprocess.run(command, cwd=cwd, stdout=log, stderr=subprocess.STDOUT)
        report["checks"].append({"name": name, "command": command, "cwd": str(cwd), "exit_code": result.returncode,
                                  "seconds": time.monotonic() - started})
        print(("PASS " if result.returncode == 0 else "FAIL ") + name, flush=True)
        if result.returncode:
            raise RuntimeError(name + " failed; see retained log")

    try:
        run("status", ["git", "status", "--porcelain"])
        if (output / "status.log").read_text():
            raise RuntimeError("initial checkout is dirty")
        run("check", ["make", "check"])
        run("race", ["go", "test", "-race", "-count=1", "./..."])
        run("demo", ["make", "demo"])
        run("reset", ["python3", "scripts/validation/abrupt_reset.py", "--kernel", str(args.kernel.resolve()), "--output", str(output / "reset")])
        disk = ["python3", "scripts/validation/abrupt_reset.py", "--kernel", str(args.kernel.resolve()), "--output", str(output / "disk-full")]
        for hook in ["object", "sqlite", "checkpoint", "staging", "fsync"]:
            disk.extend(["--hook", "enospc." + hook])
        run("disk-full", disk)
        run("checksum", ["sha256sum", "-c", "SHA256SUMS"], checkout / "dist")
        report["artifact_sha256"] = {p.name: digest(p) for p in sorted((checkout / "dist").iterdir()) if p.is_file()}
        report["binary_version"] = subprocess.check_output([str(checkout / "bin/filesync"), "version"], text=True)
        report["orbit_version"] = subprocess.check_output([str(checkout / "bin/orbit"), "version"], text=True)
        run("package-repeat", ["make", "package"])
        report["repeat_artifact_sha256"] = {p.name: digest(p) for p in sorted((checkout / "dist").iterdir()) if p.is_file()}
        report["packages_identical_on_repeat"] = report["artifact_sha256"] == report["repeat_artifact_sha256"]
        if not report["packages_identical_on_repeat"]:
            raise RuntimeError("repeated packages differ")
        report["final_tracked_changes"] = subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=no"], cwd=checkout, text=True)
        if report["final_tracked_changes"]:
            raise RuntimeError("verification changed tracked source")
        report["success"] = True
    finally:
        (output / "reproduction.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
