#!/usr/bin/env python3
"""Power-cut ONLY newly created marked VM images. No host mount or root required."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import shutil
import subprocess
import tempfile
import time

HOOKS = ["dirty-cache-control", "object.flushed", "object.installed", "object.recorded",
         "sql.version.before_commit", "sql.version.after_commit", "publication.prepared",
         "publication.stage.flushed", "publication.staged", "publication.intent",
         "publication.exchange.before", "publication.filesystem.transition",
         "publication.recovery.named", "publication.directory.flushed",
         "publication.renamed", "publication.committed"]


def newc(name, data, mode, ino):
    fields = [ino, mode, 0, 0, 1, 0, len(data), 0, 0, 0, 0, len(name) + 1, 0]
    header = b"070701" + b"".join(f"{v:08x}".encode() for v in fields)
    head = header + name.encode() + b"\0"
    head += b"\0" * (-len(head) % 4)
    return head + data + b"\0" * (-len(data) % 4)


def validate(root, disk):
    if root.is_symlink() or disk.is_symlink() or disk.parent != root:
        raise RuntimeError("unsafe VM image target")
    if (root / ".filesync-disposable").read_text() != "filesync disposable reset VM\n":
        raise RuntimeError("missing disposable VM marker")


def boot(root, disk, kernel, mode, hook, logs):
    validate(root, disk)
    args = ["qemu-system-x86_64", "-accel", "kvm", "-m", "384", "-smp", "2",
            "-nodefaults", "-nographic", "-serial", "stdio", "-no-reboot",
            "-kernel", str(kernel), "-initrd", str(root / "initramfs.cpio"),
            "-append", f"console=ttyS0 rdinit=/init panic=-1 filesync.mode={mode} filesync.hook={hook}",
            "-drive", f"file={disk},format=raw,if=virtio,cache=none"]
    proc = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    sel = selectors.DefaultSelector()
    sel.register(proc.stdout, selectors.EVENT_READ)
    output = bytearray()
    token = {"setup": "FILESYNC_RESET_SETUP_OK", "mutate": "FILESYNC_RESET_READY " + hook,
             "verify": "FILESYNC_RESET_VERIFY_OK"}[mode].encode()
    started = time.monotonic()
    try:
        while time.monotonic() - started < 45:
            for key, _ in sel.select(0.5):
                data = os.read(key.fileobj.fileno(), 65536)
                if not data:
                    raise RuntimeError("VM exited before expected boundary")
                output.extend(data)
            if b"FILESYNC_RESET_FAIL" in output and b"\n" in output[output.index(b"FILESYNC_RESET_FAIL"):]:
                raise RuntimeError(output.decode(errors="replace")[-3000:])
            if token in output and b"\n" in output[output.index(token):]:
                return output.decode(errors="replace"), time.monotonic() - started
        raise RuntimeError("VM boundary timeout: " + output.decode(errors="replace")[-3000:])
    finally:
        # This PID is the child just created here; caches in guest RAM are lost.
        validate(root, disk)
        proc.kill()
        proc.wait(timeout=5)
        sel.close()
        logs.write_text(output.decode(errors="replace"))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kernel", required=True, type=Path, help="Linux kernel with built-in virtio/ext4")
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--hook", action="append", help="subset (default: entire boundary matrix)")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    report = {"fault": "SIGKILL of dedicated QEMU VM, then cold guest boot; no clean guest unmount",
              "disk": "new raw ext4 image, virtio-blk cache=none; host storage remains running",
              "limitations": "Guest dirty caches lost; no physical power loss, host cache loss or Pi hardware claim",
              "kernel_sha256": hashlib.sha256(args.kernel.read_bytes()).hexdigest(),
              "guest_init_sha256": None, "qemu": subprocess.check_output(["qemu-system-x86_64", "--version"], text=True).splitlines()[0],
              "started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "runs": [], "success": False}
    with tempfile.TemporaryDirectory(prefix="filesync-reset-") as temp:
        root = Path(temp)
        (root / ".filesync-disposable").write_text("filesync disposable reset VM\n")
        init = root / "init"
        subprocess.run(["go", "build", "-trimpath", "-o", str(init), "scripts/validation/reset_guest.go"],
                       env={**os.environ, "CGO_ENABLED": "0"}, check=True)
        report["guest_init_sha256"] = hashlib.sha256(init.read_bytes()).hexdigest()
        (root / "initramfs.cpio").write_bytes(newc("init", init.read_bytes(), 0o100755, 1)
                                            + newc("TRAILER!!!", b"", 0, 2))
        seed = root / "seed"
        seed.mkdir()
        shutil.copy(root / ".filesync-disposable", seed)
        for index, hook in enumerate(args.hook or HOOKS):
            disk = root / f"disk-{index}.raw"
            with disk.open("wb") as handle:
                handle.truncate(256 * 1024 * 1024)
            validate(root, disk)
            subprocess.run(["mkfs.ext4", "-q", "-F", "-d", str(seed), str(disk)], check=True)
            entry = {"hook": hook, "success": False}
            report["runs"].append(entry)
            try:
                for mode in ["setup", "mutate", "verify"]:
                    text, elapsed = boot(root, disk, args.kernel, mode, hook, args.output / f"{hook}-{mode}.log")
                    entry[mode + "_seconds"] = elapsed
                    if mode == "verify":
                        entry["oracle"] = text[text.index("FILESYNC_RESET_VERIFY_OK"):].strip()
                entry["success"] = True
                print(f"PASS abrupt reset: {hook}", flush=True)
            finally:
                (args.output / "abrupt-reset.json").write_text(json.dumps(report, indent=2) + "\n")
            validate(root, disk)
            disk.unlink()
    report["success"] = all(run["success"] for run in report["runs"])
    (args.output / "abrupt-reset.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
