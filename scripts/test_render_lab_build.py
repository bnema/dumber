#!/usr/bin/env python3
"""Unit tests for scripts/render_lab_build.py.

Builds are faked: no Go toolchain, no Dumber build, and no network access are
required. The tests assert the provenance contract instead: workspace
generation, source snapshots, immutable baselines, module-identity verification,
and rejected ambiguous attribution.
"""

from __future__ import annotations

import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))

import render_lab_build  # noqa: E402
from test_render_lab import TimeoutTestCase  # noqa: E402

GIT_IDENTITY = (
    "-c",
    "user.name=render-lab-test",
    "-c",
    "user.email=render-lab-test@example.invalid",
)


def make_git_repo(root: Path, *, gitignore: str = "") -> Path:
    root.mkdir(parents=True, exist_ok=True)
    subprocess.run(["git", "init", "-q"], cwd=root, check=True)
    if gitignore:
        (root / ".gitignore").write_text(gitignore, encoding="utf-8")
    (root / "go.mod").write_text(
        "module example.invalid/lab\n\ngo 1.26.0\n\ntoolchain go1.26.7\n", encoding="utf-8"
    )
    subprocess.run(
        ["git", *GIT_IDENTITY, "add", "-A"], cwd=root, check=True, stdout=subprocess.DEVNULL
    )
    subprocess.run(
        ["git", *GIT_IDENTITY, "commit", "-q", "-m", "init"],
        cwd=root,
        check=True,
        stdout=subprocess.DEVNULL,
    )
    return root


class WorkspaceTest(TimeoutTestCase):
    def test_module_directives_are_read_from_go_mod(self) -> None:
        repo = make_git_repo(Path(tempfile.mkdtemp(prefix="lab-build-mod-")))
        self.assertEqual(render_lab_build.module_directive(repo, "go"), "1.26.0")
        self.assertEqual(render_lab_build.module_directive(repo, "toolchain"), "go1.26.7")

    def test_workspace_uses_absolute_paths_outside_source_files(self) -> None:
        candidate = make_git_repo(Path(tempfile.mkdtemp(prefix="lab-build-cand-")))
        bridge = make_git_repo(Path(tempfile.mkdtemp(prefix="lab-build-bridge-")))
        workspace = render_lab_build.write_workspace(candidate, bridge)
        text = workspace.read_text(encoding="utf-8")
        self.assertIn(f"\t{candidate.resolve()}", text)
        self.assertIn(f"\t{bridge.resolve()}", text)
        self.assertIn("go 1.26.0", text)
        self.assertIn("toolchain go1.26.7", text)
        self.assertTrue(workspace.is_relative_to(candidate))

    def test_build_environment_forces_explicit_gowork(self) -> None:
        workspace = Path("/tmp/lab-go.work")
        env = render_lab_build.build_environment(workspace)
        self.assertEqual(env["GOWORK"], str(workspace))
        self.assertNotIn("PUREGO_CEF2GTK_CEF_RUNTIME_SMOKE", env)
        self.assertEqual(render_lab_build.build_environment(None)["GOWORK"], "off")


class SourceSnapshotTest(TimeoutTestCase):
    def test_snapshot_lists_untracked_source_but_not_ignored_build_output(self) -> None:
        repo = make_git_repo(
            Path(tempfile.mkdtemp(prefix="lab-build-snap-")), gitignore="dist/\n"
        )
        (repo / "scripts").mkdir()
        (repo / "scripts" / "new_tool.py").write_text("print('lab')\n", encoding="utf-8")
        (repo / "dist" / "render-lab").mkdir(parents=True)
        (repo / "dist" / "render-lab" / "go.work").write_text("go 1.26.0\n", encoding="utf-8")

        snapshot = render_lab_build.source_snapshot(repo)
        self.assertIn("scripts/new_tool.py", snapshot)
        self.assertEqual(
            snapshot["scripts/new_tool.py"],
            render_lab_build.sha256_file(repo / "scripts" / "new_tool.py"),
        )
        self.assertFalse([path for path in snapshot if path.startswith("dist/")])

    def test_deleted_source_is_recorded_as_deleted(self) -> None:
        repo = make_git_repo(Path(tempfile.mkdtemp(prefix="lab-build-del-")))
        (repo / "go.mod").unlink()
        self.assertEqual(render_lab_build.source_snapshot(repo)["go.mod"], "deleted")

    def test_pristine_baseline_is_required(self) -> None:
        repo = make_git_repo(Path(tempfile.mkdtemp(prefix="lab-build-pristine-")))
        render_lab_build.ensure_pristine_baseline(repo)
        (repo / "untracked.txt").write_text("dirty\n", encoding="utf-8")
        with self.assertRaises(render_lab_build.BuildError):
            render_lab_build.ensure_pristine_baseline(repo)


class BuildVariantTest(TimeoutTestCase):
    """Exercises build_variant with faked assets, builds and module probes."""

    def setUp(self) -> None:
        super().setUp()
        self.candidate = make_git_repo(
            Path(tempfile.mkdtemp(prefix="lab-bv-cand-")), gitignore="dist/\n"
        )
        self.bridge = make_git_repo(Path(tempfile.mkdtemp(prefix="lab-bv-bridge-")))
        (self.candidate / "scripts").mkdir()
        (self.candidate / "scripts" / "render_lab.py").write_text("# lab\n", encoding="utf-8")
        self.assets = [
            {"path": "assets/systemviews/systemviews.wasm", "sha256": "a" * 64, "size": 1}
        ]
        self.binary_content = "binary-content-1\n"

    def _patches(self, *, selection: dict | None = None, buildinfo: dict | None = None):
        default_selection = {
            "path": render_lab_build.BRIDGE_MODULE,
            "version": "v0.10.2-0.20260911042437-e9e81371d3b8",
            "dir_label": "bridge",
            "dir_sha256": hashlib.sha256(str(self.bridge.resolve()).encode()).hexdigest()[:16],
            "replace": None,
        }
        default_buildinfo = {"version": "", "replaced_by_label": None, "replaced_by_sha256": None}

        def fake_build_binary(repo: Path, env: dict, log_path: Path) -> Path:
            log_path.parent.mkdir(parents=True, exist_ok=True)
            log_path.write_text("fake build log\n", encoding="utf-8")
            binary = repo / "dist" / "dumber"
            binary.parent.mkdir(parents=True, exist_ok=True)
            binary.write_text(self.binary_content, encoding="utf-8")
            binary.chmod(0o755)
            return binary

        return [
            mock.patch.object(render_lab_build, "ensure_assets", return_value=self.assets),
            mock.patch.object(render_lab_build, "build_binary", side_effect=fake_build_binary),
            mock.patch.object(
                render_lab_build, "module_selection", return_value=selection or default_selection
            ),
            mock.patch.object(
                render_lab_build, "buildinfo_module", return_value=buildinfo or default_buildinfo
            ),
            mock.patch.object(render_lab_build, "toolchain_record", return_value={"go_version": "x"}),
        ]

    def _build(self, **overrides):
        params = dict(
            variant="candidate",
            status="instrumentation-only",
            build_repo=self.candidate,
            artifact_repo=self.candidate,
            bridge_repo=self.bridge,
            env=render_lab_build.build_environment(None),
            linkage={"mode": "workspace"},
            expected_bridge_dir=self.bridge,
            pinned_version=None,
            fixture_sha256="c" * 64,
            baseline_asset_hashes=None,
            notes=["test"],
        )
        selection = overrides.pop("_selection", None)
        buildinfo = overrides.pop("_buildinfo", None)
        if "binary_content" in overrides:
            self.binary_content = overrides.pop("binary_content")
        params.update(overrides)
        for patch in self._patches(selection=selection, buildinfo=buildinfo):
            patch.start()
            self.addCleanup(patch.stop)
        return render_lab_build.build_variant(**params)

    def test_manifest_records_sanitized_provenance(self) -> None:
        manifest = self._build()
        manifest_path = self.candidate / "dist" / "render-lab" / "candidate" / "manifest.json"
        written = json.loads(manifest_path.read_text(encoding="utf-8"))
        self.assertEqual(written["schema"], render_lab_build.SCHEMA)
        self.assertEqual(written["variant"], "candidate")
        self.assertEqual(written["status"], "instrumentation-only")
        self.assertEqual(written["linkage"]["mode"], "workspace")
        self.assertEqual(written["fixture_sha256"], "c" * 64)
        paths = {entry["path"] for entry in written["changed_sources"]}
        self.assertIn("scripts/render_lab.py", paths)
        serialized = json.dumps(written)
        self.assertNotIn(str(Path.home()), serialized)
        self.assertNotIn(str(self.bridge.resolve()), serialized)
        self.assertEqual(manifest["binary_sha256"], written["binary_sha256"])
        self.assertEqual(
            (self.candidate / written["binary_relpath"]).read_text(encoding="utf-8"),
            "binary-content-1\n",
        )

    def test_baseline_refuses_overwrite_with_a_different_hash(self) -> None:
        first = self._build(variant="baseline", status="baseline", expected_bridge_dir=None)
        self.assertEqual(first["status"], "baseline")
        with self.assertRaises(render_lab_build.BuildError):
            self._build(
                variant="baseline",
                status="baseline",
                expected_bridge_dir=None,
                binary_content="tampered-baseline\n",
            )

    def test_identical_baseline_rebuild_is_allowed(self) -> None:
        self._build(variant="baseline", status="baseline", expected_bridge_dir=None)
        again = self._build(variant="baseline", status="baseline", expected_bridge_dir=None)
        self.assertEqual(again["status"], "baseline")

    def test_bridge_resolved_outside_expected_worktree_is_rejected(self) -> None:
        other = render_lab_build.hashlib.sha256(b"/somewhere/else").hexdigest()[:16]
        with self.assertRaises(render_lab_build.BuildError):
            self._build(
                _selection={
                    "path": render_lab_build.BRIDGE_MODULE,
                    "version": "v1.0.0",
                    "dir_label": "elsewhere",
                    "dir_sha256": other,
                    "replace": None,
                }
            )

    def test_pinned_mode_rejects_workspace_or_replace_linkage(self) -> None:
        with self.assertRaises(render_lab_build.BuildError):
            self._build(
                pinned_version="v0.10.2-0.20260911042437-e9e81371d3b8",
                _selection={
                    "path": render_lab_build.BRIDGE_MODULE,
                    "version": "v0.10.2-0.20260911042437-e9e81371d3b8",
                    "dir_label": "bridge",
                    "dir_sha256": None,
                    "replace": "/somewhere/else",
                },
            )

    def test_pinned_mode_accepts_exact_version_without_replace(self) -> None:
        pinned = "v0.10.2-0.20260911042437-e9e81371d3b8"
        manifest = self._build(
            pinned_version=pinned,
            expected_bridge_dir=None,
            linkage={"mode": "published-pin", "bridge_pseudo_version": pinned},
            _selection={
                "path": render_lab_build.BRIDGE_MODULE,
                "version": pinned,
                "dir_label": None,
                "dir_sha256": None,
                "replace": None,
            },
        )
        self.assertEqual(manifest["linkage"]["bridge_pseudo_version"], pinned)

    def test_pinned_mode_rejects_version_mismatch(self) -> None:
        with self.assertRaises(render_lab_build.BuildError):
            self._build(
                pinned_version="v0.10.2-0.20260911042437-aaaaaaaaaaaa",
                expected_bridge_dir=None,
                _selection={
                    "path": render_lab_build.BRIDGE_MODULE,
                    "version": "v0.10.2-0.20260911042437-e9e81371d3b8",
                    "dir_label": None,
                    "dir_sha256": None,
                    "replace": None,
                },
            )

    def test_asset_mismatch_between_variants_is_rejected(self) -> None:
        with self.assertRaises(render_lab_build.BuildError):
            self._build(baseline_asset_hashes=[dict(self.assets[0], sha256="f" * 64)])

    def test_source_change_during_build_is_rejected(self) -> None:
        original = render_lab_build.source_snapshot
        calls = {"count": 0}

        def flaky_snapshot(repo: Path):
            result = original(repo)
            if repo == self.candidate:
                calls["count"] += 1
                result = dict(result, **{f"late{calls['count']}.py": "0" * 64})
            return result

        with mock.patch.object(render_lab_build, "source_snapshot", side_effect=flaky_snapshot):
            with self.assertRaises(render_lab_build.BuildError):
                self._build()


class PublishedResolveTest(TimeoutTestCase):
    def test_replace_directive_is_rejected(self) -> None:
        payload = json.dumps({"Path": render_lab_build.BRIDGE_MODULE, "Replace": {"Path": "/x"}})
        with mock.patch.object(render_lab_build, "run", return_value=payload):
            with self.assertRaises(render_lab_build.BuildError):
                render_lab_build.resolve_published_version(Path("/tmp"), {}, "a" * 40)

    def test_pseudo_version_matching_sha_is_accepted(self) -> None:
        sha = "2012f6addc4eed465e5e6deff4170177747bdb4b"
        payload = json.dumps(
            {
                "Path": render_lab_build.BRIDGE_MODULE,
                "Version": "v0.10.2-0.20260911042437-2012f6addc4e",
            }
        )
        with mock.patch.object(render_lab_build, "run", return_value=payload):
            resolved = render_lab_build.resolve_published_version(Path("/tmp"), {}, sha)
        self.assertEqual(resolved, "v0.10.2-0.20260911042437-2012f6addc4e")

    def test_origin_hash_mismatch_is_rejected(self) -> None:
        sha = "a" * 40
        version = "v0.10.2-0.20260911042437-bbbbbbbbbbbb"
        list_payload = json.dumps({"Path": render_lab_build.BRIDGE_MODULE, "Version": version})
        download_payload = json.dumps(
            {"Path": render_lab_build.BRIDGE_MODULE, "Origin": {"Hash": "b" * 40}}
        )

        def fake_run(args, **_kwargs):
            return download_payload if args[:2] == ["go", "mod"] else list_payload

        with mock.patch.object(render_lab_build, "run", side_effect=fake_run):
            with self.assertRaises(render_lab_build.BuildError):
                render_lab_build.resolve_published_version(Path("/tmp"), {}, sha)


class CliTest(TimeoutTestCase):
    def test_missing_repository_is_a_prerequisite_error(self) -> None:
        missing = Path(tempfile.mkdtemp(prefix="lab-cli-")) / "nope"
        code = render_lab_build.main(
            [
                "--baseline-repo",
                str(missing),
                "--candidate-repo",
                str(missing),
                "--bridge-repo",
                str(missing),
            ]
        )
        self.assertEqual(code, 2)

    def test_pinned_and_iteration_modes_are_mutually_explicit(self) -> None:
        args = render_lab_build.parse_args(
            [
                "--baseline-repo",
                "a",
                "--candidate-repo",
                "b",
                "--bridge-repo",
                "c",
                "--published-bridge-sha",
                "d" * 40,
            ]
        )
        self.assertEqual(args.published_bridge_sha, "d" * 40)
        self.assertEqual(args.candidate_status, "instrumentation-only")


if __name__ == "__main__":
    unittest.main()
