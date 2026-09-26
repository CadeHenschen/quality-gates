#!/usr/bin/env python3
# Walks a directory's .py source (skipping __pycache__, dotdirs, and
# pytest-convention test files) and prints one JSON object per file to
# stdout: {"file": ..., "tokens": [{"text": ..., "line": ...}, ...]}.
# Uses only the standard library's `tokenize` module — no pip dependency
# needed, unlike crap-metric's Python analyzer (which shells to radon).
import json
import os
import sys
import tokenize

SKIP_TYPES = {
    tokenize.COMMENT,
    tokenize.NL,
    tokenize.NEWLINE,
    tokenize.INDENT,
    tokenize.DEDENT,
    tokenize.ENCODING,
    tokenize.ENDMARKER,
}


def is_test_file(name):
    return name.startswith("test_") or name.endswith("_test.py")


def walk(root):
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d != "__pycache__" and not d.startswith(".")]
        for name in filenames:
            if name.endswith(".py") and not is_test_file(name):
                yield os.path.join(dirpath, name)


def tokenize_file(path):
    tokens = []
    with open(path, "rb") as f:
        try:
            for tok in tokenize.tokenize(f.readline):
                if tok.type in SKIP_TYPES:
                    continue
                tokens.append({"text": tok.string, "line": tok.start[0]})
        except (tokenize.TokenizeError, SyntaxError, IndentationError) as e:
            print(f"warning: skipping {path}: {e}", file=sys.stderr)
    return tokens


def main():
    if len(sys.argv) != 1:
        print("usage: tokenize_files.py", file=sys.stderr)
        sys.exit(2)

    root = os.getcwd()
    out = []
    for path in walk(root):
        tokens = tokenize_file(path)
        if tokens:
            # Relative to root, not the raw walked path — so e.g. `--dir
            # ../../src` reports "foo.py", not "../../src/foo.py" (which
            # would also break --only-files matching in the Go CLI, whose
            # changed-file list is relative to the repo root, not to
            # wherever --dir's own ".." components happen to point).
            out.append({"file": os.path.relpath(path, root), "tokens": tokens})

    json.dump(out, sys.stdout)


if __name__ == "__main__":
    main()
