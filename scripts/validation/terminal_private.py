#!/usr/bin/env python3
"""Ordinary packaged three-installation journey over an existing private route.

SSH controls fresh marked roots; Orbit uses direct configured network sockets.
No network policy, credentials, personal services, lingering or reboot changes.
Retirement uses real CLI previews and an unmodified CLI-exported membership bundle.
"""
import argparse
import hashlib
import json
from pathlib import Path
import time
import uuid

from harness import prepare_output
from terminal_native import TerminalNode, approve, equal_bytes, wait


class PrivateNode(TerminalNode):
    def query(self, kind, **fields):
        # The worker's 8s observation deadline can expire behind a daemon-owned
        # 10s bootstrap transaction. Retry that read once; authorization or
        # other errors still fail immediately, and completion needs real facts.
        for attempt in range(2):
            try:
                return super().query(kind, **fields)
            except RuntimeError as error:
                if attempt or '"error": "timed out"' not in str(error):
                    raise
                time.sleep(.25)

    def orbit(self, *args, check=True):
        result = super().orbit(*args, check=False)
        if not check:
            return result
        if result["returncode"] == 0:
            return result["stdout"]
        # Multiple installs sharing an IP may hit the real enrollment token budget.
        # Observe the already-admitted durable job's retry; never mint another attempt.
        if args[0] in ("setup", "join") and "--request-file" in args:
            admitted = json.loads(result["stdout"])
            if admitted.get("error", {}).get("message") == "RATE_LIMITED":
                operation = admitted["operation"]["id"]
                self.restart()
                def submitted():
                    current = self.query("operation", id=operation)
                    error = current.get("error")
                    if error and not error.get("retryable"):
                        raise RuntimeError(self.role + ": retained join requires explicit recovery: " + error["code"])
                    if current["operation"]["phase"] != "awaiting_approval":
                        return False
                    if current["join"]["attempt"] != admitted["join"]["attempt"] or (
                            admitted["join"]["request"] and current["join"]["request"] != admitted["join"]["request"]):
                        raise RuntimeError("rate-limit recovery changed request/attempt")
                    return current
                print(self.role + ": observing retained rate-limited join until submission", flush=True)
                return json.dumps(wait("retained enrollment after rate limit", submitted, 90))
        raise RuntimeError(self.role + ": CLI failed: " + result["stderr"])


def membership(nodes, folder, devices):
    def agreed():
        facts = {n.role: n.query("folder_management", folder=folder)["folder_management"] for n in nodes}
        if any({m["id"] for m in f["members"]} != set(devices) for f in facts.values()):
            return False
        return facts if len({f["membership_digest"] for f in facts.values()}) == 1 else False
    return wait("exact active membership and digest", agreed)


def histories(nodes, folder, paths):
    def agreed():
        facts = {n.role: {p: n.query("history", folder=folder, path=p)["versions"] for p in paths} for n in nodes}
        for path in paths:
            versions = [{json.dumps(v["version"], sort_keys=True) for v in f[path]} for f in facts.values()]
            if not versions[0] or any(v != versions[0] for v in versions):
                return False
        return facts
    return wait("equivalent immutable version histories", agreed)


def invitation(node, folder):
    return json.loads(node.orbit("invite", "create", "--folder", folder, "--json"))["invitation_code"]


def run(dist, hosts, addresses, network, output, preflight_only=False):
    prepare_output(output)
    nodes = [PrivateNode(h, r) for h, r in zip(hosts, ("source", "receiver", "forwarder"))]
    report = {"started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "success": False,
              "network": network, "hosts": [], "preflight": [], "scenarios": {}, "personal_use": False,
              "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              "limitations": ["No login/logout/boot validation", "No network or host policy changes",
                              "Replacement is a fresh installation on the receiver's host"]}
    started = time.monotonic()
    replacement = None
    try:
        for i, n in enumerate(nodes):
            facts = n.call("network-preflight", addresses=addresses)
            report["preflight"].append({"host": n.host, "role": n.role, **facts})
        for i, n in enumerate(nodes):
            facts = report["preflight"][i]
            if network == "tailscale":
                ts = facts["tailscale"]
                if not ts["authenticated"] or addresses[i] not in ts.get("addresses", []):
                    raise RuntimeError(n.role + ": authenticated Tailscale address unavailable")
                if any(facts["routes"][a][0].get("dev") != "tailscale0" for a in addresses if a != addresses[i]):
                    raise RuntimeError(n.role + ": peer route does not use tailscale0")
        if preflight_only:
            report["preflight_success"] = True
            report["unexecuted"] = "Preflight only; no installations or journey executed"
            return
        for n, address in zip(nodes, addresses):
            n.prepare(dist, address)
            report["hosts"].append({"host": n.host, "role": n.role, "root": n.root, "device": n.device,
                                    "inventory": n.inventory, "network": n.network, "settings": n.settings, **n.provenance})
        report["physical_host_count"] = len({n.inventory["hostname"] for n in nodes})
        if report["physical_host_count"] < 3:
            report["limitations"].append("Three installations on fewer than three physical hosts; not laptop/Pi/VPS acceptance")
        print("Prepared three fresh packaged installations", flush=True)
        a, b, c = nodes
        contents = {n.role + "-existing": (n.role + " preexisting bytes\n").encode() for n in nodes}
        for n in nodes:
            n.put("data/" + n.role + "-existing", contents[n.role + "-existing"])
        created = a.setup()
        folder = created["join"]["folder"]
        for owner, receiver in ((a, c), (c, b)):
            pending = receiver.setup(invitation=invitation(owner, folder))
            if not pending["join"]["request"] or pending["state"] == "completed":
                raise RuntimeError("approval bypassed")
            owner.restart(); receiver.restart()
            report["scenarios"][receiver.role + "_approval"] = approve(owner, receiver, pending)
        report["scenarios"]["membership"] = membership(nodes, folder, [n.device for n in nodes])
        print("PASS ordinary joining, delayed approval and exact membership", flush=True)
        report["scenarios"]["direct_tcp"] = {n.role: {p.role: [n.call("tcp-probe", address=p.address,
            port=int(p.settings[key].rsplit(":", 1)[1])) for key in ("peer_listen", "enrollment_listen")]
            for p in nodes if p is not n} for n in nodes}
        for path, value in contents.items():
            equal_bytes(nodes, path, value)
        report["scenarios"]["preserved_existing"] = histories(nodes, folder, list(contents))
        for n in nodes:
            path, value = n.role + "-edit", (n.role + " ordinary edit\n").encode()
            contents[path] = value
            n.put("data/" + path, value)
            equal_bytes(nodes, path, value)
        # B is stopped for the entire A -> C transfer. A is then stopped before B returns.
        b.call("terminal-stop"); b.workers.clear()
        forwarded = b"original source bytes forwarded without source/receiver overlap\n"
        contents["forwarded"] = forwarded
        a.put("data/forwarded", forwarded)
        equal_bytes([a, c], "forwarded", forwarded)
        source_history = histories([a, c], folder, ["forwarded"])
        original_version = source_history[a.role]["forwarded"][0]["version"]
        if original_version["author"] != a.device or len(source_history[a.role]["forwarded"]) != 1:
            raise RuntimeError("forwarded version lost its original author")
        report["scenarios"]["source_status_before_offline"] = json.loads(a.orbit("status", "--json"))
        a.call("terminal-stop"); a.workers.clear()
        b.restart()
        equal_bytes([b, c], "forwarded", forwarded)
        forwarded_history = histories([b, c], folder, ["forwarded"])
        if forwarded_history[b.role]["forwarded"][0]["version"] != original_version:
            raise RuntimeError("forwarder reauthored history")
        report["scenarios"]["forwarding"] = {"original_version": original_version, "history": forwarded_history,
                                                 "source_receiver_online_overlap": False}
        print("PASS original-author forwarding without source/receiver overlap", flush=True)
        a.restart()
        histories(nodes, folder, list(contents))
        report["scenarios"]["qualified_status"] = {n.role: json.loads(n.orbit("status", "--json")) for n in nodes}
        # Quiesce all participants before reviewing identical retiree snapshots on survivors.
        for n in nodes:
            n.call("terminal-stop"); n.workers.clear()
        previews = {n.role: json.loads(n.cli("peers", "retire", "--folder", folder, "--peer-device", b.device,
                                           "--preview", "--json")) for n in (a, c)}
        if previews[a.role] != previews[c.role]:
            raise RuntimeError("survivors disagree on exact retirement preview")
        human = json.loads(a.orbit("devices", "retire", "--folder", folder, "--device", b.device, "--preview", "--json"))
        if not human["warning"] or not human["disclaimer"]:
            raise RuntimeError("retirement omitted permanence/data-preservation preview")
        args = ("peers", "retire", "--folder", folder, "--peer-device", b.device,
                "--idempotency-key", "release-" + uuid.uuid4().hex, "--json")
        retired = json.loads(a.cli(*args))
        replay = json.loads(a.cli(*args))
        if not replay.get("replay") or replay["approved_digest"] != retired["approved_digest"]:
            raise RuntimeError("retirement replay differs")
        bundle = a.cli("membership", "export", "--folder", folder, "--json").encode()
        c.put("retirement-bundle.json", bundle)
        path = c.root + "/retirement-bundle.json"
        preview = json.loads(c.cli("membership", "preview", "--folder", folder, "--file", path, "--json"))
        if not preview["valid_transition"] or preview["next_digest"] != retired["approved_digest"]:
            raise RuntimeError("exported retirement transition disagrees")
        imported = json.loads(c.cli("membership", "import", "--folder", folder, "--file", path, "--approve", "--json"))
        report["scenarios"]["retirement"] = {"previews": previews, "warning": human, "result": retired,
            "replay": replay, "survivor_import": imported, "bundle_sha256": hashlib.sha256(bundle).hexdigest()}
        print("PASS exact survivor retirement review, exported rollout and replay", flush=True)
        for n in (a, c):
            n.restart()
        membership([a, c], folder, [a.device, c.device])
        # A request grants no membership; approval must refuse the same retired key.
        pending = b.setup("rejoin", invitation(a, folder))
        request = pending["join"]["request"]
        item = next(x for x in a.query("requests", limit="20")["requests"] if x["id"] == request)
        if item["requester"] != b.device or pending["state"] == "completed":
            raise RuntimeError("retired identity was admitted before approval")
        review = {k: item[k] for k in ("folder", "requester", "key_pin", "transcript_digest", "expected_membership")}
        review.update(request=request, decision="approve")
        a.put("retired-review.json", json.dumps(review).encode())
        rejected = a.orbit("requests", "approve", "--request", request,
                           "--review-file", a.root + "/retired-review.json", "--json", check=False)
        if rejected["returncode"] == 0 or "RETIRED_MEMBER_REVIVAL" not in rejected["stderr"]:
            raise RuntimeError("retired identity approval did not give its explicit refusal")
        membership([a, c], folder, [a.device, c.device])
        operation = b.query("operation", id=pending["operation"]["id"])
        if operation["state"] == "completed" or operation["operation"]["committed_effects"]:
            raise RuntimeError("rejected rejoin committed effects")
        report["scenarios"]["retired_rejoin_refused"] = {"approval": rejected, "operation": operation}
        review["decision"] = "decline"
        a.put("retired-decline.json", json.dumps(review).encode())
        a.orbit("requests", "decline", "--request", request, "--review-file", a.root + "/retired-decline.json", "--json")
        # Fresh installation, fresh persistent key. Old state/root are preserved.
        b.cleanup()
        replacement = PrivateNode(b.host, "replacement")
        replacement.prepare(dist, addresses[1])
        report["hosts"].append({"host": replacement.host, "role": replacement.role, "root": replacement.root,
                               "device": replacement.device, "inventory": replacement.inventory, **replacement.provenance})
        if replacement.device in {n.device for n in nodes}:
            raise RuntimeError("replacement reused an identity")
        replacement.put("data/replacement-existing", b"new installation preserves existing bytes\n")
        # Four quick installations can share one source IP. Replenish the real
        # per-IP enrollment budget before creating another signed attempt;
        # expiration of an unsent throttled attempt still requires new review.
        report["scenarios"]["replacement_enrollment_quiet_seconds"] = 60
        print("Waiting 60 seconds for the unchanged per-IP enrollment budget", flush=True)
        time.sleep(60)
        # Any active survivor may approve; use the forwarder after the source's
        # separate retired-key refusal, avoiding an artificial same-IP invite burst.
        pending = replacement.setup(invitation=invitation(c, folder))
        report["scenarios"]["replacement_approval"] = approve(c, replacement, pending)
        membership([a, c, replacement], folder, [a.device, c.device, replacement.device])
        for path, value in contents.items():
            equal_bytes([a, c, replacement], path, value)
            equal_bytes([b], path, value)
        equal_bytes([a, c, replacement], "replacement-existing", b"new installation preserves existing bytes\n")
        report["scenarios"]["replacement_history"] = histories([a, c, replacement], folder, list(contents))
        report["scenarios"]["replacement_membership"] = {n.role: json.loads(n.orbit("devices", "list", "--folder", folder, "--json")) for n in (a, c, replacement)}
        for facts in report["scenarios"]["replacement_membership"].values():
            if b.device not in {r["device"] for r in facts["retired"]}:
                raise RuntimeError("replacement lost retired identity fencing")
        report["success"] = True
    except Exception as error:
        # Never archive an exception containing invitation inputs/private request files.
        report["error"] = str(error).split(":", 1)[0][:160]
        raise
    finally:
        failures = []
        for n in [*nodes, *([replacement] if replacement else [])]:
            try:
                n.cleanup()
            except Exception:
                failures.append(n.role)
        report["cleanup_errors"] = failures
        if failures:
            report["success"] = False
        report["seconds"] = time.monotonic() - started
        report["ended_utc"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        (output / "terminal-private.json").write_text(json.dumps(report, indent=2) + "\n")
        if failures:
            raise RuntimeError("owned daemon cleanup failed: " + ", ".join(failures))
    print("PASS packaged ordinary joining, forwarding, reviewed retirement and replacement", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", type=Path, default=Path("dist"))
    parser.add_argument("--hosts", nargs=3, default=["laptop", "rpi", "vps"])
    parser.add_argument("--addresses", nargs=3, required=True)
    parser.add_argument("--network", choices=("lan", "private", "tailscale"), required=True)
    parser.add_argument("--preflight-only", action="store_true")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    run(args.dist, args.hosts, args.addresses, args.network, args.output, args.preflight_only)
