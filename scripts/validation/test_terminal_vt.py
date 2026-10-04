"""Regression for independent visible-screen assertions in the PTY harness."""
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from terminal_vt import Screen


class TerminalScreenTests(unittest.TestCase):
    def test_differential_label_reconstruction(self):
        s = Screen(40, 16)
        s.feed(b"Overview | f n d o\r\nold error")
        s.feed(b"\x1b[1;1HFolders \x1b[P\x1b[2;1H\x1b[K")
        self.assertIn("Folders | f n d o", s.text())
        self.assertNotIn("old error", s.text())

    def test_split_utf8_and_wide_cursor_cells(self):
        s = Screen(10, 3)
        data = "a界e\u0301".encode()
        for b in data:
            s.feed(bytes([b]))
        s.feed(b"\x1b[1;5HX")
        self.assertEqual(s.text().splitlines()[0].rstrip(), "a界e\u0301X")
        s.feed(b"\x1b[2J\x1b[Hfresh")
        self.assertNotIn("界", s.text())

    def test_scroll_region_preserves_header_footer(self):
        s=Screen(6,5)
        s.feed(b"HEAD\r\n111\r\n222\r\n333\r\nFOOT")
        s.feed(b"\x1b[2;4r\x1b[4;1H\n")
        self.assertEqual(s.text().splitlines(),["HEAD  ","222   ","333   ","      ","FOOT  "])
        s.feed(b"\x1b[2;1H\x1bM")
        self.assertEqual(s.text().splitlines()[0],"HEAD  ")
        self.assertEqual(s.text().splitlines()[-1],"FOOT  ")

    def test_disabled_autowrap_and_fullwidth_sgr(self):
        s=Screen(4,3)
        s.feed(b"\x1b[?7l12345")
        self.assertEqual(s.text().splitlines()[0],"1235")
        s.feed(b"\x1b[?7h\x1b[2;1Habcd\x1b[mZ")
        self.assertEqual(s.text().splitlines()[1],"abcd")
        self.assertEqual(s.text().splitlines()[2],"Z   ")

    def test_repeat_character(self):
        s=Screen(8,2)
        s.feed(b"a\x1b[3b!")
        self.assertEqual(s.text().splitlines()[0],"aaaa!   ")


if __name__ == "__main__":
    unittest.main()
