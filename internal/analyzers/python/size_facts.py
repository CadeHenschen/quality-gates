#!/usr/bin/env python3
"""Walks a directory's .py source with the stdlib `ast` module and prints
one JSON object per function/method/async-function to stdout: its
parameter count, its deepest control-flow nesting depth, and its file's
total physical line count. Merged in Go against radon's cc output (which
carries complexity but not these facts) by (file, lineno) — the "def"
line, the same convention radon itself uses.  Stdlib only, no pip package
needed, same posture as scan_tests.py / tokenize_files.py."""
import ast
import json
import os
import sys

SKIP_DIRS = {"node_modules", "__pycache__", "venv", "site-packages", "vendor"}


def param_count(node, is_method):
    """Counts a function's declared parameters — positional-only,
    positional-or-keyword, and keyword-only args each count once, *args/
    **kwargs count once each if present. self/cls isn't counted for an
    instance/class method (mirrors how the Go analyzer excludes the
    receiver), but IS counted for an explicit @staticmethod, which has
    no implicit first argument to exclude."""
    a = node.args
    n = len(a.posonlyargs) + len(a.args) + len(a.kwonlyargs)
    if a.vararg:
        n += 1
    if a.kwarg:
        n += 1
    if is_method and n > 0:
        n -= 1
    return n


def is_staticmethod(node):
    for d in node.decorator_list:
        target = d.func if isinstance(d, ast.Call) else d
        if isinstance(target, ast.Name) and target.id == "staticmethod":
            return True
        if isinstance(target, ast.Attribute) and target.attr == "staticmethod":
            return True
    return False


# nesting_depth/nesting_of mirror the Go analyzer's algorithm (see
# internal/analyzers/golang/size.go): each control-flow block adds one
# level, but a chained `elif` does NOT nest deeper than its `if` — it
# reads like a switch's cases, the same reasoning size-metric exists for.
# Python's grammar can't tell `elif b:` apart from `else:\n    if b:` at
# the ast level (both parse to orelse=[If(...)]) — a known limitation,
# same spirit as swiftlex's documented lexical gaps: this flattens a
# deliberate `else: if` too, which is rare enough in real code not to
# matter for a generous, egregious-cases-only gate.
def nesting_depth(stmts, depth):
    m = depth
    for s in stmts:
        d = nesting_of(s, depth)
        if d > m:
            m = d
    return m


def nesting_of(stmt, depth):
    if isinstance(stmt, ast.If):
        m = nesting_depth(stmt.body, depth + 1)
        if len(stmt.orelse) == 1 and isinstance(stmt.orelse[0], ast.If):
            d = nesting_of(stmt.orelse[0], depth)
        elif stmt.orelse:
            d = nesting_depth(stmt.orelse, depth + 1)
        else:
            d = depth
        return max(m, d)
    looping = (ast.For, ast.While)
    if hasattr(ast, "AsyncFor"):
        looping = looping + (ast.AsyncFor,)
    if isinstance(stmt, looping):
        return nesting_depth(stmt.body, depth + 1)
    withs = (ast.With,)
    if hasattr(ast, "AsyncWith"):
        withs = withs + (ast.AsyncWith,)
    if isinstance(stmt, withs):
        return nesting_depth(stmt.body, depth + 1)
    if isinstance(stmt, ast.Try):
        m = nesting_depth(stmt.body, depth + 1)
        for h in stmt.handlers:
            m = max(m, nesting_depth(h.body, depth + 1))
        if stmt.orelse:
            m = max(m, nesting_depth(stmt.orelse, depth + 1))
        if stmt.finalbody:
            m = max(m, nesting_depth(stmt.finalbody, depth + 1))
        return m
    if hasattr(ast, "Match") and isinstance(stmt, ast.Match):
        m = depth
        for case in stmt.cases:
            m = max(m, nesting_depth(case.body, depth + 1))
        return m
    return depth


FUNC_TYPES = (ast.FunctionDef, ast.AsyncFunctionDef)


def scan_defs(body, is_method, out):
    """Records every top-level def in body (module level or a class's
    own members) — NOT recursing into a function's own nested defs,
    matching radon's own flat entries (a nested/local def gets its own
    separate top-level-ish entry in radon's output too, keyed by its own
    lineno, so it's handled when scan_file walks to it directly, not by
    double-recursing here)."""
    for node in body:
        if isinstance(node, FUNC_TYPES):
            out.append((node, is_method and not is_staticmethod(node)))
        elif isinstance(node, ast.ClassDef):
            scan_defs(node.body, True, out)


def scan_file(path):
    with open(path, encoding="utf-8") as f:
        src = f.read()
    tree = ast.parse(src, filename=path)
    file_lines = len(src.splitlines())

    defs = []
    scan_defs(tree.body, False, defs)
    # Nested defs (a function defined inside another function) aren't
    # walked by scan_defs above; ast.walk finds every FunctionDef in the
    # tree regardless of nesting, so union that in too, keyed by lineno
    # to skip ones scan_defs already found (nested defs are never
    # methods, so no self/cls to exclude).
    seen_lines = {node.lineno for node, _ in defs}
    for node in ast.walk(tree):
        if isinstance(node, FUNC_TYPES) and node.lineno not in seen_lines:
            defs.append((node, False))
            seen_lines.add(node.lineno)

    out = []
    for node, is_method in defs:
        out.append({
            "file": path,
            "lineno": node.lineno,
            "param_count": param_count(node, is_method),
            "max_nesting_depth": nesting_depth(node.body, 0),
            "file_lines": file_lines,
        })
    return out


def main():
    root = os.getcwd()
    results = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIRS and not d.startswith("."))
        for fn in sorted(filenames):
            if not fn.endswith(".py"):
                continue
            path = os.path.join(dirpath, fn)
            results.extend(scan_file(path))
    json.dump(results, sys.stdout)


if __name__ == "__main__":
    main()
