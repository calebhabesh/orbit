#!/usr/bin/env python3
"""Package-extracted ordinary terminal onboarding on fresh marked native roots.

No hand-built memberships/certificates, firewall/VPN policy, lingering, reboot,
or personal-pilot changes. SSH carries harness control only; Orbit uses the
selected direct network addresses. Output never includes invitation capabilities.
"""
import argparse
import base64
import hashlib
import io
import json
from pathlib import Path
import tarfile
import time
import uuid

from harness import AGENT, Node, prepare_output


def wait(label, fn, seconds=90):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = fn()
        if value:
            return value
        time.sleep(.25)
    raise RuntimeError("timeout: " + label)


def package_binary(dist, arch):
    sums = {line.split()[1].lstrip('*'): line.split()[0]
            for line in (dist / "SHA256SUMS").read_text().splitlines()}
    matches = list(dist.glob("orbit-*linux-" + arch + ".tar.gz"))
    if len(matches) != 1:
        raise RuntimeError("expected one package for " + arch)
    package = matches[0]
    raw = package.read_bytes()
    if hashlib.sha256(raw).hexdigest() != sums[package.name]:
        raise RuntimeError("package checksum mismatch")
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz") as archive:
        members = [m for m in archive.getmembers() if m.name == "filesync" and m.isfile()]
        if len(members) != 1:
            raise RuntimeError("expected one packaged filesync executable")
        binary = archive.extractfile(members[0]).read()
    return binary, {"package": package.name, "package_sha256": sums[package.name],
                    "binary_sha256": hashlib.sha256(binary).hexdigest()}


class TerminalNode(Node):
    def prepare(self, dist, address=None):
        self.inventory = self.call("inventory")
        self.root = self.call("create")["root"]
        self.put("host_agent.py", AGENT.encode())
        binary, self.provenance = package_binary(dist, "arm64" if self.inventory["arch"] == "aarch64" else "amd64")
        self.put("filesync", binary, mode=0o700)
        self.network = self.call("network")
        candidates = [v["local"] for interface in self.network["addresses"]
                      if interface["operstate"] == "UP" and not interface["ifname"].startswith(("docker", "br-"))
                      for v in interface["addr_info"] if v["scope"] == "global"]
        self.address = address or candidates[0]
        if self.address.startswith("127."):
            raise RuntimeError("direct native campaign requires nonloopback address")
        ports = []
        while len(set(ports)) < 2:
            ports.append(self.call("reserve-port", address=self.address)["port"])
            ports = list(dict.fromkeys(ports))
        self.settings = dict(data_budget="1073741824", metadata_budget="268435456", reserve_bytes="16777216",
                             retention_seconds="0", concurrency="4", bandwidth_bytes_per_second="0",
                             peer_listen=f"{self.address}:{ports[0]}", enrollment_listen=f"{self.address}:{ports[1]}",
                             advertised_peer=f"{self.address}:{ports[0]}", advertised_enrollment=f"{self.address}:{ports[1]}", startup="manual")
        self.put("settings.json", json.dumps(self.settings).encode())
        self.cli("init")
        self.device = json.loads(base64.b64decode(self.call("read", path="state/config.json")["data"]))["device_id"]

    def orbit(self, *args, check=True):
        return self.cli("orbit", *args, check=check)

    def query(self, kind, **fields):
        return self.call("terminal-query", query={"kind": kind, **fields})

    def restart(self):
        self.call("terminal-stop")
        # Old worker records belong to exited processes; roots remain retained.
        self.workers.clear()
        self.start("serve", ["serve", "--control-listen", "127.0.0.1:0", "--sync-interval", "1s"])
        wait("live owner control", lambda: self.live(), 15)

    def live(self):
        try:
            return self.query("capabilities")
        except RuntimeError:
            return False

    def setup(self, relative="data", invitation=None):
        label = self.role.capitalize()
        review = relative + "-review.json"
        args = ["join" if invitation else "setup", "--root", self.root + "/" + relative,
                "--label", label, "--name", "Notes" if relative == "data" else relative,
                "--settings-file", self.root + "/settings.json", "--preview", "--review-file", self.root + "/" + review, "--json"]
        if invitation:
            self.put(relative + "-invitation.txt", invitation.encode())
            args += ["--invitation-file", self.root + "/" + relative + "-invitation.txt"]
        preview = json.loads(self.orbit(*args))
        if not preview["preview"]["complete"]:
            raise RuntimeError("incomplete adoption preview")
        result = json.loads(self.orbit(args[0], "--request-file", self.root + "/" + review,
                                      "--timeout", "0", "--json"))
        self.restart()
        return result

    def cleanup(self):
        if self.root:
            self.call("terminal-stop")
            self.workers.clear()


def approve(owner, receiver, pending):
    request = pending["join"]["request"]
    items = owner.query("requests", limit="20")["requests"]
    item = next(x for x in items if x["id"] == request)
    review = {"request": request, "folder": item["folder"], "requester": item["requester"],
              "key_pin": item["key_pin"], "transcript_digest": item["transcript_digest"],
              "expected_membership": item["expected_membership"], "decision": "approve"}
    if item["requester"] != receiver.device:
        raise RuntimeError("approval identity differs")
    # Exact private review comes from controller observations, never synthesized membership.
    path = "approval-" + uuid.uuid4().hex + ".json"
    owner.put(path, json.dumps(review).encode())
    owner.orbit("requests", "approve", "--request", request, "--review-file", owner.root + "/" + path, "--json")
    operation = pending["operation"]["id"]
    last = {}
    def done():
        result = receiver.query("operation", id=operation)
        last["result"] = result
        return result if result["state"] == "completed" else False
    # The joiner polls status every 15 s within the inviter's per-source budget
    # (5 requests/minute); co-located rehearsal hosts share one source address,
    # so allow several polls on slow shared runners.
    try:
        completed = wait("approved durable join", done, seconds=180)
    except RuntimeError as error:
        # Phase and error code only: no invitation, pin or path material.
        r = last.get("result") or {}
        op, err = r.get("operation") or {}, r.get("error") or (r.get("operation") or {}).get("error") or {}
        raise RuntimeError(f"{error}: state={r.get('state')} phase={op.get('phase')} "
                           f"error={err.get('code')} retryable={err.get('retryable')} "
                           f"message={err.get('message')!r}") from None
    if completed["join"]["request"] != request or completed["join"]["attempt"] != pending["join"]["attempt"]:
        raise RuntimeError("join restart changed request/attempt")
    return {"request": request, "attempt": completed["join"]["attempt"], "operation": operation,
            "verification_code": item["verification_code"], "readiness": completed["readiness"]}


def equal_bytes(nodes, relative, value):
    digest = hashlib.sha256(value).hexdigest()
    def matches():
        try:
            values = {n.role: n.call("hash", path=relative)["sha256"] for n in nodes}
            return values if set(values.values()) == {digest} else False
        except RuntimeError:
            return False
    return {"expected_sha256": digest, "observed": wait("working bytes " + relative, matches)}


def run(dist, hosts, output, addresses=None):
    prepare_output(output)
    nodes = [TerminalNode(host, role) for host, role in zip(hosts, ("laptop", "pi"))]
    report = {"started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "success": False,
              "type": "automated direct-network packaged terminal journey", "personal_use": False,
              "hosts": [], "scenarios": {}, "limitations": ["No login/logout/boot or lingering changes", "No Tailscale claim unless supplied existing addresses route over it", "Two native participants; third-host engine campaign is separate"]}
    started = time.monotonic()
    try:
        for i, n in enumerate(nodes):
            n.prepare(dist, addresses[i] if addresses else None)
            report["hosts"].append({"host": n.host, "root": n.root, "device": n.device,
                                    "inventory": n.inventory, "network": n.network, "settings": n.settings, **n.provenance})
        a, b = nodes
        original = b"reviewed existing laptop bytes\n"
        local = b"reviewed existing Pi bytes\n"
        a.put("data/original", original); b.put("data/pi-local", local)
        created = a.setup()
        folder = created["join"]["folder"]
        invitation = json.loads(a.orbit("invite", "create", "--json"))["invitation_code"]
        pending = b.setup(invitation=invitation)
        if not pending["join"]["request"] or pending["state"] == "completed":
            raise RuntimeError("approval was bypassed")
        # Both actual daemons restart before delayed exact-request approval.
        a.restart(); b.restart()
        report["scenarios"]["delayed_approval_restart"] = approve(a, b, pending)
        report["scenarios"]["existing_bytes"] = [equal_bytes(nodes, "original", original), equal_bytes(nodes, "pi-local", local)]
        for n in nodes:
            value = (n.role + " ordinary edit\n").encode()
            n.put("data/" + n.role + "-edit", value)
            report["scenarios"][n.role + "_edit"] = equal_bytes(nodes, n.role + "-edit", value)
        # Joining the additional folder uses the same persistent device and key.
        a.put("second/second-only", b"second folder owner bytes\n")
        b.put("second/local-second", b"second folder receiver bytes\n")
        second = a.setup("second")
        second_folder = second["join"]["folder"]
        management = a.query("folder_management", folder=second_folder)["folder_management"]
        mutation = {"version": "1", "kind": "share", "operation_id": uuid.uuid4().hex * 2,
                    "invite": {"folder": second_folder, "device": b.device,
                               "expected_membership": management["membership_digest"],
                               "expires_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time()+3600))}}
        a.put("share.json", json.dumps(mutation).encode())
        invitation = json.loads(a.orbit("folders", "share", "--request-file", a.root + "/share.json", "--json"))["invitation"]
        code = "orbit-invitation:v2:" + base64.urlsafe_b64encode(json.dumps(invitation).encode()).decode().rstrip("=")
        joined = b.setup("second", code)
        second_receipt = approve(a, b, joined)
        if second_receipt["request"] == pending["join"]["request"]:
            raise RuntimeError("same device request collision")
        report["scenarios"]["second_folder"] = second_receipt
        # hash action's base is data; use read for the independently registered root.
        def second_equal():
            return all(base64.b64decode(n.call("read", path="second/"+path)["data"]) == value
                       for n in nodes for path, value in (("second-only", b"second folder owner bytes\n"), ("local-second", b"second folder receiver bytes\n")))
        wait("second folder actual bytes", second_equal)
        report["scenarios"]["second_folder"]["both_existing_files_preserved"] = True
        b.call("terminal-stop")
        a.put("data/offline", b"while Pi is offline\n")
        # Saved local evidence must precede reconnect.
        wait("offline local capture", lambda: a.query("history", folder=folder, path="offline").get("versions"))
        b.restart()
        report["scenarios"]["offline_reconnect"] = equal_bytes(nodes, "offline", b"while Pi is offline\n")
        report["scenarios"]["qualified_status"] = {n.role: json.loads(n.orbit("status", "--json")) for n in nodes}
        report["scenarios"]["keys_membership_heads"] = {}
        for n in nodes:
            cfg = json.loads(base64.b64decode(n.call("read", path="state/config.json")["data"]))
            if cfg["device_id"] != n.device:
                raise RuntimeError("identity changed")
            membership = n.query("folder_management", folder=folder)["folder_management"]
            versions = {path: n.query("history", folder=folder, path=path)["versions"]
                        for path in ("original", "pi-local", "laptop-edit", "pi-edit", "offline")}
            report["scenarios"]["keys_membership_heads"][n.role] = {"device": n.device, "membership": membership, "versions": versions}
        facts = report["scenarios"]["keys_membership_heads"]
        if facts[a.role]["membership"]["membership_digest"] != facts[b.role]["membership"]["membership_digest"]:
            raise RuntimeError("membership differs")
        for path in facts[a.role]["versions"]:
            ids = [{json.dumps(v["version"], sort_keys=True) for v in facts[n.role]["versions"][path]} for n in nodes]
            if ids[0] != ids[1] or len(ids[0]) != 1:
                raise RuntimeError("head identity differs: " + path)
        report["success"] = True
    finally:
        failures = []
        for n in nodes:
            try: n.cleanup()
            except Exception as error: failures.append(str(error))
        report["cleanup_errors"] = failures
        report["seconds"] = time.monotonic() - started
        report["ended_utc"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        if failures: report["success"] = False
        (output / "terminal-native.json").write_text(json.dumps(report, indent=2)+"\n")
        if failures: raise RuntimeError("owned daemon cleanup failed: " + "; ".join(failures))
    print("PASS direct packaged terminal onboarding, restart, existing files, second-folder sharing and edits", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", type=Path, default=Path("dist"))
    parser.add_argument("--hosts", nargs=2, default=["laptop", "rpi"])
    parser.add_argument("--addresses", nargs=2, help="existing reachable LAN or Tailscale IPs; never changes network policy")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    run(args.dist, args.hosts, args.output, args.addresses)
