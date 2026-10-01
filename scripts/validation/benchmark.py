#!/usr/bin/env python3
"""Measured TLS/TCP traffic against a verified, durable full-file HTTPS baseline."""
import argparse
import concurrent.futures
import hashlib
import http.client
import http.server
import json
import os
from pathlib import Path
import random
import ssl
import socket
import statistics
import threading
import time
import urllib.parse
import uuid

from harness import Node, Proxy, pair, sync


def digest(path):
    result = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def inventory(root):
    return {p.relative_to(root).as_posix(): {"sha256": digest(p), "size": p.stat().st_size}
            for p in sorted(root.rglob("*")) if p.is_file() and ".filesync-internal" not in p.parts}


def flush_dir(path):
    fd = os.open(path, os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


class FullFileBaseline:
    def __init__(self, source, receiver, resume=False):
        self.source, self.receiver = source, receiver
        self.root = Path(source.root) / "data"
        self.destination = Path(receiver.root) / "baseline"
        self.destination.mkdir(exist_ok=resume)
        self.manifest, self.payload_bytes = {}, 0
        self.lock = threading.Lock()
        baseline = self

        class Handler(http.server.BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def setup(self):
                self.request.setsockopt(socket.IPPROTO_TCP,socket.TCP_NODELAY,1)
                super().setup()

            def log_message(self, *args):
                pass

            def do_GET(self):
                if self.path == "/manifest":
                    data = json.dumps(baseline.manifest, separators=(",", ":")).encode()
                    self.send_response(200)
                    self.send_header("Content-Length", str(len(data)))
                    self.end_headers()
                    self.wfile.write(data)
                    return
                name = urllib.parse.unquote(self.path.removeprefix("/file/"))
                if name not in baseline.manifest:
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Content-Length", str(baseline.manifest[name]["size"]))
                self.end_headers()
                with (baseline.root / name).open("rb") as data:
                    for chunk in iter(lambda: data.read(65536), b""):
                        self.wfile.write(chunk)
                        with baseline.lock:
                            baseline.payload_bytes += len(chunk)

        http.server.ThreadingHTTPServer.daemon_threads = True
        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        server_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        server_ctx.minimum_version = ssl.TLSVersion.TLSv1_3
        server_ctx.maximum_version = ssl.TLSVersion.TLSv1_3
        server_ctx.load_cert_chain(source.root + "/state/identity/peer-identity.pem")
        server_ctx.load_verify_locations(cadata=receiver.cert.decode())
        server_ctx.verify_mode = ssl.CERT_REQUIRED
        self.server.socket = server_ctx.wrap_socket(self.server.socket, server_side=True)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.context = ssl.create_default_context(cadata=source.cert.decode())
        self.context.minimum_version = ssl.TLSVersion.TLSv1_3
        self.context.maximum_version = ssl.TLSVersion.TLSv1_3
        self.context.load_cert_chain(receiver.root + "/state/identity/peer-identity.pem")
        # Pinned trust root, same server-name validation as File Sync.

    def connect(self, port):
        class Connection(http.client.HTTPSConnection):
            def connect(conn):
                import socket
                sock = socket.create_connection(("127.0.0.1", port), timeout=30)
                sock.setsockopt(socket.IPPROTO_TCP,socket.TCP_NODELAY,1)
                conn.sock = self.context.wrap_socket(sock, server_hostname="peer.filesync.invalid")
        return Connection("peer.filesync.invalid", port, context=self.context)

    def run(self, bandwidth=0, latency=0):
        proxy = Proxy(self.server.server_address, bandwidth, latency)
        start = time.monotonic()
        self.payload_bytes = 0
        try:
            self.manifest = inventory(self.root)
            conn = self.connect(proxy.port)
            conn.request("GET", "/manifest")
            response = conn.getresponse()
            if response.status != 200:
                raise RuntimeError("baseline manifest failed")
            manifest = json.loads(response.read())
            conn.close()
            current = inventory(self.destination)
            changed = [name for name, record in manifest.items() if current.get(name) != record]

            def receive(names):
                connection = self.connect(proxy.port)
                try:
                    for name in names:
                        record = manifest[name]
                        connection.request("GET", "/file/" + urllib.parse.quote(name))
                        response = connection.getresponse()
                        if response.status != 200:
                            raise RuntimeError("baseline payload failed")
                        target = self.destination / name
                        target.parent.mkdir(parents=True, exist_ok=True)
                        temporary = target.with_name(target.name + ".incoming")
                        whole = hashlib.sha256()
                        count = 0
                        with temporary.open("wb") as handle:
                            while data := response.read(65536):
                                whole.update(data)
                                count += len(data)
                                handle.write(data)
                            handle.flush()
                            os.fsync(handle.fileno())
                        if count != record["size"] or whole.hexdigest() != record["sha256"]:
                            raise RuntimeError("baseline whole-file verification failed")
                        temporary.replace(target)
                        flush_dir(target.parent)
                        parent = target.parent
                        while parent != self.destination:
                            flush_dir(parent.parent)
                            parent = parent.parent
                        flush_dir(self.destination)
                finally:
                    connection.close()

            with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                list(pool.map(receive, [changed[i::4] for i in range(min(4, len(changed)))]))
            for name in current.keys() - manifest.keys():
                target = self.destination / name
                target.unlink()
                flush_dir(target.parent)
            elapsed = time.monotonic() - start
            if inventory(self.destination) != manifest:
                raise RuntimeError("baseline resulting tree differs")
            return {"seconds": elapsed, "payload_bytes": self.payload_bytes, **proxy.metrics()}
        finally:
            proxy.close()

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=3)


def write_random(path, size, seed):
    randomizer = random.Random(seed)
    with path.open("wb") as out:
        remaining = size
        while remaining:
            count = min(1024 * 1024, remaining)
            out.write(randomizer.randbytes(count))
            remaining -= count


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--repetitions", type=int, default=3)
    parser.add_argument("--small-files", type=int, default=1000)
    parser.add_argument("--large-mib", type=int, default=1024)
    parser.add_argument("--resume-after-initial", action="store_true",
                        help="resume an interrupted one-repetition campaign after its completed initial workload")
    args = parser.parse_args()
    if args.repetitions < 1 or args.small_files < 1 or args.large_mib < 1:
        parser.error("positive workload dimensions required")
    args.output.mkdir(parents=True, exist_ok=True)
    report = {"type": "synthetic", "seed": 20261001, "success": False, "runs": [],
              "measurement": "TCP stream bytes in both directions INCLUDING TLS records/handshake; EXCLUDING IP/TCP and SSH headers",
              "cache": "fresh application stores per repetition; warm OS page cache, no privileged cache drops",
              "tcp": "TCP_NODELAY enabled in both proxy directions and baseline sockets, matching Go TCP defaults; avoids proxy-introduced delayed-ACK/Nagle stalls",
              "storage_limits": "Fresh File Sync states use init's finite 10-GiB data, 256-MiB metadata and 512-MiB free-space reserve defaults; raw receiver totals are recorded",
              "baseline": "TLS 1.3 mutual auth, full SHA256 on both sides, flush received bytes, atomic replace + directory fsync, four workers; unchanged files skipped by full hash",
              "timing": "sender scan/hash + sync to durable receipt/publication; baseline source/destination hash + durable publication. Server/tunnel startup and final comparison excluded",
              "limitations": "Python baseline vs Go engine; baseline retains only working tree, engine additionally commits history/content/journals. No equivalent CPU-work or generic speedup claim",
              "hosts": [], "summaries": []}
    if args.resume_after_initial:
        report = json.loads((args.output / "benchmarks.json").read_text())
        if args.repetitions != 1 or len(report["hosts"]) != 1 or len(report["runs"]) != 1 or \
                report["runs"][0]["workload"] != "small_files_initial" or not report["runs"][0]["success"]:
            raise RuntimeError("resume requires exactly one completed initial workload and no later mutation")
        report["campaign_interruption"] = "orchestrator turn interrupted after initial workload; preserved roots and bytes checked before continuation"
    try:
        for repetition in range(args.repetitions):
            nodes = [Node("local", "source"), Node("local", "receiver")]
            baseline = None
            try:
                folder = uuid.uuid4().hex * 2
                if args.resume_after_initial:
                    for node, saved_root in zip(nodes, report["hosts"][0]["roots"]):
                        node.root = saved_root
                        node.token = (Path(saved_root) / ".filesync-disposable").read_text().strip()
                        node.inventory = node.call("inventory")
                        # Reap only orphaned workers from this exact marked run.
                        for pidfile in Path(saved_root).glob("*.pid.json"):
                            name = pidfile.name.removesuffix(".pid.json")
                            node.call("stop", name=name)
                        identity = node.cli("identity", "--certificate")
                        import re
                        node.device = re.search(r"device=([a-f0-9]{64})", identity)[1]
                        node.pin = re.search(r"key-pin=([a-f0-9]{64})", identity)[1]
                        node.cert = identity[identity.index("-----BEGIN CERTIFICATE-----"):].encode()
                        node.binary_hash = hashlib.sha256((Path(saved_root)/"filesync").read_bytes()).hexdigest()
                        if node.binary_hash != report["hosts"][0]["binary_sha256"]:
                            raise RuntimeError("resume binary differs from measured initial workload")
                        c = __import__("sqlite3").connect(f"file:{saved_root}/state/metadata.sqlite?mode=ro",uri=True)
                        try:
                            folder = c.execute("select lower(hex(folder_id)) from folders").fetchone()[0]
                        finally:
                            c.close()
                        node.folder = folder
                else:
                    for node in nodes:
                        node.setup(folder)
                source, receiver = nodes
                if not args.resume_after_initial:
                    pair(nodes)
                baseline = FullFileBaseline(source, receiver, resume=args.resume_after_initial)
                root = Path(source.root) / "data"
                if not args.resume_after_initial:
                    report["hosts"].append({"repetition": repetition, "roots": [n.root for n in nodes],
                                       "inventory": source.inventory, "binary_sha256": source.binary_hash})
                else:
                    expected = inventory(root)
                    if inventory(Path(receiver.root)/"data") != expected or inventory(baseline.destination) != expected or \
                            hashlib.sha256(json.dumps(expected,sort_keys=True).encode()).hexdigest() != report["runs"][0]["workload_digest"]:
                        raise RuntimeError("resume fixture differs from completed initial workload")

                def measure(name, bandwidth=0, latency=0):
                    entry = {"repetition": repetition, "workload": name, "bandwidth_bytes_second": bandwidth,
                             "proxy_delay_seconds_per_64k_read_each_direction": latency, "success": False}
                    report["runs"].append(entry)
                    try:
                        start = time.monotonic()
                        source.scan()
                        scan_seconds = time.monotonic() - start
                        entry["filesync"] = sync(source, receiver, bandwidth=bandwidth, latency=latency)
                        entry["filesync"]["scan_seconds"] = scan_seconds
                        entry["receiver_storage_bytes"] = sum(p.stat().st_size for p in (Path(receiver.root) / "state").rglob("*") if p.is_file())
                        entry["filesync"]["total_seconds"] = entry["filesync"]["seconds"] + scan_seconds
                        entry["baseline"] = baseline.run(bandwidth, latency)
                        expected = inventory(root)
                        if inventory(Path(receiver.root) / "data") != expected:
                            raise RuntimeError("File Sync resulting tree differs")
                        entry["file_count"] = len(expected)
                        entry["source_payload_bytes"] = sum(v["size"] for v in expected.values())
                        entry["workload_digest"] = hashlib.sha256(json.dumps(expected, sort_keys=True).encode()).hexdigest()
                        fs_bytes, full_bytes = entry["filesync"]["total_tls_tcp_bytes"], entry["baseline"]["total_tls_tcp_bytes"]
                        entry["tls_tcp_savings_percent"] = 100 * (1 - fs_bytes / full_bytes)
                        entry["success"] = True
                        print(f"PASS {repetition + 1} {name}: FileSync={fs_bytes} baseline={full_bytes} TLS/TCP bytes", flush=True)
                    except Exception as error:
                        entry["error"] = str(error)
                        raise
                    finally:
                        (args.output / "benchmarks.json").write_text(json.dumps(report, indent=2) + "\n")

                if not args.resume_after_initial:
                    rng = random.Random(20261001)
                    for i in range(args.small_files):
                        path = root / f"d{i % 10}" / f"s{i % 7}" / f"note-{i}.bin"
                        path.parent.mkdir(parents=True, exist_ok=True)
                        path.write_bytes(rng.randbytes(rng.randrange(4096, 65537)))
                    measure("small_files_initial")
                measure("unchanged_tree")
                large = root / "archive.bin"
                write_random(large, args.large_mib * 1024 * 1024, 17)
                measure("large_initial")
                with large.open("r+b") as out:
                    out.seek(-4096, os.SEEK_END)
                    out.write(b"E" * 4096)
                measure("tail_overwrite")
                with large.open("ab") as out:
                    out.write(b"A" * 4096)
                measure("append")
                shifted = root / "prefix.bin"
                write_random(shifted, 20 * 1024 * 1024, 19)
                measure("mixed_initial")
                shifted.write_bytes(b"X" + shifted.read_bytes())
                measure("prefix_insert", bandwidth=8 * 1024 * 1024, latency=0.002)
                large.rename(root / "renamed-archive.bin")
                measure("rename")
                shifted.unlink()
                measure("delete")
            finally:
                if baseline:
                    baseline.close()
                for node in nodes:
                    if node.root:
                        node.cleanup()
        for workload in sorted({run["workload"] for run in report["runs"]}):
            runs = [r for r in report["runs"] if r["workload"] == workload]
            report["summaries"].append({"workload": workload, "samples": len(runs),
                "median_tls_tcp_savings_percent": statistics.median(r["tls_tcp_savings_percent"] for r in runs),
                "median_filesync_seconds": statistics.median(r["filesync"]["total_seconds"] for r in runs),
                "median_baseline_seconds": statistics.median(r["baseline"]["seconds"] for r in runs)})
        report["success"] = True
    finally:
        (args.output / "benchmarks.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
