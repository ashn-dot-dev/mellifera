#!/bin/sh
#
# Usage:
#   /path/to/mellifera$ sh tools/validate-dump-compatibility.sh
set -e
export LC_ALL=C.UTF-8

make build-go >/dev/null

MELLIFERA_HOME=$(pwd)
MELLIFERA_PROG_PY="${MELLIFERA_HOME}/mf.py"
MELLIFERA_PROG_GO="${MELLIFERA_HOME}/bin/mf"

TMPDIR=$(mktemp -d)
trap '{ { set +x; } 2>/dev/null; rm -rf -- "${TMPDIR}"; }' EXIT

echo 'VALIDATE TOKEN DUMP COMPATIBILITY...'
set -x
"${MELLIFERA_PROG_PY}" --dump-tokens tools/validate-dump-compatibility.mf >"${TMPDIR}/dump-tokens.py.comb"
"${MELLIFERA_PROG_GO}" --dump-tokens tools/validate-dump-compatibility.mf >"${TMPDIR}/dump-tokens.go.comb"
diff "${TMPDIR}/dump-tokens.py.comb" "${TMPDIR}/dump-tokens.go.comb"
{ set +x; } 2>/dev/null

echo 'VALIDATE AST DUMP COMPATIBILITY...'
set -x
"${MELLIFERA_PROG_PY}" --dump-ast tools/validate-dump-compatibility.mf >"${TMPDIR}/dump-ast.py.comb"
"${MELLIFERA_PROG_GO}" --dump-ast tools/validate-dump-compatibility.mf >"${TMPDIR}/dump-ast.go.comb"
diff "${TMPDIR}/dump-tokens.py.comb" "${TMPDIR}/dump-tokens.go.comb"
{ set +x; } 2>/dev/null

echo 'PASSED'
