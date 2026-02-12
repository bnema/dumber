#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

denied_patterns=(
  'import "C"'
  '^//export '
)

search_paths=(
  cmd
  internal
  pkg
  scripts
)

for pattern in "${denied_patterns[@]}"; do
  if command -v rg >/dev/null 2>&1; then
    if rg -n --glob '*.go' "$pattern" "${search_paths[@]}" >/dev/null; then
      echo "Forbidden pattern detected: $pattern"
      rg -n --glob '*.go' "$pattern" "${search_paths[@]}"
      exit 1
    fi
    continue
  fi

  if grep -RInE --include='*.go' "$pattern" "${search_paths[@]}" >/dev/null; then
    echo "Forbidden pattern detected: $pattern"
    grep -RInE --include='*.go' "$pattern" "${search_paths[@]}"
    exit 1
  fi
done

echo "verify_purego_only: OK"
