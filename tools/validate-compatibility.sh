#!/bin/sh
#
# Usage:
#   /path/to/mellifera$ sh tools/validate-compatibility.sh
export LC_ALL=C.UTF-8

make build-go >/dev/null

MELLIFERA_HOME=$(pwd)
MELLIFERA_PROG_PY="${MELLIFERA_HOME}/mf.py"
MELLIFERA_PROG_GO="${MELLIFERA_HOME}/bin/mf"

TMPDIR=$(mktemp -d)
trap '{ { set +x; } 2>/dev/null; rm -rf -- "${TMPDIR}"; }' EXIT

FILESRUN=0
FAILURES=0

validate() {
    FILE="$1"
    FILESRUN=$((FILESRUN + 1))

    echo "[${FILE}] VALIDATE TOKEN DUMP COMPATIBILITY"
    "${MELLIFERA_PROG_PY}" --dump-tokens "${FILE}" >"${TMPDIR}/$(basename ${FILE}).dump-tokens.py.comb" 2>/dev/null
    "${MELLIFERA_PROG_GO}" --dump-tokens "${FILE}" >"${TMPDIR}/$(basename ${FILE}).dump-tokens.go.comb" 2>/dev/null
    if ! diff "${TMPDIR}/$(basename ${FILE}).dump-tokens.py.comb" "${TMPDIR}/$(basename ${FILE}).dump-tokens.go.comb"; then
        FAILURES=$((FAILURES + 1))
        return 1
    fi

    echo "[${FILE}] VALIDATE AST DUMP COMPATIBILITY"
    "${MELLIFERA_PROG_PY}" --dump-ast "${FILE}" >"${TMPDIR}/$(basename ${FILE}).dump-ast.py.comb" 2>/dev/null
    "${MELLIFERA_PROG_GO}" --dump-ast "${FILE}" >"${TMPDIR}/$(basename ${FILE}).dump-ast.go.comb" 2>/dev/null
    if ! diff "${TMPDIR}/$(basename ${FILE}).dump-ast.py.comb" "${TMPDIR}/$(basename ${FILE}).dump-ast.go.comb"; then
        FAILURES=$((FAILURES + 1))
        return 1
    fi
}

for f in overview.mf $(find examples tests -name '*.mf' | sort); do
    validate "${f}"
done

echo "FILES CHECKED => ${FILESRUN}"
echo "FAILURE COUNT => ${FAILURES}"

[ "${FAILURES}" -eq 0 ] || exit 1
