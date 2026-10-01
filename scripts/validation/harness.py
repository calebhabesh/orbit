#!/usr/bin/env python3
"""Shared isolated host operations and measured TCP forwarding."""
import base64
from contextlib import contextmanager
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import socket
import subprocess
import threading
import time
import uuid

AGENT = Path(__file__).with_name("host_agent.py").read_text()


class Node:
    def __init__(self, host, role, purpose="validation"):
        self.host, self.role, self.root = host, role, None
        self.purpose = purpose
        self.token = uuid.uuid4().hex
        self.workers = []

    def call(self, action, **kwargs):
        req = {"action": action, "root": self.root, "token": self.token, "purpose":self.purpose, **kwargs}
        command = ["python3", "-c", AGENT]
        if self.host != "local":
            command = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", self.host, shlex.join(command)]
        proc = subprocess.run(command, input=json.dumps(req), capture_output=True, text=True, timeout=1900)
        if proc.returncode:
            raise RuntimeError(f"{self.role} {action}: {proc.stdout} {proc.stderr}")
        return json.loads(proc.stdout)

    def setup(self, folder):
        self.inventory = self.call("inventory")
        self.root = self.call("create")["root"]
        binary = Path("bin/filesync-linux-arm64" if self.inventory["arch"] == "aarch64" else "bin/filesync")
        if self.host == "local":
            self.put("filesync", binary.read_bytes(), mode=0o700)
        else:
            subprocess.run(["scp", "-q", str(binary), f"{self.host}:{self.root}/filesync"], check=True)
            self.call("run", args=["version"])
        self.put("host_agent.py", AGENT.encode())
        self.folder = folder
        self.cli("init")
        self.cli("register", "--folder", folder, "--root", self.root + "/data")
        identity = self.cli("identity", "--certificate")
        self.device = re.search(r"device=([a-f0-9]{64})", identity)[1]
        self.pin = re.search(r"key-pin=([a-f0-9]{64})", identity)[1]
        self.cert = identity[identity.index("-----BEGIN CERTIFICATE-----"):].encode()
        self.binary_hash = hashlib.sha256(binary.read_bytes()).hexdigest()

    def put(self, path, data, **kwargs):
        return self.call("put", path=path, data=base64.b64encode(data).decode(), **kwargs)

    def cli(self, *args, check=True):
        result = self.call("run", args=[*args, "--state", self.root + "/state"])
        self.last_resources = result.get("resources")
        if check and result["returncode"]:
            raise RuntimeError(f"{self.role}: {args}: {result}")
        return result["stdout"] if check else result

    def scan(self):
        return self.cli("scan", "--folder", self.folder)

    def start(self, kind, args):
        name = kind + "-" + uuid.uuid4().hex[:8]
        self.call("start", name=name, kind=kind, args=[*args, "--state", self.root + "/state"])
        self.workers.append(name)
        return name

    def serve(self):
        name = self.start("serve", ["serve", "--peer-listen", "127.0.0.1:0", "--no-watch"])
        for _ in range(200):
            result = self.call("poll", name=name)
            if result["port"]:
                return name, result["port"]
            if result["returncode"] is not None:
                raise RuntimeError("server exited: " + result["log"])
            time.sleep(0.025)
        raise RuntimeError("server readiness timeout")

    def stop(self, name):
        self.call("stop", name=name)
        self.workers.remove(name)

    def cleanup(self):
        for name in self.workers[:]:
            self.stop(name)
        # Retain roots for inspection; never delete an existing pilot folder.


def pair(nodes):
    membership = {"membership": {"Folder": nodes[0].folder, "Revision": 1,
                                  "PriorDigest": "0" * 64,
                                  "Active": [{"Device": n.device, "KeyPin": n.pin} for n in nodes],
                                  "Retired": []}}
    for node in nodes:
        node.put("membership.json", json.dumps(membership).encode())
        node.cli("membership", "import", "--folder", node.folder, "--file", node.root + "/membership.json", "--approve")
        for peer in nodes:
            node.put(peer.role + ".pem", peer.cert)


class Proxy:
    """Counts actual TLS-bearing TCP stream bytes; excludes IP/TCP/SSH headers."""
    def __init__(self, destination, bandwidth=0, latency=0):
        self.destination = destination
        self.bandwidth, self.latency = bandwidth, latency
        self.listener = socket.socket()
        self.listener.bind(("127.0.0.1", 0))
        self.listener.listen()
        self.port = self.listener.getsockname()[1]
        self.counts = [0, 0]
        self.lock = threading.Lock()
        self.connections, self.threads = [], []
        self.errors = []
        self.closed = False
        self.acceptor = threading.Thread(target=self.accept, daemon=True)
        self.acceptor.start()

    def accept(self):
        while not self.closed:
            try:
                client, _ = self.listener.accept()
                server = socket.create_connection(self.destination, timeout=15)
                server.settimeout(None)
                self.connections.extend([client, server])
                for source, target, direction in [(client, server, 0), (server, client, 1)]:
                    thread = threading.Thread(target=self.copy, args=(source, target, direction), daemon=True)
                    self.threads.append(thread)
                    thread.start()
            except OSError as error:
                if not self.closed:
                    self.errors.append(str(error))
                return

    def copy(self, source, target, direction):
        try:
            while data := source.recv(65536):
                if self.bandwidth and direction == 1:
                    time.sleep(len(data) / self.bandwidth)
                if self.latency:
                    time.sleep(self.latency)
                target.sendall(data)
                with self.lock:
                    self.counts[direction] += len(data)
        except (ConnectionError, OSError):
            pass
        finally:
            try:
                target.shutdown(socket.SHUT_WR)
            except OSError:
                pass

    def close(self):
        self.closed = True
        self.listener.close()
        for conn in self.connections:
            try:
                conn.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            conn.close()
        for thread in self.threads:
            thread.join(timeout=3)

    def metrics(self):
        with self.lock:
            return {"request_tls_tcp_bytes": self.counts[0], "response_tls_tcp_bytes": self.counts[1],
                    "total_tls_tcp_bytes": sum(self.counts), "proxy_errors": self.errors[:]}


def reserved_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


@contextmanager
def link(source, target, remote_port, bandwidth=0, latency=0):
    processes = []
    proxy = None
    try:
        port = remote_port
        if source.host != "local":
            port = reserved_port()
            command = ["ssh", "-N", "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", "-L",
                       f"127.0.0.1:{port}:127.0.0.1:{remote_port}", source.host]
            processes.append(subprocess.Popen(command, stderr=subprocess.PIPE))
            for _ in range(200):
                if processes[-1].poll() is not None:
                    raise RuntimeError("SSH local forwarding failed")
                try:
                    with socket.create_connection(("127.0.0.1", port), 0.1):
                        break
                except OSError:
                    time.sleep(0.025)
            else:
                raise RuntimeError("SSH local forwarding timeout")
        proxy = Proxy(("127.0.0.1", port), bandwidth, latency)
        target_port = proxy.port
        if target.host != "local":
            command = ["ssh", "-N", "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", "-R",
                       f"0:127.0.0.1:{proxy.port}", target.host]
            proc = subprocess.Popen(command, stderr=subprocess.PIPE, text=True)
            processes.append(proc)
            # SSH announces server allocation before accepting client work.
            import selectors
            sel = selectors.DefaultSelector()
            sel.register(proc.stderr, selectors.EVENT_READ)
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                if proc.poll() is not None:
                    raise RuntimeError("SSH reverse forwarding failed")
                if sel.select(0.2):
                    line = proc.stderr.readline()
                    match = re.search(r"Allocated port (\d+)", line)
                    if match:
                        target_port = int(match[1])
                        break
            else:
                raise RuntimeError("SSH reverse allocation timeout")
            sel.close()
        yield f"https://127.0.0.1:{target_port}", proxy
    finally:
        for proc in processes:
            proc.terminate()
            proc.wait(timeout=5)
        if proxy:
            proxy.close()


def sync(source, target, **network):
    name, port = source.serve()
    try:
        with link(source, target, port, **network) as (url, proxy):
            start = time.monotonic()
            result = json.loads(target.cli("sync", "--folder", target.folder, "--peer-url", url,
                                           "--peer-device", source.device, "--peer-certificate", target.root + "/" + source.role + ".pem", "--json"))
            return {"result": result, "seconds": time.monotonic() - start, "receiver_resources": target.last_resources, **proxy.metrics()}
    finally:
        source.stop(name)
