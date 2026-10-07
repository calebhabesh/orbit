"""W15 refusal paths. No firewall commands and no process is signalled."""
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock
from wan_safety import MARKER, TOKEN, validate_child, validate_namespace, validate_root, validate_transcript


class W15SafetyTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix='orbit-w15-', dir='/tmp')
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.marker = self.root / MARKER
        self.marker.write_text(TOKEN)
        self.marker.chmod(0o600)

    def test_private_marked_root(self):
        self.assertEqual(validate_root(str(self.root)), self.root)

    def test_missing_wrong_linked_public_marker(self):
        self.marker.unlink()
        with self.assertRaises(FileNotFoundError):
            validate_root(self.root)
        self.marker.write_text('wrong')
        self.marker.chmod(0o600)
        with self.assertRaises(RuntimeError):
            validate_root(self.root)
        self.marker.write_text(TOKEN)
        os.link(self.marker, self.root / 'alias')
        with self.assertRaises(RuntimeError):
            validate_root(self.root)
        (self.root / 'alias').unlink()
        self.marker.chmod(0o644)
        with self.assertRaises(RuntimeError):
            validate_root(self.root)
        self.marker.unlink()
        self.marker.symlink_to(self.root / 'absent')
        with self.assertRaises(RuntimeError):
            validate_root(self.root)

    def test_symlinks_escapes_personal_roots(self):
        link = self.root / 'link'
        link.symlink_to(self.root, target_is_directory=True)
        for path in (link, self.root / '..', Path.home(), '/tmp', str(self.root) + '/link/..'):
            with self.assertRaises(RuntimeError):
                validate_root(path)
        (self.root / '.filesync-pilot').write_text(TOKEN)
        with self.assertRaises(RuntimeError):
            validate_root(self.root)

    def test_public_root(self):
        self.root.chmod(0o755)
        with self.assertRaises(RuntimeError):
            validate_root(self.root)

    def test_host_namespace(self):
        with self.assertRaises(RuntimeError):
            validate_namespace(os.readlink('/proc/self/ns/net'))
        with self.assertRaises(RuntimeError):
            validate_namespace('')

    def test_unrelated_live_and_exited_process(self):
        child = Mock(pid=os.getpid())
        child.poll.return_value = None
        with self.assertRaises(RuntimeError):
            validate_child(child, self.root)
        child.poll.return_value = 0
        with self.assertRaises(RuntimeError):
            validate_child(child, self.root)


class W15ExecutionTest(unittest.TestCase):
    def test_zero_matches_skips_failures_are_unexecuted(self):
        for text in ['testing: warning: no tests to run\nPASS\n', '--- SKIP: TestWANW15WholeDaemonCrashRouteAndReceiptRecovery (0.00s)\nPASS\n', '--- FAIL: TestWANW15WholeDaemonCrashRouteAndReceiptRecovery (0.01s)']:
            with self.assertRaises(RuntimeError):
                validate_transcript('crash', text)
        validate_transcript('crash', '--- PASS: TestWANW15WholeDaemonCrashRouteAndReceiptRecovery (25.0s)\nPASS\n')
        with self.assertRaises(RuntimeError):
            validate_transcript('ice', '--- PASS: TestWANW10IsolatedSTUNIPv6 (0.00s)\n')
