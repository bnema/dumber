#!/usr/bin/env python3
"""Report numeric CEF runtime version fields plus library hash.

Opens only the explicitly selected trusted libcef.so using the standard
library ctypes module and calls the header-verified cef_version_info(int)
entries from /usr/include/cef/include/cef_version_info.h:

  0 - CEF_VERSION_MAJOR
  1 - CEF_VERSION_MINOR
  2 - CEF_VERSION_PATCH
  3 - CEF_COMMIT_NUMBER
  4 - CHROME_VERSION_MAJOR
  5 - CHROME_VERSION_MINOR
  6 - CHROME_VERSION_BUILD
  7 - CHROME_VERSION_PATCH

Outputs JSON to stdout with numeric fields and the library SHA-256. Never
outputs filesystem paths. This probe is not an ABI bypass: the candidate must
still pass its normal loader ABI/version validation.
"""

import argparse
import ctypes
import hashlib
import json
import os
import sys


ENTRIES = (
    ("cef_version_major", 0),
    ("cef_version_minor", 1),
    ("cef_version_patch", 2),
    ("cef_commit_number", 3),
    ("chrome_version_major", 4),
    ("chrome_version_minor", 5),
    ("chrome_version_build", 6),
    ("chrome_version_patch", 7),
)


def fail(message):
    print("cef-runtime-probe: {}".format(message), file=sys.stderr)
    raise SystemExit(2)


def main():
    parser = argparse.ArgumentParser(description="Probe selected libcef.so version fields.")
    parser.add_argument("--cef-dir", required=True, help="Selected CEF runtime directory.")
    args = parser.parse_args()

    cef_dir = args.cef_dir
    if not cef_dir or not os.path.isabs(cef_dir):
        fail("cef dir must be an absolute path")
    lib_name = os.path.join(cef_dir, "libcef.so")
    try:
        is_file = os.path.isfile(lib_name) and not os.path.islink(lib_name)
    except OSError:
        fail("could not stat runtime library")
    if not is_file:
        fail("runtime library not found")

    try:
        with open(lib_name, "rb") as candidate:
            library_sha256 = hashlib.file_digest(candidate, "sha256").hexdigest()
    except OSError:
        fail("could not hash runtime library")

    try:
        lib = ctypes.CDLL(lib_name)
    except OSError:
        fail("could not open runtime library")
    try:
        version_info = lib.cef_version_info
    except AttributeError:
        fail("cef_version_info symbol is unavailable")
    version_info.argtypes = (ctypes.c_int,)
    version_info.restype = ctypes.c_int

    result = {}
    for field, entry in ENTRIES:
        try:
            value = version_info(entry)
        except (ctypes.ArgumentError, OSError, ValueError):
            fail("version query failed")
        if not isinstance(value, int) or value < 0:
            fail("version query failed")
        result[field] = value
    result["libcef_sha256"] = library_sha256
    json.dump(result, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
