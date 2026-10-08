#!/usr/bin/env python3
"""Exercises the actual worker's refusal paths without signalling real services."""
import json
import os
from pathlib import Path
import tempfile
import unittest

from host_agent import beneath, dispatch, identity, validated_root


class SafetyTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="orbit-safety-")
        self.root = Path(self.temp.name)
        self.req = {"root": str(self.root), "token": "disposable safety fixture"}
        (self.root / ".orbit-disposable").write_text(self.req["token"])
        self.addCleanup(self.temp.cleanup)

    def test_missing_and_wrong_marker_refused_before_write(self):
        (self.root / ".orbit-disposable").unlink()
        with self.assertRaises(FileNotFoundError):
            dispatch({**self.req, "action": "put", "path": "important", "data": ""})
        self.assertFalse((self.root / "important").exists())
        (self.root / ".orbit-disposable").write_text("wrong token")
        with self.assertRaises(RuntimeError):
            validated_root(self.req)

    def test_paths_and_links_refused(self):
        for path in ["/etc/passwd", "../outside", "."]:
            with self.assertRaises(RuntimeError):
                beneath(self.root, path)
        (self.root / "link").symlink_to("/tmp")
        with self.assertRaises(RuntimeError):
            beneath(self.root, "link/file")
        (self.root / "original").write_bytes(b"protected")
        os.link(self.root / "original", self.root / "alias")
        with self.assertRaises(RuntimeError):
            beneath(self.root, "alias")
        self.assertEqual((self.root / "original").read_bytes(), b"protected")

    def test_terminal_daemon_signal_and_pilot_refused(self):
        (self.root / "state").mkdir()
        (self.root / "state/.agent.pid").write_text(str(os.getpid()))
        with self.assertRaises(RuntimeError):
            dispatch({**self.req, "action": "terminal-stop"})
        (self.root / ".orbit-disposable").unlink()
        (self.root / ".orbit-pilot").write_text(self.req["token"])
        with self.assertRaises(FileNotFoundError):
            dispatch({**self.req, "action": "terminal-stop"})
        with self.assertRaises(FileNotFoundError):
            dispatch({**self.req, "action": "terminal-pty", "script": "terminal_pty_test.py"})

    def test_terminal_query_and_campaign_paths_refused(self):
        with self.assertRaises(RuntimeError):
            dispatch({**self.req, "action": "terminal-pty", "script": "../unrelated.py"})
        (self.root / "state").mkdir()
        (self.root / "state/control.addr").write_text("192.0.2.1:8080")
        with self.assertRaises(RuntimeError):
            dispatch({**self.req, "action": "terminal-query", "query": {"kind": "status"}})

    def test_unrelated_pid_and_reused_pid_refused(self):
        ticks, _ = identity(os.getpid())
        for value in [ticks, "incorrect-start-time"]:
            (self.root / "serve-test.pid.json").write_text(json.dumps({"pid": os.getpid(), "start_ticks": value, "kind": "serve"}))
            with self.assertRaises(RuntimeError):
                dispatch({**self.req, "action": "stop", "name": "serve-test"})

    def test_network_preflight_rejects_unsafe_addresses_before_commands(self):
        from unittest.mock import patch
        with patch("host_agent.subprocess.check_output") as command:
            for address in ("127.0.0.1", "0.0.0.0", "8.8.8.8", "192.168.1.1;reboot"):
                with self.assertRaises((RuntimeError, ValueError)):
                    dispatch({"action": "network-preflight", "addresses": [address]})
            command.assert_not_called()

    def test_network_probe_requires_disposable_marker(self):
        (self.root / ".orbit-disposable").unlink()
        from unittest.mock import patch
        with patch("host_agent.socket.create_connection") as connect:
            with self.assertRaises(FileNotFoundError):
                dispatch({**self.req, "action": "tcp-probe", "address": "192.168.1.1", "port": 8443})
            connect.assert_not_called()


if __name__ == "__main__":
    unittest.main()
