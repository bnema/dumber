#!/usr/bin/env python3
"""Build baseline and candidate Dumber binaries for the isolated render lab.

The baseline is built from a pristine checkout with GOWORK=off. The candidate is
built either in a generated, ignored workspace that links the candidate Dumber
tree to a local bridge worktree (iteration mode), or against an immutable
published bridge revision (final handoff mode, --published-bridge-sha).

    python3 scripts/render_lab_build.py \
        --baseline-repo ../baseline/main \
        --candidate-repo . \
        --bridge-repo ../../purego-cef2gtk/.worktrees/perf/render-observability

The script writes a local `render-lab-v1` manifest next to each binary. It never
installs anything, never edits committed files, and refuses to attribute a build
it cannot verify. This is deliberately not the cold-start build-manifest
generator used for releases.

Exit codes: 0 success, 1 failure, 2 missing prerequisite.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

SCHEMA = "render-lab-v1"
BRIDGE_MODULE = "github.com/bnema/purego-cef2gtk"
BINARY_RELPATH = Path("dist/render-lab")
BUILD_DIR = Path("dist/render-lab")
FIXTURE_PATH = Path("scripts/render_lab/scroll.html")

SOURCE_EXCLUDES = (
    "dist/",
    ".worktrees/",
    "vendor/",
)
ASSET_PATHS = (
    "assets/systemviews/systemviews.wasm",
    "assets/systemviews/systemviews.wasm.br",
    "assets/systemviews/wasm_exec.js",
    "assets/systemviews/asset-manifest.json",
)
CANDIDATE_STATUSES = ("instrumentation-only", "ownership-verified-candidate")


class BuildError(Exception):
    """Failure that maps to exit code 1."""


class PrerequisiteError(BuildError):
    """Missing prerequisite that maps to exit code 2."""


def run(args: list[str], *, cwd: Path, env: dict | None = None, check: bool = True) -> str:
    completed = subprocess.run(
        args,
        cwd=str(cwd),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        check=False,
    )
    output = completed.stdout or ""
    if check and completed.returncode != 0:
        raise BuildError(
            f"command failed ({completed.returncode}): {' '.join(args)}\n{output.strip()}"
        )
    return output


def git(repo: Path, *args: str) -> str:
    """Run git and return raw stdout; callers choose their own stripping."""
    return run(["git", *args], cwd=repo).rstrip("\n")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def path_label(repo: Path) -> dict:
    """Repo-relative, sanitized identity for a repository root."""
    return {
        "name": repo.name,
        "head": git(repo, "rev-parse", "HEAD"),
        "path_sha256": hashlib.sha256(str(repo.resolve()).encode()).hexdigest()[:16],
    }


def source_snapshot(repo: Path) -> dict[str, str]:
    """Digest every uncommitted non-ignored source path, using NUL-framed output.

    `-z` keeps leading status columns, spaces and renames unambiguous; the text
    format collapses a leading " D" and would misattribute deleted paths.
    """
    entries: dict[str, str] = {}
    fields = git(repo, "status", "--porcelain", "-uall", "-z", "--", ".").split("\0")
    index = 0
    while index < len(fields):
        entry = fields[index]
        index += 1
        if not entry:
            continue
        status = entry[:2]
        relpath = entry[3:]
        if status[0] in ("R", "C"):
            # Rename/copy entries carry the source path as the next field.
            index += 1
        if not relpath:
            continue
        if any(relpath.startswith(prefix) for prefix in SOURCE_EXCLUDES):
            continue
        candidate = repo / relpath
        if not candidate.is_file():
            entries[relpath] = "deleted"
            continue
        entries[relpath] = sha256_file(candidate)
    return entries


def ensure_pristine_baseline(repo: Path) -> None:
    dirty = git(repo, "status", "--porcelain", "-uall")
    if dirty.strip():
        raise BuildError(
            "baseline repository is not pristine; refusing to build an attributed baseline\n"
            + dirty
        )


def module_directive(repo: Path, directive: str) -> str:
    for line in (repo / "go.mod").read_text(encoding="utf-8").splitlines():
        if line.startswith(directive + " "):
            return line.split(None, 1)[1].strip()
    return ""


def write_workspace(candidate_repo: Path, bridge_repo: Path) -> Path:
    workspace_dir = candidate_repo / BUILD_DIR
    workspace_dir.mkdir(parents=True, exist_ok=True)
    go_directive = module_directive(candidate_repo, "go") or "1.26"
    toolchain = module_directive(candidate_repo, "toolchain")
    lines = [f"go {go_directive}", ""]
    if toolchain:
        lines.insert(1, f"toolchain {toolchain}")
        lines.insert(2, "")
    lines.extend(
        [
            "use (",
            f"\t{candidate_repo.resolve()}",
            f"\t{bridge_repo.resolve()}",
            ")",
            "",
        ]
    )
    path = workspace_dir / "go.work"
    path.write_text("\n".join(lines), encoding="utf-8")
    return path


def build_environment(gowork: Path | None) -> dict:
    env = dict(os.environ)
    env["GOWORK"] = str(gowork) if gowork is not None else "off"
    env.pop("PUREGO_CEF2GTK_CEF_RUNTIME_SMOKE", None)
    return env


def ensure_assets(repo: Path, env: dict) -> list[dict]:
    check = subprocess.run(
        ["go", "run", "./cmd/systemviews-assets", "-check-if-present", "-dir", "assets/systemviews"],
        cwd=str(repo),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        check=False,
    )
    if check.returncode != 0:
        run(["make", "build-systemviews"], cwd=repo, env=env)
    artifacts = []
    for relpath in ASSET_PATHS:
        path = repo / relpath
        if not path.is_file():
            raise BuildError(f"required build asset missing after generation: {relpath}")
        artifacts.append({"path": relpath, "sha256": sha256_file(path), "size": path.stat().st_size})
    return artifacts


def build_command(env: dict) -> list[str]:
    """Make command for one variant; workspace mode forbids `-mod=mod`."""
    command = ["make", "build-quick", "BUILDVCS=true"]
    gowork = env.get("GOWORK", "off")
    if gowork not in ("", "off"):
        # Go rejects -mod=mod inside a workspace; keep the module graph read-only
        # so a build can never rewrite a module's go.mod. No go.work.sum entry is
        # needed here because every selected module already carries its sums.
        command.append("GOFLAGS=-mod=readonly")
    return command


def build_binary(repo: Path, env: dict, log_path: Path) -> Path:
    log_path.parent.mkdir(parents=True, exist_ok=True)
    completed = subprocess.run(
        build_command(env),
        cwd=str(repo),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        check=False,
    )
    log_path.write_text(completed.stdout or "", encoding="utf-8")
    if completed.returncode != 0:
        raise BuildError(f"build failed in {repo.name}; see {log_path}")
    binary = repo / "dist" / "dumber"
    if not binary.is_file():
        raise BuildError(f"build reported success but {binary} is missing")
    return binary


def module_selection(repo: Path, env: dict) -> dict:
    output = run(["go", "list", "-m", "-json", BRIDGE_MODULE], cwd=repo, env=env)
    try:
        info = json.loads(output)
    except json.JSONDecodeError as err:
        raise BuildError(f"unparseable go list -m output: {err}") from err
    return {
        "path": info.get("Path"),
        "version": info.get("Version", ""),
        "dir_label": Path(info["Dir"]).name if info.get("Dir") else None,
        "dir_sha256": (
            hashlib.sha256(str(Path(info["Dir"]).resolve()).encode()).hexdigest()[:16]
            if info.get("Dir")
            else None
        ),
        "replace": info.get("Replace", {}).get("Path") if info.get("Replace") else None,
    }


def buildinfo_module(binary: Path) -> dict:
    """Extract the bridge dependency and any replacement recorded in the binary.

    `go version -m` prints a `dep` line per dependency and, when the module was
    substituted, a following `=>` line whose first field is the local path and
    optional second field `(devel)`.
    """
    output = run(["go", "version", "-m", str(binary)], cwd=binary.parent)
    version = ""
    replaced_by = None
    for line in output.splitlines():
        fields = [field for field in line.split("\t") if field != ""]
        if not fields:
            continue
        if fields[0] == "dep":
            if len(fields) >= 3 and fields[1] == BRIDGE_MODULE:
                version = fields[2]
            continue
        if fields[0] == "=>" and len(fields) >= 2:
            candidate = fields[1]
            if candidate.startswith("("):
                continue
            replaced_by = candidate
    return {
        "version": version,
        "replaced_by_label": Path(replaced_by).name if replaced_by else None,
        "replaced_by_sha256": (
            hashlib.sha256(str(Path(replaced_by).resolve()).encode()).hexdigest()[:16]
            if replaced_by
            else None
        ),
    }


def resolve_published_version(repo: Path, env: dict, sha: str) -> str:
    output = run(["go", "list", "-m", "-json", BRIDGE_MODULE], cwd=repo, env=env)
    info = json.loads(output)
    if info.get("Replace"):
        raise BuildError("published mode must not use a replace directive")
    version = info.get("Version", "")
    if not version:
        raise BuildError("published mode resolved an empty module version")
    if len(sha) == 40 and version.endswith(sha[:12]):
        return version
    # Fall back to the module cache download record, which binds version to Git SHA.
    download = run(
        ["go", "mod", "download", "-json", f"{BRIDGE_MODULE}@{version}"], cwd=repo, env=env
    )
    try:
        record = json.loads(download)
    except json.JSONDecodeError as err:
        raise BuildError(f"unparseable go mod download output: {err}") from err
    origin_hash = (record.get("Origin") or {}).get("Hash", "")
    if origin_hash and origin_hash != sha:
        raise BuildError(
            f"pinned bridge revision mismatch: go.mod resolves {version} to {origin_hash[:12]}"
        )
    return version


def write_manifest(destination: Path, manifest: dict) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def read_recorded_asset_hashes(variant_dir: Path) -> list[dict] | None:
    """Reuse the recorded baseline asset hashes when the baseline is not rebuilt."""
    manifest_path = variant_dir / "manifest.json"
    if not manifest_path.is_file():
        return None
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return None
    assets = manifest.get("assets")
    return assets if isinstance(assets, list) else None


def toolchain_record(repo: Path) -> dict:
    return {
        "go_directive": module_directive(repo, "go"),
        "toolchain_directive": module_directive(repo, "toolchain"),
        "go_version": run(["go", "version"], cwd=repo).strip(),
    }


def build_variant(
    *,
    variant: str,
    status: str,
    build_repo: Path,
    artifact_repo: Path,
    bridge_repo: Path,
    env: dict,
    linkage: dict,
    expected_bridge_dir: Path | None,
    pinned_version: str | None,
    fixture_sha256: str,
    baseline_asset_hashes: list[dict] | None,
    notes: list[str],
) -> dict:
    binary_relpath = BINARY_RELPATH / variant / "dumber"
    destination = artifact_repo / binary_relpath

    before = source_snapshot(build_repo)
    bridge_before = source_snapshot(bridge_repo)
    assets = ensure_assets(build_repo, env)
    binary = build_binary(
        build_repo, env, artifact_repo / BUILD_DIR / variant / "build.log"
    )
    after = source_snapshot(build_repo)
    bridge_after = source_snapshot(bridge_repo)
    if before != after or bridge_before != bridge_after:
        raise BuildError(
            f"{variant}: source changed while building; refusing to attribute this binary"
        )

    selection = module_selection(build_repo, env)
    if expected_bridge_dir is not None:
        expected = hashlib.sha256(str(expected_bridge_dir.resolve()).encode()).hexdigest()[:16]
        if selection["dir_sha256"] != expected:
            raise BuildError(
                f"{variant}: go list resolved the bridge module outside the expected worktree"
            )
    if pinned_version is not None:
        if selection["version"] != pinned_version or selection["replace"]:
            raise BuildError(
                f"{variant}: expected pinned version {pinned_version} without replace, "
                f"got {selection['version']!r} replace={selection['replace']!r}"
            )
    buildinfo = buildinfo_module(binary)
    if expected_bridge_dir is not None and buildinfo["replaced_by_sha256"] is not None:
        expected = hashlib.sha256(str(expected_bridge_dir.resolve()).encode()).hexdigest()[:16]
        if buildinfo["replaced_by_sha256"] != expected:
            raise BuildError(f"{variant}: binary build info points at an unexpected bridge tree")

    if destination.is_file():
        existing = sha256_file(destination)
        incoming = sha256_file(binary)
        if existing != incoming and variant == "baseline":
            raise BuildError(
                "baseline is already frozen at "
                f"{existing[:12]} and this build produced {incoming[:12]}. "
                "Builds are not byte-identical because the linker stamps a build "
                "date, so the frozen baseline is kept until it is replaced on "
                f"purpose: delete {destination} and any runs that used it, then "
                "rebuild. Use --only candidate for ordinary iteration."
            )
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(binary, destination)
    patch_dir = artifact_repo / BUILD_DIR / variant
    (patch_dir / "source-tracked-dumber.patch").write_text(
        git(build_repo, "diff", "HEAD"), encoding="utf-8"
    )
    (patch_dir / "source-tracked-bridge.patch").write_text(
        git(bridge_repo, "diff", "HEAD"), encoding="utf-8"
    )

    manifest = {
        "schema": SCHEMA,
        "variant": variant,
        "status": status,
        "binary_relpath": str(binary_relpath),
        "binary_sha256": sha256_file(destination),
        "repo_head": git(build_repo, "rev-parse", "HEAD"),
        "bridge_head": git(bridge_repo, "rev-parse", "HEAD"),
        "repo_label": path_label(build_repo),
        "bridge_label": path_label(bridge_repo),
        "linkage": linkage,
        "module_selection": selection,
        "binary_module": buildinfo,
        "toolchain": toolchain_record(build_repo),
        "flags": build_command(env),
        "assets": assets,
        "changed_sources": [
            {"repo": "dumber", "path": path, "sha256": digest}
            for path, digest in sorted(before.items())
        ]
        + [
            {"repo": "bridge", "path": path, "sha256": digest}
            for path, digest in sorted(bridge_before.items())
        ],
        "fixture_sha256": fixture_sha256,
        "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds"),
        "notes": notes,
    }
    write_manifest(patch_dir / "manifest.json", manifest)

    if baseline_asset_hashes is not None:
        baseline_map = {item["path"]: item["sha256"] for item in baseline_asset_hashes}
        candidate_map = {item["path"]: item["sha256"] for item in assets}
        if baseline_map != candidate_map:
            raise BuildError(
                "baseline and candidate systemviews assets differ; variants would not be comparable"
            )
    return manifest


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--baseline-repo", type=Path, required=True)
    parser.add_argument("--candidate-repo", type=Path, required=True)
    parser.add_argument("--bridge-repo", type=Path, required=True)
    parser.add_argument("--published-bridge-sha", default=None)
    parser.add_argument("--only", choices=("both", "baseline", "candidate"), default="both")
    parser.add_argument(
        "--candidate-status", choices=CANDIDATE_STATUSES, default="instrumentation-only"
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    try:
        return _run(parse_args(sys.argv[1:] if argv is None else argv))
    except PrerequisiteError as err:
        print(f"render_lab_build: blocked: {err}", file=sys.stderr)
        return 2
    except BuildError as err:
        print(f"render_lab_build: error: {err}", file=sys.stderr)
        return 1


def _run(args: argparse.Namespace) -> int:
    os.umask(0o077)
    baseline_repo = args.baseline_repo.resolve()
    candidate_repo = args.candidate_repo.resolve()
    bridge_repo = args.bridge_repo.resolve()
    for label, repo in (
        ("baseline", baseline_repo),
        ("candidate", candidate_repo),
        ("bridge", bridge_repo),
    ):
        if not repo.is_dir():
            raise PrerequisiteError(f"{label} repo directory does not exist: {repo}")
        try:
            inside = git(repo, "rev-parse", "--is-inside-work-tree")
        except BuildError:
            inside = ""
        if inside != "true":
            raise PrerequisiteError(f"{label} repo is not a git work tree: {repo}")

    fixture = candidate_repo / FIXTURE_PATH
    if not fixture.is_file():
        raise PrerequisiteError(f"fixture missing: {fixture}")
    fixture_digest = sha256_file(fixture)

    results: dict[str, dict] = {}
    baseline_assets: list[dict] | None = None

    if args.only in ("both", "baseline"):
        ensure_pristine_baseline(baseline_repo)
        env = build_environment(None)
        results["baseline"] = build_variant(
            variant="baseline",
            status="baseline",
            build_repo=baseline_repo,
            artifact_repo=candidate_repo,
            bridge_repo=bridge_repo,
            env=env,
            linkage={"mode": "gowork-off", "note": "pristine baseline built from its own go.mod"},
            expected_bridge_dir=None,
            pinned_version=None,
            fixture_sha256=fixture_digest,
            baseline_asset_hashes=None,
            notes=["baseline is immutable once written"],
        )
        baseline_assets = results["baseline"]["assets"]

    if baseline_assets is None:
        baseline_assets = read_recorded_asset_hashes(candidate_repo / BINARY_RELPATH / "baseline")

    if args.only in ("both", "candidate"):
        if args.published_bridge_sha:
            env = build_environment(None)
            pinned = resolve_published_version(candidate_repo, env, args.published_bridge_sha)
            linkage = {
                "mode": "published-pin",
                "bridge_pseudo_version": pinned,
                "requested_bridge_sha": args.published_bridge_sha,
            }
            notes = ["final handoff mode: workspace substitution disabled"]
        else:
            workspace = write_workspace(candidate_repo, bridge_repo)
            env = build_environment(workspace)
            linkage = {
                "mode": "workspace",
                "workspace_relpath": str(workspace.relative_to(candidate_repo)),
                "note": "iteration mode only; the binary depends on local worktree contents",
            }
            notes = ["local workspace build: not portable beyond this machine"]
        results["candidate"] = build_variant(
            variant="candidate",
            status=args.candidate_status,
            build_repo=candidate_repo,
            artifact_repo=candidate_repo,
            bridge_repo=bridge_repo,
            env=env,
            linkage=linkage,
            expected_bridge_dir=None if args.published_bridge_sha else bridge_repo,
            pinned_version=pinned if args.published_bridge_sha else None,
            fixture_sha256=fixture_digest,
            baseline_asset_hashes=baseline_assets,
            notes=notes,
        )

    for variant, manifest in results.items():
        print(f"{variant}: {manifest['status']} sha256={manifest['binary_sha256']}")
        print(f"  binary:   {candidate_repo / manifest['binary_relpath']}")
        print(f"  linkage:  {manifest['linkage']['mode']} bridge_head={manifest['bridge_head'][:12]}")
        print(f"  sources:  {len(manifest['changed_sources'])} changed path(s) recorded")
    return 0


if __name__ == "__main__":
    sys.exit(main())
