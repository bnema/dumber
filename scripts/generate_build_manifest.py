#!/usr/bin/env python3
"""Generate the build manifest required by scripts/collect_first_presentation.sh.

The collector binds the measured binary to its source and dependency
revisions via `go version -m` plus this manifest, and fails closed when
attribution is missing or mismatched. Generate the manifest at build time
from the same tree that produced the binary:

    python3 scripts/generate_build_manifest.py --binary dist/dumber
    DUMBER_CEF_DIR=/path/to/cef-runtime scripts/collect_first_presentation.sh

Manifest schema (JSON, sorted keys):
    {
      "binary_sha256": "<hex sha256 of the measured binary>",
      "source_revision": "<40-hex git revision of the built source>",
      "modules": {
        "<module path>": {"version": "<exact version>", "revision": "<40-hex>"},
        ...
      }
    }
"""

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys

UPSTREAM_MODULE = "github.com/bnema/purego-cef2gtk"
VERSION_RE = r"(v\d+\.\d+\.\d+)-0\.\d{14}-([0-9a-f]{12})"
TAG_RE = r"v\d+\.\d+\.\d+"


def run(args, **kwargs):
    return subprocess.check_output(args, text=True, **kwargs).strip()


def fail(message):
    print("generate-build-manifest: {}".format(message), file=sys.stderr)
    raise SystemExit(2)


def main():
    parser = argparse.ArgumentParser(description="Generate a collector build manifest.")
    parser.add_argument("--binary", required=True, help="Measured binary to attribute.")
    parser.add_argument("--output", default=None, help="Manifest path (default: <binary>.manifest.json).")
    parser.add_argument("--source-revision", default=None,
                        help="40-hex source revision (default: git rev-parse HEAD).")
    parser.add_argument("--module", default=UPSTREAM_MODULE, help="Dependency module to bind.")
    args = parser.parse_args()

    if not os.path.isfile(args.binary):
        fail("binary not found")
    output = args.output or (args.binary + ".manifest.json")

    with open(args.binary, "rb") as candidate:
        # Chunked streaming hash: hashlib.file_digest needs 3.11+, this
        # stays compatible with older 3.x interpreters.
        digest = hashlib.sha256()
        for chunk in iter(lambda: candidate.read(65536), b""):
            digest.update(chunk)
        binary_sha256 = digest.hexdigest()

    try:
        buildinfo = run(["go", "version", "-m", args.binary], stderr=subprocess.DEVNULL)
    except (OSError, subprocess.CalledProcessError):
        fail("could not read embedded build info")
    if "=>" in buildinfo:
        fail("binary was built with a replacement; refusing to attribute it")
    dep_version = None
    vcs_revision = None
    vcs_modified = None
    for line in buildinfo.splitlines():
        parts = line.split()
        if len(parts) >= 3 and parts[0] == "dep" and parts[1] == args.module:
            dep_version = parts[2]
        # `go version -m` reports stamping as "build vcs.revision=<hex>"
        # (two tab-separated fields, key=value form).
        elif len(parts) >= 2 and parts[0] == "build" and parts[1].startswith("vcs."):
            key, _, value = parts[1].partition("=")
            if key == "vcs.revision":
                vcs_revision = value
            elif key == "vcs.modified":
                vcs_modified = value
    # The measured binary, not the ambient checkout, is the provenance
    # source: build-manifest builds with VCS stamping enabled, so the
    # embedded revision describes the exact built tree. Binaries without
    # stamping (for example -buildvcs=false quick builds) or built from a
    # dirty tree are rejected instead of inheriting the checkout HEAD.
    if vcs_revision is None or not re.fullmatch(r"[0-9a-f]{40}", vcs_revision):
        fail("binary lacks embedded VCS revision; rebuild with VCS stamping enabled")
    if vcs_modified != "false":
        fail("binary was built from a dirty checkout; refusing to attribute it")
    if dep_version is None:
        fail("dependency {} not found in build info".format(args.module))
    pseudo = re.fullmatch(VERSION_RE, dep_version)
    tag = re.fullmatch(TAG_RE, dep_version)
    if not pseudo and not tag:
        fail("dependency version is not an immutable tag or pseudo-version")

    try:
        downloaded = json.loads(run(["go", "mod", "download", "-json", "{}@{}".format(args.module, dep_version)],
                                    stderr=subprocess.DEVNULL))
    except (OSError, subprocess.CalledProcessError, ValueError):
        fail("could not resolve dependency origin")
    origin = downloaded.get("Origin") or {}
    revision = origin.get("Hash", "")
    if origin.get("VCS") != "git" or not re.fullmatch(r"[0-9a-f]{40}", revision):
        fail("dependency origin is not an immutable git hash")
    if pseudo and not revision.startswith(pseudo.group(2)):
        fail("dependency revision does not match its pseudo-version")

    source_revision = args.source_revision or vcs_revision
    if not re.fullmatch(r"[0-9a-f]{40}", source_revision or ""):
        fail("source revision must be a 40-hex git revision")
    if source_revision != vcs_revision:
        fail("source revision does not match the revision embedded in the binary")

    manifest = {
        "binary_sha256": binary_sha256,
        "source_revision": source_revision,
        "modules": {
            args.module: {"version": dep_version, "revision": revision},
        },
    }
    with open(output, "w", encoding="utf-8") as handle:
        json.dump(manifest, handle, indent=2, sort_keys=True)
        handle.write("\n")
    print("manifest: {}".format(output))


if __name__ == "__main__":
    main()
