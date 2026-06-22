#!/usr/bin/env bash
# zk-parity-check.sh — falsifiable check that gnark-crypto's purego and asm
# field backends agree (F4).
#
# Bazel builds petri-pilot with `-tags purego` (pure-Go field arithmetic), but
# `make build` ships the amd64/arm64 assembly fast path. The .bazelrc asserts
# they are bit-identical. This builds cmd/zk-field-parity both ways and diffs
# the deterministic digests it emits. Identical output => the backends agree on
# every surface a divergence would corrupt (raw Fp/Fr ops, native MiMC, R1CS,
# solved witness). A mismatch fails loudly and prints the diverging section.
#
# Usage: scripts/zk-parity-check.sh   (run from the repo root)
set -euo pipefail

cd "$(dirname "$0")/.."

pkg=./cmd/zk-field-parity
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "building purego backend..."
go build -tags purego -o "$tmp/parity_purego" "$pkg"
echo "building asm backend..."
go build -o "$tmp/parity_asm" "$pkg"

out_purego="$("$tmp/parity_purego")"
out_asm="$("$tmp/parity_asm")"

echo
echo "purego:"; echo "$out_purego" | sed 's/^/  /'
echo "asm:";    echo "$out_asm"    | sed 's/^/  /'
echo

if [ "$out_purego" = "$out_asm" ]; then
	echo "PASS: purego and asm field backends produce identical digests"
	exit 0
fi

echo "FAIL: purego and asm field backends DIVERGE" >&2
diff <(echo "$out_purego") <(echo "$out_asm") >&2 || true
exit 1
