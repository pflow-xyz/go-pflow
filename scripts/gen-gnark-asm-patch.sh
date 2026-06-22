#!/usr/bin/env bash
# gen-gnark-asm-patch.sh — regenerate bazel/patches/gnark-crypto-asm-hermetic.patch
#
# F4 Tier 2: make gnark-crypto's amd64/arm64 assembly build hermetically under
# Bazel, so `bazel build` compiles the SAME field backend `make build` ships
# (instead of falling back to -tags purego).
#
# Root cause: each ecc/<curve>/<fp|fr>/element_<arch>.s does a relative
# cross-package `#include "../../../field/asm/element_Nw/element_Nw_<arch>.s"`.
# The included file lives in a separate Bazel package (a go-mod-vendor hack,
# gnark-crypto issue #619) that rules_go neither stages as an asm include nor
# can have wired into the consuming package's srcs (go_deps' gazelle drops
# loose .h files). Fix: inline the included file's content directly into the
# consuming .s — no new files, no BUILD/srcs changes, no gazelle dependency.
#
# The patch is applied via go_deps.module_override in MODULE.bazel. Re-run this
# after bumping gnark-crypto (the generated asm changes each release).
#
# Usage: scripts/gen-gnark-asm-patch.sh [path-to-gnark-crypto-source]
#   default source: $(go env GOMODCACHE)/github.com/consensys/gnark-crypto@<ver>
set -euo pipefail

cd "$(dirname "$0")/.."
repo_root="$(pwd)"
out="$repo_root/bazel/patches/gnark-crypto-asm-hermetic.patch"

# Resolve the gnark-crypto module source.
src="${1:-}"
if [ -z "$src" ]; then
	ver="$(go list -m -f '{{.Version}}' github.com/consensys/gnark-crypto)"
	src="$(go env GOMODCACHE)/github.com/consensys/gnark-crypto@${ver}"
fi
[ -d "$src" ] || { echo "gnark-crypto source not found: $src" >&2; exit 1; }
echo "source: $src"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cp -r "$src" "$work/orig"
cp -r "$src" "$work/new"
chmod -R u+w "$work/new" "$work/orig"

# Matches every cross-package asm include, regardless of relative prefix:
#   ecc/<curve>/<fp|fr>/  uses  ../../../field/asm/element_Nw/...
#   field/{babybear,koalabear}/ uses  ../asm/element_Nw/...
inc_re='#include "[^"]*asm/element_[0-9]+[a-z]+/element_'

count=0
while IFS= read -r f; do
	rel="${f#"$work/new/"}"
	# The single cross-package include directive in this file.
	inc_line="$(grep -m1 -E "$inc_re" "$f")"
	inc_path="$(printf '%s\n' "$inc_line" | sed -E 's/.*#include "([^"]+)".*/\1/')"
	# Resolve relative to the .s file's directory.
	abs_inc="$(cd "$(dirname "$f")" && cd "$(dirname "$inc_path")" && pwd)/$(basename "$inc_path")"
	[ -f "$abs_inc" ] || { echo "missing include target for $rel: $abs_inc" >&2; exit 1; }
	# Replace the include line with the included file's literal content.
	python3 - "$f" "$abs_inc" "$inc_line" <<'PY'
import sys
f, inc, line = sys.argv[1], sys.argv[2], sys.argv[3]
src = open(f).read()
body = open(inc).read()
marker = "// --- F4 Tier2: inlined %s (was: %s) ---\n" % (inc.split("field/asm/")[-1], line.strip())
assert line in src, "include line vanished in %s" % f
open(f, "w").write(src.replace(line, marker + body))
PY
	count=$((count + 1))
done < <(grep -rlE "$inc_re" "$work/new")

echo "inlined $count assembly files"

# Produce a module-root-relative unified diff (patch_strip = 1). Normalize the
# a/ b/ prefixes and zero the mtime stamps so the patch is byte-deterministic
# across runs (only real content changes show up after a gnark-crypto bump).
epoch='1970-01-01 00:00:00.000000000 +0000'
( cd "$work" && diff -ruN orig new || true ) \
	| sed -E "s|^(--- )orig/([^\t]*)\t.*|\1a/\2\t${epoch}|; s|^(\+\+\+ )new/([^\t]*)\t.*|\1b/\2\t${epoch}|" > "$out"

echo "wrote $out ($(wc -l < "$out") lines)"
