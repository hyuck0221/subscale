#!/usr/bin/env python3
"""Update the tap's four release URLs/checksums without changing its behavior."""
import argparse
from pathlib import Path
import re


ASSETS = {
    f"subscale_{system}_{arch}.tar.gz"
    for system in ("darwin", "linux")
    for arch in ("amd64", "arm64")
}
BASE = "https://github.com/hyuck0221/subscale/releases/download/"
TAG = r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
ENTRY = re.compile(
    r'(?P<url>https://github\.com/hyuck0221/subscale/releases/download/'
    r'(?P<tag>v[0-9]+\.[0-9]+\.[0-9]+)/'
    r'(?P<asset>subscale_(?:darwin|linux)_(?:amd64|arm64)\.tar\.gz))'
    r'(?P<between>"[ \t]*\n[ \t]+sha256 ")(?P<digest>[0-9a-f]{64})'
)


def version(tag):
    match = re.fullmatch(TAG, tag)
    if not match:
        raise ValueError(f"expected a stable release tag vMAJOR.MINOR.PATCH, got {tag!r}")
    return tuple(map(int, match.groups()))


def checksums(text):
    result = {}
    for line in text.splitlines():
        if not line.strip():
            continue
        fields = line.split()
        if len(fields) != 2 or not re.fullmatch(r"[0-9a-fA-F]{64}", fields[0]):
            raise ValueError("invalid checksum manifest entry")
        digest, name = fields
        if name in result:
            raise ValueError(f"duplicate checksum for {name}")
        result[name] = digest.lower()
    missing = ASSETS - result.keys()
    if missing:
        raise ValueError(f"missing release checksums: {', '.join(sorted(missing))}")
    return result


def update_formula(formula, manifest, tag):
    target_version = version(tag)
    digests = checksums(manifest)
    matches = list(ENTRY.finditer(formula))
    if len(matches) != 4 or {m['asset'] for m in matches} != ASSETS:
        raise ValueError("expected exactly four platform URL/checksum pairs in the formula")
    old_tags = {m['tag'] for m in matches}
    if len(old_tags) != 1:
        raise ValueError("formula contains mixed release versions")
    old_tag = old_tags.pop()
    if target_version < version(old_tag):
        return formula  # An older workflow finishing late must not downgrade the tap.
    return ENTRY.sub(
        lambda m: BASE + tag + "/" + m['asset'] + m['between'] + digests[m['asset']],
        formula,
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--formula", required=True, type=Path)
    parser.add_argument("--checksums", required=True, type=Path)
    parser.add_argument("--tag", required=True)
    args = parser.parse_args()
    try:
        original = args.formula.read_text()
        updated = update_formula(original, args.checksums.read_text(), args.tag)
    except (ValueError, OSError) as error:
        parser.exit(1, f"Tap update failed: {error}\n")
    if updated == original:
        print("Tap already has this release or a newer version; no changes.")
        return
    args.formula.write_text(updated)
    print(f"Updated {args.formula} to {args.tag}")


if __name__ == "__main__":
    main()
