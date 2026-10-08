#!/usr/bin/env python3
"""Scripted demo on three actual hosts; this does not constitute personal use."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import random
import time
import uuid

from harness import Node, link, pair, sync, prepare_output


def conflicts(node, path):
    report = json.loads(node.cli("conflicts", "--folder", node.folder, "--json"))
    return next((c for c in (report["conflicts"] or []) if c["path"] == path), None)


def version_text(value):
    return value["Author"] + ":" + str(value["Counter"])


def select(node, conflict, selected):
    reviewed = ",".join(version_text(h["id"]) for h in conflict["heads"])
    return json.loads(node.cli("resolve", "select", "--folder", node.folder, "--path", conflict["path"],
                               "--reviewed", reviewed, "--head-token", conflict["head_token"],
                               "--selected", selected, "--idempotency-key", uuid.uuid4().hex, "--json"))


def verify(nodes, path, expected):
    digest = hashlib.sha256(expected).hexdigest()
    observed = {n.role: n.call("hash", path=path)["sha256"] for n in nodes}
    if set(observed.values()) != {digest}:
        raise RuntimeError(f"working bytes differ for {path}: {observed}")
    return {"expected_sha256": digest, "observed": observed}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--laptop", default="laptop")
    parser.add_argument("--pi", default="rpi")
    parser.add_argument("--vps", default="vps")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    prepare_output(args.output)
    start = time.monotonic()
    nodes = [Node(args.laptop, "laptop"), Node(args.pi, "pi"), Node(args.vps, "vps")]
    a, b, c = nodes
    report = {"type": "automated actual-host demonstration", "personal_pilot": "not established by this script",
              "started_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
              "hosts": [], "scenarios": {}, "success": False}
    results = report["scenarios"]
    try:
        folder = uuid.uuid4().hex * 2
        for n in nodes:
            n.setup(folder)
            report["hosts"].append({"role": n.role, "ssh_alias": n.host, "root": n.root,
                                     "inventory": n.inventory, "binary_sha256": n.binary_hash, "device": n.device})
        pair(nodes)
        baseline = b"Dedicated Orbit validation note: initial version.\n"
        a.put("data/note.txt", baseline)
        a.scan()
        results["normal_sync"] = {"a_to_vps": sync(a, c), "vps_to_pi": sync(c, b),
                                   "hashes": verify(nodes, "note.txt", baseline)}
        history = json.loads(a.cli("history", "--folder", folder, "--path", "note.txt", "--json"))
        results["initial_history"] = history
        # Capture a separate three-way conflict before exercising late arrival.
        for n in nodes:
            n.put("data/three.txt", (n.role + " independent offline version\n").encode())
            n.scan()
        sync(a, c)
        sync(b, c)
        sync(c, a)
        sync(c, b)
        triples = {n.role: conflicts(n, "three.txt") for n in nodes}
        if any(v is None or len(v["heads"]) != 3 for v in triples.values()):
            raise RuntimeError("three independent heads did not survive")
        if len({v["head_token"] for v in triples.values()}) != 1:
            raise RuntimeError("three-head tokens differ")
        results["three_way_conflict"] = triples
        chosen = next(version_text(h["id"]) for h in triples[a.role]["heads"] if h["id"]["Author"] == a.device)
        results["three_way_resolution"] = select(a, triples[a.role], chosen)
        sync(a, c)
        sync(c, b)
        results["three_way_resolved_hashes"] = verify(nodes, "three.txt", (a.role + " independent offline version\n").encode())
        # No daemons are running while independent edits are captured.
        edits = [b"Laptop offline proposal\n", b"Pi offline proposal\n", b"VPS offline proposal\n"]
        for n, data in zip(nodes, edits):
            n.put("data/note.txt", data)
            n.scan()
        results["offline_capture"] = {"all_peer_daemons_stopped": all(not n.workers for n in nodes)}
        # A and B exchange while C's version is withheld. Resolution is explicit.
        results["a_b_reconnect"] = sync(a, b)
        results["b_a_reconnect"] = sync(b, a)
        reviewed = conflicts(a, "note.txt")
        if reviewed is None or len(reviewed["heads"]) != 2:
            raise RuntimeError("expected two reviewed heads before late arrival")
        selected = next(version_text(h["id"]) for h in reviewed["heads"] if h["id"]["Author"] == a.device)
        results["reviewed_a_b_resolution"] = select(a, reviewed, selected)
        results["late_c_arrival"] = sync(c, a)
        late = conflicts(a, "note.txt")
        if late is None or len(late["heads"]) != 2:
            raise RuntimeError("late C was silently resolved")
        # A reviewed token from before C's arrival must fail, preserving both heads.
        stale = a.cli("resolve", "select", "--folder", folder, "--path", "note.txt",
                      "--reviewed", ",".join(version_text(h["id"]) for h in reviewed["heads"]),
                      "--head-token", reviewed["head_token"], "--selected", selected, check=False)
        if stale["returncode"] == 0 or "stale" not in stale["stderr"].lower():
            raise RuntimeError("stale reviewed state was not rejected")
        results["late_arrival_conflict"] = {"heads": late, "stale_request_rejected": True}
        final_selected = next(version_text(h["id"]) for h in late["heads"] if h["id"]["Author"] == a.device)
        results["final_resolution"] = select(a, late, final_selected)
        sync(a, c)
        sync(c, b)
        results["resolved_hashes"] = verify(nodes, "note.txt", edits[0])
        if any(conflicts(n, "note.txt") for n in nodes):
            raise RuntimeError("conflict did not clear on all hosts")
        # Forwarding sessions do not overlap. A has no listener during B's pull.
        forward = b"Original laptop author preserved through VPS relay.\n"
        a.put("data/forward.txt", forward)
        a.scan()
        results["forward_to_vps"] = sync(a, c)
        if a.workers:
            raise RuntimeError("A must be offline before B connects")
        results["forward_to_pi"] = sync(c, b)
        results["forward_hashes"] = verify(nodes, "forward.txt", forward)
        forwarded = json.loads(b.cli("history", "--folder", folder, "--path", "forward.txt", "--json"))
        results["forwarded_history"] = forwarded
        if a.device not in json.dumps(forwarded):
            raise RuntimeError("original author missing after forwarding")
        results["qualified_a_status"] = json.loads(a.cli("status", "--folder", folder, "--json"))
        forward_id = forwarded[-1]["id"]
        if any(p["peer"] == b.device and p["version"] == forward_id and p["receipt"]
               for p in (results["qualified_a_status"]["peers"] or [])):
            raise RuntimeError("A falsely claims B's receipt after forwarding without contact")
        results["forward_status_assertion"] = {"original_author": forward_id["Author"],
                                                "a_does_not_claim_b_receipt": True}
        # Distinct pseudorandom chunks avoid accidental fixture deduplication.
        payload = random.Random(20261001).randbytes(12 * 1024 * 1024)
        a.put("data/resume.bin", payload)
        a.scan()
        before = b.call("progress")
        before_objects = b.call("verified-objects")
        name, port = a.serve()
        try:
            with link(a, b, port, bandwidth=1024 * 1024) as (url, proxy):
                worker = b.start("sync", ["sync", "--folder", folder, "--peer-url", url, "--peer-device", a.device,
                                           "--peer-certificate", b.root + "/laptop.pem", "--json"])
                deadline = time.monotonic() + 45
                while time.monotonic() < deadline:
                    progress = b.call("verified-objects")
                    if progress["verified_objects"] >= before_objects["verified_objects"] + 2:
                        break
                    if b.call("poll", name=worker)["returncode"] is not None:
                        raise RuntimeError("transfer completed before interruption")
                    time.sleep(0.1)
                else:
                    raise RuntimeError("no verified chunk before interruption")
                b.stop(worker)
                interrupted = b.call("progress")
                transferred = interrupted["verified_chunks"] - before["verified_chunks"]
                if not 0 < transferred < 12:
                    raise RuntimeError("interruption was not mid-file")
                partial = {"verified_before_restart": transferred, **proxy.metrics()}
        finally:
            a.stop(name)
        resumed = sync(a, b)
        results["interrupted_resume"] = {"partial": partial, "resumed": resumed,
                                         "hashes": verify([a, b], "resume.bin", payload)}
        counts = resumed["result"]
        # An object can be durable just before its progress-row transaction.
        # Reusing that extra verified object is valid; all recorded progress
        # must be reused, and every distinct chunk must be accounted for.
        if counts["chunks_fetched"] > 12 - transferred or counts["chunks_reused"] < transferred or \
                counts["chunks_fetched"] + counts["chunks_reused"] != 12:
            raise RuntimeError("restart did not reuse verified chunk progress")
        # Restore from the first recorded source, using a fresh reviewed preview.
        entries = history
        source = version_text(entries[0]["id"])
        preview = json.loads(a.cli("restore", "--folder", folder, "--path", "note.txt", "--source", source, "--preview", "--json"))
        restored = json.loads(a.cli("restore", "--folder", folder, "--path", "note.txt", "--source", source,
                                    "--reviewed", ",".join(version_text(v) for v in preview["current_heads"]),
                                    "--head-token", preview["expected_head_token"], "--idempotency-key", uuid.uuid4().hex, "--json"))
        if restored["resolved_id"] == entries[0]["id"]:
            raise RuntimeError("restore reused historical version identity")
        sync(a, c)
        sync(c, b)
        results["restore"] = {"result": restored, "hashes": verify(nodes, "note.txt", baseline)}
        results["restart"] = {}
        for n in nodes:
            daemon, _ = n.serve()
            n.stop(daemon)
            daemon, _ = n.serve()
            n.stop(daemon)
            results["restart"][n.role] = n.call("integrity")
            if results["restart"][n.role]["sqlite"] != "ok":
                raise RuntimeError("SQLite integrity failed")
        report["success"] = True
    finally:
        for n in nodes:
            if n.root:
                n.cleanup()
        report["seconds"] = time.monotonic() - start
        (args.output / "three-host.json").write_text(json.dumps(report, indent=2) + "\n")
    print(f"PASS actual-host demo in {report['seconds']:.1f}s; roots retained; personal use remains separate")


if __name__ == "__main__":
    main()
