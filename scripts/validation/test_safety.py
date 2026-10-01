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
        self.temp = tempfile.TemporaryDirectory(prefix="filesync-safety-")
        self.root = Path(self.temp.name)
        self.req = {"root": str(self.root), "token": "disposable safety fixture"}
        (self.root / ".filesync-disposable").write_text(self.req["token"])
        self.addCleanup(self.temp.cleanup)

    def test_missing_and_wrong_marker_refused_before_write(self):
        (self.root / ".filesync-disposable").unlink()
        with self.assertRaises(FileNotFoundError):
            dispatch({**self.req, "action": "put", "path": "important", "data": ""})
        self.assertFalse((self.root / "important").exists())
        (self.root / ".filesync-disposable").write_text("wrong token")
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

    def test_unrelated_pid_and_reused_pid_refused(self):
        ticks, _ = identity(os.getpid())
        for value in [ticks, "incorrect-start-time"]:
            (self.root / "serve-test.pid.json").write_text(json.dumps({"pid": os.getpid(), "start_ticks": value, "kind": "serve"}))
            with self.assertRaises(RuntimeError):
                dispatch({**self.req, "action": "stop", "name": "serve-test"})


if __name__ == "__main__":
    unittest.main()
