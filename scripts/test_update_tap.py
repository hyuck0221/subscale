import subprocess
import sys
import tempfile
from pathlib import Path
import unittest

from update_tap import ASSETS, BASE, update_formula


def fixture(tag="v0.1.3"):
    pairs = "\n".join(
        f'      url "{BASE}{tag}/{name}"\n      sha256 "{"a" * 64}"'
        for name in sorted(ASSETS)
    )
    return 'class Subscale < Formula\n' + pairs + '\n  # Keep wrapper, service and tests unchanged.\nend\n'


def manifest():
    return "\n".join(
        f'{str(index) * 64}  {name}'
        for index, name in enumerate(sorted(ASSETS), start=1)
    )


class TapUpdateTest(unittest.TestCase):
    def test_all_platforms_updated_and_other_content_preserved(self):
        updated = update_formula(fixture(), manifest(), "v0.2.0")
        for index, name in enumerate(sorted(ASSETS), start=1):
            self.assertIn(f'{BASE}v0.2.0/{name}"\n      sha256 "{str(index) * 64}"', updated)
        self.assertTrue(updated.endswith('  # Keep wrapper, service and tests unchanged.\nend\n'))
        self.assertNotIn("v0.1.3", updated)

    def test_rerun_is_idempotent(self):
        updated = update_formula(fixture(), manifest(), "v0.2.0")
        self.assertEqual(updated, update_formula(updated, manifest(), "v0.2.0"))

    def test_late_old_release_cannot_downgrade_tap(self):
        original = fixture("v0.1.10")
        self.assertEqual(original, update_formula(original, manifest(), "v0.1.9"))
        self.assertIn("v0.1.11", update_formula(original, manifest(), "v0.1.11"))

    def test_rejects_incomplete_invalid_and_duplicate_checksums(self):
        for text in ["", manifest().splitlines()[0], manifest() + "\n" + manifest(),
                     manifest().replace("1" * 64, "bad-digest")]:
            with self.subTest(manifest=text), self.assertRaises(ValueError):
                update_formula(fixture(), text, "v0.2.0")

    def test_rejects_unexpected_formula_layout(self):
        for text in ["class Subscale < Formula\nend\n", fixture().replace("v0.1.3", "v0.1.2", 1),
                     fixture() + fixture(), fixture().replace("subscale_linux_arm64", "subscale_linux_amd64")]:
            with self.subTest(formula=text), self.assertRaises(ValueError):
                update_formula(text, manifest(), "v0.2.0")

    def test_rejects_nonstable_and_unsafe_tags(self):
        for tag in ["v0.2.0-rc.1", "v0.2.0+build", "v01.2.0", "0.2.0", "v0.2", "v0.2.0/other", "$(id)"]:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                update_formula(fixture(), manifest(), tag)

    def test_cli_does_not_write_on_validation_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            formula = Path(directory) / "subscale.rb"
            checksums = Path(directory) / "checksums.txt"
            original = fixture()
            formula.write_text(original)
            checksums.write_text("invalid manifest")
            result = subprocess.run(
                [sys.executable, str(Path(__file__).with_name("update_tap.py")),
                 "--formula", str(formula), "--checksums", str(checksums), "--tag", "v0.2.0"],
                capture_output=True, text=True, timeout=10,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Tap update failed", result.stderr)
            self.assertEqual(formula.read_text(), original)


if __name__ == "__main__":
    unittest.main()
