#!/usr/bin/env bash
set -euo pipefail

if ! command -v makepkg >/dev/null 2>&1; then
  echo "makepkg is required to verify AUR metadata" >&2
  exit 1
fi

for pkgdir in aur/dumber-browser-bin aur/dumber-browser-git; do
  tmp=$(mktemp)
  trap 'rm -f "$tmp"' EXIT
  (cd "$pkgdir" && makepkg --printsrcinfo > "$tmp")
  diff -u "$pkgdir/.SRCINFO" "$tmp"
  grep -qxE '[[:space:]]*depends = cef' "$pkgdir/.SRCINFO"
  grep -qxE '[[:space:]]*depends = cef' "$tmp"
  for metadata in "$pkgdir/.SRCINFO" "$tmp"; do
    if grep -qxE '[[:space:]]*depends = cef-vaapi(-bin)?([<>=].*)?' "$metadata"; then
      echo "cef-vaapi providers must remain optdepends, not hard dependencies: $metadata" >&2
      exit 1
    fi
  done
  grep -q "optdepends = cef-vaapi-bin: Prebuilt CEF provider with VA-API hardware video decoding support" "$pkgdir/.SRCINFO"
  grep -q "optdepends = cef-vaapi: Source-built CEF provider with VA-API hardware video decoding support" "$pkgdir/.SRCINFO"
  grep -q "optdepends = cef-vaapi-bin: Prebuilt CEF provider with VA-API hardware video decoding support" "$tmp"
  grep -q "optdepends = cef-vaapi: Source-built CEF provider with VA-API hardware video decoding support" "$tmp"
  rm -f "$tmp"
  trap - EXIT
  echo "verified $pkgdir"
done

expected="depends=('gtk4' 'cef' 'webkitgtk-6.0')"
matches=$(grep -F -c "$expected" .github/workflows/aur.yml || true)
if [[ "$matches" -lt 2 ]]; then
  echo "expected dependency string in both AUR workflow package blocks" >&2
  exit 1
fi

for expected in \
  "'cef-vaapi-bin: Prebuilt CEF provider with VA-API hardware video decoding support'" \
  "'cef-vaapi: Source-built CEF provider with VA-API hardware video decoding support'"; do
  matches=$(grep -F -c "$expected" .github/workflows/aur.yml || true)
  if [[ "$matches" -lt 2 ]]; then
    echo "expected $expected in both AUR workflow package blocks" >&2
    exit 1
  fi
done
