#!/usr/bin/env python3
"""Walks a directory's Python test files (test_*.py / *_test.py, skipping
dotdirs, venvs, node_modules, __pycache__) with the stdlib `ast` module and
prints one JSON object per test function to stdout: its assertion counts and
whether it is skipped / expected-to-fail / leaks a temp dir. Stdlib only —
no pip package needed, same as the dupe-metric tokenizer script."""
import ast
import json
import os
import re
import sys

SKIP_DIRS = {"node_modules", "__pycache__", "venv", "site-packages", "vendor"}
HELPER = re.compile(r"^_?(assert|check|verify|expect|ensure|validate|compare)", re.I)
MOCK_ASSERTS = {
    "assert_called", "assert_called_once", "assert_called_with",
    "assert_called_once_with", "assert_any_call", "assert_has_calls",
    "assert_not_called", "assert_awaited", "assert_awaited_once",
    "assert_awaited_with", "assert_awaited_once_with", "assert_any_await",
    "assert_has_awaits", "assert_not_awaited",
}
MOCK_ATTRS = {"called", "call_count", "call_args", "call_args_list", "mock_calls", "method_calls"}
PYTEST_ASSERTERS = {"raises", "warns", "deprecated_call", "fail"}
CLEANUP = {"rmtree", "unlink", "remove", "rmdir", "removedirs", "addCleanup", "addfinalizer", "cleanup"}
TEMP_CREATE = {"mkdtemp", "mkstemp"}
# ast.Match only exists on Python 3.10+; older interpreters just lack the node.
CONDITIONAL_NODES = tuple(getattr(ast, n) for n in ("If", "Try", "Match") if hasattr(ast, n))


def dotted(node):
    """Dotted name of a Name/Attribute chain ('pytest.mark.skip'), stripping
    calls so a decorator like @pytest.mark.skip(reason=...) resolves too."""
    if isinstance(node, ast.Call):
        return dotted(node.func)
    parts = []
    while isinstance(node, ast.Attribute):
        parts.append(node.attr)
        node = node.value
    if isinstance(node, ast.Name):
        parts.append(node.id)
    return ".".join(reversed(parts))


def last(name):
    return name.rsplit(".", 1)[-1]


def decorator_facts(decorators):
    skipped = expected_fail = False
    for d in decorators:
        name = dotted(d)
        tail = last(name)
        # skipif/skipUnless are conditional (environment guards), not
        # disabled tests, so only the unconditional forms count.
        if tail == "skip":
            skipped = True
        elif tail in ("xfail", "expectedFailure"):
            expected_fail = True
    return skipped, expected_fail


def analyze(func, inherited_skip, inherited_xfail):
    asserts = interactions = 0
    skipped, expected_fail = decorator_facts(func.decorator_list)
    skipped = skipped or inherited_skip
    expected_fail = expected_fail or inherited_xfail
    created_temp = cleaned = False

    def visit(node, conditional):
        nonlocal asserts, interactions, skipped, created_temp, cleaned
        if isinstance(node, ast.Assert):
            asserts += 1
            if any(isinstance(n, ast.Attribute) and n.attr in MOCK_ATTRS for n in ast.walk(node.test)):
                interactions += 1
        elif isinstance(node, ast.Call):
            name = dotted(node.func)
            tail = last(name)
            if tail in MOCK_ASSERTS:
                asserts += 1
                interactions += 1
            elif tail.lower().startswith("assert") or HELPER.match(tail):
                asserts += 1
            elif tail in PYTEST_ASSERTERS and name.startswith("pytest."):
                asserts += 1
            elif name in ("pytest.skip", "self.skipTest") and not conditional:
                skipped = True
            elif tail in TEMP_CREATE:
                created_temp = True
            elif tail in CLEANUP:
                cleaned = True
        elif isinstance(node, ast.Try) and node.finalbody:
            cleaned = True
        nested = conditional or isinstance(node, CONDITIONAL_NODES)
        for child in ast.iter_child_nodes(node):
            visit(child, nested)

    for stmt in func.body:
        visit(stmt, False)

    return {
        "name": func.name,
        "line": func.lineno,
        "assertions": asserts,
        "interactions": interactions,
        "skipped": skipped,
        "expected_failure": expected_fail,
        "uncleaned_temp": created_temp and not cleaned,
    }


def scan_file(path):
    with open(path, encoding="utf-8") as f:
        tree = ast.parse(f.read(), filename=path)
    out = []
    for node in tree.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name.startswith("test"):
            out.append(analyze(node, False, False))
        elif isinstance(node, ast.ClassDef) and (node.name.startswith("Test") or node.bases):
            cls_skip, cls_xfail = decorator_facts(node.decorator_list)
            for member in node.body:
                if isinstance(member, (ast.FunctionDef, ast.AsyncFunctionDef)) and member.name.startswith("test"):
                    fact = analyze(member, cls_skip, cls_xfail)
                    fact["name"] = node.name + "." + member.name
                    out.append(fact)
    return out


def main():
    root = os.getcwd()
    results = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIRS and not d.startswith("."))
        for fn in sorted(filenames):
            if not (fn.startswith("test_") or fn.endswith("_test.py")) or not fn.endswith(".py"):
                continue
            path = os.path.join(dirpath, fn)
            for fact in scan_file(path):
                fact["file"] = os.path.abspath(path)
                results.append(fact)
    json.dump(results, sys.stdout)


if __name__ == "__main__":
    main()
