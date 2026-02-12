#!/usr/bin/env bash
set -euo pipefail

# Use the directory containing this script's parent as repo root,
# avoiding git which fails in CI with "dubious ownership" errors.
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

denied_patterns=(
  'import "C"'
  '^//export '
)

all_paths=(cmd internal pkg scripts)
search_paths=()
for p in "${all_paths[@]}"; do
  [ -d "$p" ] && search_paths+=("$p")
done

if [ ${#search_paths[@]} -eq 0 ]; then
  echo "verify_purego_only: no source directories found"
  exit 1
fi

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
