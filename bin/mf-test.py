#!/usr/bin/env python3
import argparse
import difflib
import os
import re
import pathlib
import subprocess
import sys

SEP = re.compile(r"^########(#*)$", re.MULTILINE)
BOL = re.compile(r"^(#[ ]?)?")
DIR = os.path.dirname(os.path.realpath(__file__))
MELLIFERA_HOME = os.environ.get("MELLIFERA_HOME", os.path.dirname(DIR))
MELLIFERA_PROG = f"{MELLIFERA_HOME}{os.path.sep}mf.py"
PYTHON_PROG = sys.executable

testsrun = 0
failures = 0


def test(path):
    global testsrun
    global failures

    print(f"[= TEST {path} =]")
    with open(path, "r", encoding="utf-8") as f:
        source = f.read()

    expected = []
    if match := SEP.search(source):
        lines = source[match.end() :].lstrip().splitlines(keepends=True)
        for line in lines:
            bol = BOL.match(line)
            assert bol is not None, "expected beginning-of-line regex match"
            expected.append(line[bol.end() :])

    received = subprocess.run(
        [PYTHON_PROG, MELLIFERA_PROG, os.path.basename(path)],
        cwd=os.path.dirname(path),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    ).stdout.splitlines(keepends=True)

    def diff():
        return difflib.unified_diff(expected, received)

    if len(list(diff())) == 0:
        status = "PASS"
    else:
        status = "FAIL"
        failures += 1
        sys.stdout.writelines(diff())
    print(f"[= {status} =]")
    testsrun += 1


parser = argparse.ArgumentParser()
parser.add_argument("files", type=str, nargs="*")
args = parser.parse_args()

if len(args.files) != 0:
    files = []
    for file in args.files:
        if os.path.isdir(file):
            files.extend(list(pathlib.Path(file).absolute().rglob("*.test.mf")))
        else:
            files.append(file)
else:
    files = list(pathlib.Path.cwd().rglob("*.test.mf"))

for file in files:
    test(file)

print(f"TESTS RUN => {testsrun}")
print(f"FAILURES  => {failures}")
sys.exit(0 if failures == 0 else 1)
