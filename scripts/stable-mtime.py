#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Sets the mtime of every tracked file and directory of the working tree from its content, the same in every checkout.

Go's test result cache keys a file that a test opens on its size and mtime, and a fresh checkout gives every file a
new mtime, so the tests that read files at run time (testdata, golden files, the documents go:config-docs compares)
never hit the cache that main builds (ADR-0079 §7); Go checks the directories a test stats too. Here a file's mtime
is derived from its blob ID in the index (equal content gives an equal mtime in every checkout, and changed content
another one), and the directories get one constant mtime. A constant mtime would not do: an edit that keeps the
size would reuse a stale result.

  scripts/stable-mtime.py            (from the repository's root, after the checkout)
"""
import os
import subprocess
import sys

# 2000-01-01 plus up to about 19 years, far enough in the past that Go never treats a file as modified just now.
EPOCH = 946684800
SPAN = 600_000_000


def main() -> int:
    out = subprocess.run(["git", "ls-files", "-s", "-z"], check=True, capture_output=True).stdout
    n = 0
    for entry in out.split(b"\0"):
        if not entry:
            continue
        meta, path = entry.split(b"\t", 1)
        mode, blob, _stage = meta.split(b" ")
        if mode == b"160000":  # a submodule has no file of its own
            continue
        t = EPOCH + int(blob[:12], 16) % SPAN
        try:
            os.utime(path, (t, t), follow_symlinks=False)
        except FileNotFoundError:
            continue
        n += 1
    # The directories get one constant mtime. Go hashes a directory that a test opens by its entries, each with its
    # own stat, so a directory's mtime adds nothing; one derived from its tree ID would change the root and every
    # parent with any change of the commit, and miss the cache for every test that stats them.
    dirs = {b"."}
    for entry in out.split(b"\0"):
        if entry:
            path = entry.split(b"\t", 1)[1]
            while b"/" in path:
                path = path.rsplit(b"/", 1)[0]
                if path in dirs:
                    break
                dirs.add(path)
    d = 0
    for path in dirs:
        try:
            os.utime(path, (EPOCH, EPOCH), follow_symlinks=False)
        except FileNotFoundError:
            continue
        d += 1
    print(f"stable-mtime: {n} files, {d} directories", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
