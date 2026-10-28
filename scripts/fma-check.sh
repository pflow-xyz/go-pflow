#!/usr/bin/env bash
#
# fma-check.sh — prove the deterministic core compiles to no fused
# multiply-add on arm64.
#
# The Go spec lets a compiler fuse x*y + z into one rounding, across
# statements, and the arm64 backend does; amd64 (GOAMD64=v1) does not. A fused
# product rounds differently, so the byte-exact paths shared with pflow-rs and
# pflow-xyz (the ODE solvers, the portable SSA and SDE, crmath.Pow) wrap every
# product that feeds an add in float64(...). This compiles those packages for
# arm64 and fails if any FMADD/FMSUB/FNMADD/FNMSUB survives, naming the line.
#
#   ./scripts/fma-check.sh [package ...]    default: ./crmath ./solver ./stochastic
set -euo pipefail
cd "$(dirname "$0")/.."
pkgs=("$@")
[ ${#pkgs[@]} -gt 0 ] || pkgs=(./crmath ./solver ./stochastic)
out=$(GOARCH=arm64 go build -gcflags=-S "${pkgs[@]}" 2>&1 | grep -E '\b(FMADDD|FMSUBD|FNMADDD|FNMSUBD)\b' || true)
if [ -n "$out" ]; then
  echo "fused multiply-adds on arm64 in ${pkgs[*]}:" >&2
  echo "$out" | grep -oE '\([^)]*\.go:[0-9]+\)' | sort -u >&2
  exit 1
fi
echo "fma-check: no fused multiply-add in ${pkgs[*]} (arm64)"
