#!/usr/bin/env python3
"""Unit tests for scripts/render_lab.py.

The tests use fake browser processes, temporary fake repositories, and sentinel
paths. They never launch a real Dumber build and never touch the developer's
home, XDG directories, or running browser sessions.
"""

from __future__ import annotations

import contextlib
import hashlib
import io
import json
import os
import signal
import stat
import subprocess
import sys
import tempfile
import time
import unittest
import urllib.error
import urllib.request
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))

import render_lab  # noqa: E402

# Captured before the test base class patches it, so report formatting stays
# directly testable.
REAL_PRINT_RUN_REPORT = render_lab.print_run_report

# Hard ceiling per test: a wedged child or socket must fail fast, never hang CI.
TEST_TIMEOUT_SECONDS = 25


class TimeoutTestCase(unittest.TestCase):
    """Fail instead of hanging when a child process or socket wedges."""

    timeout_seconds = TEST_TIMEOUT_SECONDS

    def setUp(self) -> None:
        super().setUp()
        self._previous_handler = signal.signal(signal.SIGALRM, self._on_timeout)
        signal.setitimer(signal.ITIMER_REAL, self.timeout_seconds)
        self._report_patch = mock.patch.object(render_lab, "print_run_report")
        self._report_patch.start()

    def tearDown(self) -> None:
        self._report_patch.stop()
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, self._previous_handler)
        super().tearDown()

    def _on_timeout(self, _signum, _frame):
        raise AssertionError(f"test exceeded {self.timeout_seconds}s hard timeout")


FAKE_BINARY_TEMPLATE = """#!/usr/bin/env python3
import os, signal, subprocess, sys, time

if sys.argv[1:3] == ["config", "status"]:
    # Emulate the real CLI: a fast, non-GUI config read with no side effects.
    print("Configuration is up to date")
    sys.exit(0)

here = os.getcwd()
with open(os.path.join(here, "child-env.json"), "w", encoding="utf-8") as handle:
    import json
    json.dump({
        "argv": sys.argv[1:],
        "cwd": here,
        "ENV": os.environ.get("ENV"),
        "HOME": os.environ.get("HOME"),
        "XDG_CONFIG_HOME": os.environ.get("XDG_CONFIG_HOME"),
        "XDG_RUNTIME_DIR": os.environ.get("XDG_RUNTIME_DIR"),
        "DUMBER_CEF_ROOT_CACHE_PATH": os.environ.get("DUMBER_CEF_ROOT_CACHE_PATH"),
        "DUMBER_CEF_EXTERNAL_BEGIN_FRAME": os.environ.get("DUMBER_CEF_EXTERNAL_BEGIN_FRAME"),
        "DUMBER_CEF2GTK_PROFILE": os.environ.get("DUMBER_CEF2GTK_PROFILE"),
        "GSK_RENDERER": os.environ.get("GSK_RENDERER"),
        "GTK_DEBUG": os.environ.get("GTK_DEBUG"),
        "http_proxy": os.environ.get("http_proxy"),
        "no_proxy": os.environ.get("no_proxy"),
        "LEAKED_DUMBER": os.environ.get("DUMBER_LEAKED_TEST_VALUE"),
        "CEF_DIR": os.environ.get("CEF_DIR"),
    }, handle)
with open(os.path.join(here, "child-pgid.txt"), "w", encoding="utf-8") as handle:
    handle.write(str(os.getpgid(0)))
if os.environ.get("FAKE_SPAWN_GRANDCHILD") == "1":
    subprocess.Popen([sys.executable, "-c",
                      "import time; time.sleep(120)"])
EXIT_CODE = int(os.environ.get("FAKE_EXIT_CODE", "0"))
SLEEP_SECONDS = float(os.environ.get("FAKE_SLEEP", "0"))
if os.environ.get("FAKE_IGNORE_SIGTERM") == "1":
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
if SLEEP_SECONDS > 0:
    time.sleep(SLEEP_SECONDS)
sys.exit(EXIT_CODE)
"""


def write_fake_binary(path: Path) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(FAKE_BINARY_TEMPLATE, encoding="utf-8")
    path.chmod(0o755)
    return path


def build_fake_repo(root: Path, variant: str = "candidate") -> Path:
    """Create a minimal repo layout with a fake binary and a valid manifest."""
    binary_relpath = Path("dist/render-lab") / variant / "dumber"
    binary = write_fake_binary(root / binary_relpath)
    fixture_dir = root / "scripts" / "render_lab"
    fixture_dir.mkdir(parents=True, exist_ok=True)
    fixture = fixture_dir / "scroll.html"
    fixture.write_text("<html><body>fixture</body></html>", encoding="utf-8")
    manifest = {
        "schema": render_lab.SCHEMA,
        "variant": variant,
        "status": "instrumentation-only" if variant == "candidate" else "baseline",
        "binary_relpath": str(binary_relpath),
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "repo_head": "0" * 40,
        "bridge_head": "1" * 40,
        "fixture_sha256": hashlib.sha256(fixture.read_bytes()).hexdigest(),
    }
    manifest_path = root / "dist" / "render-lab" / variant / "manifest.json"
    manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
    return root


class ConfigGenerationTest(TimeoutTestCase):
    def test_monitor_mode_adapts_with_fallback(self) -> None:
        parsed = render_lab.validate_config_toml(
            render_lab.generated_config_toml("monitor", "vulkan")
        )
        self.assertIs(parsed["engine"]["cef"]["adaptive_windowless_frame_rate"], True)
        self.assertEqual(parsed["engine"]["cef"]["windowless_frame_rate"], 60)
        self.assertEqual(parsed["engine"]["cef"]["windowless_frame_rate_max"], 240)
        self.assertEqual(parsed["engine"]["type"], "cef")

    def test_fixed_fps_disables_adaptation(self) -> None:
        for rate in ("60", "120", "165"):
            with self.subTest(rate=rate):
                parsed = render_lab.validate_config_toml(
                    render_lab.generated_config_toml(rate, "egl")
                )
                self.assertIs(parsed["engine"]["cef"]["adaptive_windowless_frame_rate"], False)
                self.assertEqual(parsed["engine"]["cef"]["windowless_frame_rate"], int(rate))
                self.assertEqual(parsed["engine"]["cef"]["render_stack"], "egl")

    def test_appearance_is_fixed_and_theme_free(self) -> None:
        parsed = render_lab.validate_config_toml(
            render_lab.generated_config_toml("monitor", "vulkan")
        )
        self.assertEqual(parsed["appearance"]["color_scheme"], "prefer-dark")
        self.assertIs(parsed["appearance"]["external_theme"]["enabled"], False)

    def test_validation_rejects_unknown_or_invalid_keys(self) -> None:
        with self.assertRaises(render_lab.LabError):
            render_lab.validate_config_toml("[engine]\ntype = \"cef\"\nunknown = 1\n")
        with self.assertRaises(render_lab.LabError):
            render_lab.validate_config_toml(
                render_lab.generated_config_toml("monitor", "vulkan").replace(
                    'render_stack = "vulkan"', 'render_stack = "auto"'
                )
            )
        with self.assertRaises(render_lab.LabError):
            render_lab.validate_config_toml("this is not toml")


class EnvironmentTest(TimeoutTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.run_a = Path(tempfile.mkdtemp(prefix="lab-env-a-"))
        self.run_b = Path(tempfile.mkdtemp(prefix="lab-env-b-"))

    def _base_env(self) -> dict:
        return {
            "PATH": "/usr/bin",
            "WAYLAND_DISPLAY": "wayland-1",
            "XDG_RUNTIME_DIR": "/run/user/4242",
            "DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/4242/bus",
            "PULSE_SERVER": "unix:/run/user/4242/pulse/native",
            "DUMBER_LEAKED_TEST_VALUE": "should-not-survive",
            "DUMBER_CEF_ROOT_CACHE_PATH": "/tmp/malicious-cef-cache",
            "PUREGO_CEF2GTK_BACKEND": "legacy",
            "GSK_RENDERER": "ngl",
            "GTK_DEBUG": "all",
            "GDK_DEBUG": "gl",
            "http_proxy": "http://proxy.invalid:3128",
            "ALL_PROXY": "socks5://proxy.invalid:1080",
            "CEF_DIR": "/opt/cef-test-runtime",
        }

    def test_run_roots_are_unique_and_private(self) -> None:
        env_a = render_lab.dev_environment(self.run_a, self._base_env())
        env_b = render_lab.dev_environment(self.run_b, self._base_env())
        self.assertNotEqual(env_a["HOME"], env_b["HOME"])
        self.assertNotEqual(env_a["DUMBER_CEF_ROOT_CACHE_PATH"], env_b["DUMBER_CEF_ROOT_CACHE_PATH"])
        for env, run_root in ((env_a, self.run_a), (env_b, self.run_b)):
            self.assertEqual(env["ENV"], "dev")
            root = render_lab.dev_root(run_root)
            self.assertEqual(Path(env["HOME"]), root / "home")
            self.assertEqual(Path(env["XDG_CONFIG_HOME"]), root / "config")
            self.assertEqual(Path(env["XDG_DATA_HOME"]), root / "data")
            self.assertEqual(Path(env["XDG_STATE_HOME"]), root / "state")
            self.assertEqual(Path(env["XDG_CACHE_HOME"]), root / "cache")
            self.assertTrue(Path(env["DUMBER_CEF_ROOT_CACHE_PATH"]).is_relative_to(run_root))

    def test_host_connectivity_is_preserved(self) -> None:
        env = render_lab.dev_environment(self.run_a, self._base_env())
        self.assertEqual(env["XDG_RUNTIME_DIR"], "/run/user/4242")
        self.assertEqual(env["WAYLAND_DISPLAY"], "wayland-1")
        self.assertEqual(env["DBUS_SESSION_BUS_ADDRESS"], "unix:path=/run/user/4242/bus")
        self.assertEqual(env["PULSE_SERVER"], "unix:/run/user/4242/pulse/native")

    def test_inherited_overrides_are_sanitized(self) -> None:
        env = render_lab.dev_environment(self.run_a, self._base_env())
        self.assertNotIn("DUMBER_LEAKED_TEST_VALUE", env)
        self.assertNotIn("PUREGO_CEF2GTK_BACKEND", env)
        self.assertNotIn("GSK_RENDERER", env)
        self.assertNotIn("GTK_DEBUG", env)
        self.assertNotIn("GDK_DEBUG", env)
        self.assertNotIn("http_proxy", env)
        self.assertNotIn("ALL_PROXY", env)
        self.assertNotEqual(env["DUMBER_CEF_ROOT_CACHE_PATH"], "/tmp/malicious-cef-cache")
        self.assertEqual(env["no_proxy"], "127.0.0.1,localhost")

    def test_cef_runtime_selection_is_preserved_and_hashed(self) -> None:
        env = render_lab.dev_environment(self.run_a, self._base_env())
        self.assertEqual(env["CEF_DIR"], "/opt/cef-test-runtime")
        record = render_lab.cef_runtime_record(self._base_env())
        self.assertEqual(record["env_var"], "CEF_DIR")
        self.assertNotIn("/opt/cef-test-runtime", json.dumps(record))
        self.assertEqual(record["path_sha256"], render_lab.sha256_text("/opt/cef-test-runtime")[:16])

    def test_profiling_and_external_begin_frame_are_opt_in(self) -> None:
        repo = build_fake_repo(Path(tempfile.mkdtemp(prefix="lab-repo-")))
        plan = render_lab.build_launch_plan(
            repo_root=repo,
            output_root=repo / "out",
            variant="candidate",
            fps_mode="monitor",
            render_path="current",
            stack=None,
            external_begin_frame=False,
            profile_enabled=False,
            trace_geometry=False,
            base_env=self._base_env(),
            dry_run=True,
        )
        self.assertNotIn("DUMBER_CEF_EXTERNAL_BEGIN_FRAME", plan.run_env)
        self.assertNotIn("DUMBER_CEF2GTK_PROFILE", plan.run_env)
        self.assertIsNone(plan.profile_output)

        plan = render_lab.build_launch_plan(
            repo_root=repo,
            output_root=repo / "out",
            variant="candidate",
            fps_mode="monitor",
            render_path="current",
            stack=None,
            external_begin_frame=True,
            profile_enabled=True,
            trace_geometry=False,
            base_env=self._base_env(),
            dry_run=True,
        )
        self.assertEqual(plan.run_env["DUMBER_CEF_EXTERNAL_BEGIN_FRAME"], "1")
        self.assertEqual(plan.run_env["DUMBER_CEF2GTK_PROFILE"], "1")
        self.assertTrue(Path(plan.run_env["DUMBER_CEF2GTK_PROFILE_OUTPUT"]).is_relative_to(plan.run_root))


class ManifestTest(TimeoutTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.repo = build_fake_repo(Path(tempfile.mkdtemp(prefix="lab-manifest-")))

    def _plan(self, **overrides):
        kwargs = dict(
            repo_root=self.repo,
            output_root=self.repo / "out",
            variant="candidate",
            fps_mode="monitor",
            render_path="current",
            stack=None,
            external_begin_frame=False,
            profile_enabled=False,
            trace_geometry=False,
            base_env={"PATH": "/usr/bin"},
            dry_run=True,
        )
        kwargs.update(overrides)
        return render_lab.build_launch_plan(**kwargs)

    def test_tampered_binary_is_rejected(self) -> None:
        binary = self.repo / "dist" / "render-lab" / "candidate" / "dumber"
        binary.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        with self.assertRaises(render_lab.EnvironmentBlocker):
            self._plan()

    def test_missing_manifest_is_rejected(self) -> None:
        (self.repo / "dist" / "render-lab" / "candidate" / "manifest.json").unlink()
        with self.assertRaises(render_lab.EnvironmentBlocker):
            self._plan()

    def test_wrong_variant_manifest_is_rejected(self) -> None:
        manifest_path = self.repo / "dist" / "render-lab" / "candidate" / "manifest.json"
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        manifest["variant"] = "baseline"
        manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
        with self.assertRaises(render_lab.EnvironmentBlocker):
            self._plan()

    def test_absolute_binary_path_is_rejected(self) -> None:
        manifest_path = self.repo / "dist" / "render-lab" / "candidate" / "manifest.json"
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        manifest["binary_relpath"] = "/usr/bin/true"
        manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
        with self.assertRaises(render_lab.EnvironmentBlocker):
            self._plan()

    def test_symlinked_output_root_is_rejected(self) -> None:
        real = self.repo / "real-output"
        real.mkdir()
        link = self.repo / "linked-output"
        link.symlink_to(real)
        with self.assertRaises(render_lab.EnvironmentBlocker):
            self._plan(output_root=link)

    def test_metadata_is_sanitized(self) -> None:
        plan = self._plan()
        metadata = json.dumps(plan.metadata())
        self.assertNotIn(str(Path.home()), metadata)
        self.assertIsNone(json.loads(metadata)["cef_runtime"]["version"])
        self.assertIn(plan.run_root.name, metadata)


class FixtureServerTest(TimeoutTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.fixture = Path(tempfile.mkdtemp(prefix="lab-fixture-")) / "scroll.html"
        self.fixture.write_text("<html><body>ok</body></html>", encoding="utf-8")
        self.server, self.url, self.digest = render_lab.start_fixture_server(self.fixture)
        self.addCleanup(render_lab.stop_fixture_server, self.server)

    def _get(self, path: str):
        return urllib.request.urlopen(f"http://127.0.0.1:{self.server.server_address[1]}{path}")

    def test_serves_only_the_allowlisted_path(self) -> None:
        with self._get("/scroll.html") as response:
            self.assertEqual(response.status, 200)
            self.assertEqual(response.headers["Content-Type"], "text/html; charset=utf-8")
            self.assertEqual(response.read(), self.fixture.read_bytes())

    def test_rejects_traversal_and_listings(self) -> None:
        for path in ("/", "/scroll.html/", "/../etc/passwd", "/etc/passwd", "/index.html"):
            with self.subTest(path=path):
                with self.assertRaises(urllib.error.HTTPError) as caught:
                    self._get(path)
                self.assertEqual(caught.exception.code, 404)
                self.assertNotIn("scroll", caught.exception.read().decode("utf-8", "replace"))
                caught.exception.close()

    def test_digest_matches_fixture_bytes(self) -> None:
        self.assertEqual(self.digest, hashlib.sha256(self.fixture.read_bytes()).hexdigest())

    def test_fixture_is_self_contained(self) -> None:
        html = (Path(__file__).resolve().parent / "render_lab" / "scroll.html").read_text(
            encoding="utf-8"
        )
        self.assertIn("scrollTo", html)
        self.assertIn("prefers-reduced-motion", html)
        self.assertIn("not GTK presentation", html)
        self.assertNotIn("http://", html)
        self.assertNotIn("https://", html)
        self.assertIn("scroll-behavior: auto", html)
        self.assertIn("aria", html.lower())
        self.assertIn("<button", html)


class ExecutionTest(TimeoutTestCase):
    def setUp(self) -> None:
        super().setUp()
        self.repo = build_fake_repo(Path(tempfile.mkdtemp(prefix="lab-exec-")))
        self.base_env = dict(os.environ)
        self.base_env.pop("DUMBER_CEF2GTK_PROFILE", None)

    def _plan(self, **overrides):
        kwargs = dict(
            repo_root=self.repo,
            output_root=self.repo / "out",
            variant="candidate",
            fps_mode="monitor",
            render_path="current",
            stack=None,
            external_begin_frame=False,
            profile_enabled=False,
            trace_geometry=False,
            base_env=self.base_env,
            dry_run=False,
        )
        kwargs.update(overrides)
        return render_lab.build_launch_plan(**kwargs)

    def test_dry_run_does_not_spawn_a_browser(self) -> None:
        plan = self._plan(dry_run=True)
        exit_code = render_lab.execute_plan(plan, None)
        self.assertEqual(exit_code, 0)
        self.assertFalse((plan.run_root / "dumber.log").exists())
        metadata = json.loads((plan.run_root / "run.json").read_text(encoding="utf-8"))
        self.assertEqual(metadata["termination"], "dry-run")

    def test_child_runs_isolated_with_lab_config(self) -> None:
        env = dict(self.base_env)
        env["FAKE_EXIT_CODE"] = "0"
        plan = self._plan(base_env=env)
        exit_code = render_lab.execute_plan(plan, None)
        self.assertEqual(exit_code, 0)
        child_env = json.loads((plan.run_root / "child-env.json").read_text(encoding="utf-8"))
        self.assertEqual(child_env["ENV"], "dev")
        self.assertEqual(child_env["argv"][0], "browse")
        self.assertTrue(child_env["argv"][1].startswith("http://127.0.0.1:"))
        self.assertTrue(child_env["argv"][1].endswith("/scroll.html"))
        self.assertEqual(child_env["cwd"], str(plan.run_root))
        self.assertTrue(Path(child_env["HOME"]).is_relative_to(plan.run_root))
        self.assertTrue(Path(child_env["DUMBER_CEF_ROOT_CACHE_PATH"]).is_relative_to(plan.run_root))
        self.assertEqual(child_env["no_proxy"], "127.0.0.1,localhost")
        self.assertIsNone(child_env["LEAKED_DUMBER"])

    def test_early_exit_is_recorded_not_hidden(self) -> None:
        env = dict(self.base_env)
        env["FAKE_EXIT_CODE"] = "3"
        plan = self._plan(base_env=env)
        exit_code = render_lab.execute_plan(plan, None)
        metadata = json.loads((plan.run_root / "run.json").read_text(encoding="utf-8"))
        self.assertEqual(metadata["exit_code"], 3)
        self.assertEqual(metadata["termination"], "natural")
        self.assertEqual(exit_code, 1)

    def test_duration_terminates_only_its_own_process_group(self) -> None:
        env = dict(self.base_env, FAKE_SLEEP="60", FAKE_SPAWN_GRANDCHILD="1")
        plan = self._plan(base_env=env)
        sentinel = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"])
        self.addCleanup(lambda: (sentinel.kill(), sentinel.wait()))
        started = time.monotonic()
        exit_code = render_lab.execute_plan(plan, 0.6)
        elapsed = time.monotonic() - started
        self.assertLess(elapsed, render_lab.GRACEFUL_EXIT_SECONDS + 5)
        self.assertEqual(exit_code, 0)
        metadata = json.loads((plan.run_root / "run.json").read_text(encoding="utf-8"))
        self.assertTrue(metadata["bounded_by_duration"])
        self.assertIn(metadata["termination"], {"graceful", "forced"})
        self.assertIsNone(sentinel.poll(), "unrelated process must survive lab cleanup")
        pgid = int((plan.run_root / "child-pgid.txt").read_text(encoding="utf-8"))
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            try:
                os.killpg(pgid, 0)
            except ProcessLookupError:
                break
            time.sleep(0.05)
        else:
            self.fail("lab-owned process group survived termination")

    def test_stubborn_child_is_forced_after_graceful_timeout(self) -> None:
        env = dict(self.base_env, FAKE_SLEEP="60", FAKE_IGNORE_SIGTERM="1")
        with mock.patch.object(render_lab, "GRACEFUL_EXIT_SECONDS", 0.5):
            plan = self._plan(base_env=env)
            exit_code = render_lab.execute_plan(plan, 0.4)
        self.assertEqual(exit_code, 0)
        metadata = json.loads((plan.run_root / "run.json").read_text(encoding="utf-8"))
        self.assertEqual(metadata["termination"], "forced")

    def test_run_artifacts_are_private_and_sanitized(self) -> None:
        plan = self._plan(base_env=dict(self.base_env, FAKE_EXIT_CODE="0"))
        render_lab.execute_plan(plan, None)
        run_json = plan.run_root / "run.json"
        mode = stat.S_IMODE(run_json.stat().st_mode)
        self.assertEqual(mode, 0o600)
        self.assertEqual(stat.S_IMODE(plan.run_root.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE(plan.config_path.stat().st_mode), 0o600)
        metadata = json.loads(run_json.read_text(encoding="utf-8"))
        self.assertNotIn(str(Path.home()), json.dumps(metadata))

    def test_user_profile_sentinels_are_untouched(self) -> None:
        sentinel = Path(tempfile.mkdtemp(prefix="lab-user-"))
        (sentinel / "config.toml").write_text("sentinel", encoding="utf-8")
        env = dict(self.base_env)
        env["HOME"] = str(sentinel)
        env["XDG_CONFIG_HOME"] = str(sentinel)
        plan = self._plan(base_env=env)
        render_lab.execute_plan(plan, None)
        self.assertEqual((sentinel / "config.toml").read_text(encoding="utf-8"), "sentinel")
        self.assertEqual(sorted(p.name for p in sentinel.iterdir()), ["config.toml"])

    def test_report_prints_pipeline_counters_and_quantiles(self) -> None:
        repo = build_fake_repo(Path(tempfile.mkdtemp(prefix="lab-report-pipe-")))
        plan = render_lab.build_launch_plan(
            repo_root=repo,
            output_root=repo / "out",
            variant="candidate",
            fps_mode="monitor",
            render_path="current",
            stack=None,
            external_begin_frame=False,
            profile_enabled=True,
            trace_geometry=False,
            base_env=self.base_env,
            dry_run=False,
        )
        summary = render_lab.summarize_profile(
            self._pipeline_fixture(repo, path_hint="report")
        )
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            REAL_PRINT_RUN_REPORT(
                plan,
                dict(plan.metadata(), termination="natural", exit_code=0),
                summary,
            )
        report = buffer.getvalue()
        for expected in (
            "variant:        candidate",
            "what changed:   measurement only",
            "received_to_import_ms: samples=6",
            "p50=1.5000ms",
            "counters: received=6 replaced=2",
            "feedback:  available=4 unavailable=2",
            "present:  frames_received=80 frames_queued=80",
            "input:    scroll_events=24",
            "per window: 40.0 frames, 12.0 GTK scroll events",
            "observer:  import_copy samples=80 gtk_wait samples=6 goroutines_max=41",
        ):
            self.assertIn(expected, report)
        self.assertNotIn("None", report)
        self.assertNotIn("KeyError", report)

    def _pipeline_fixture(self, repo: Path, *, path_hint: str) -> Path:
        path = repo / f"{path_hint}.jsonl"
        records = []
        for p50 in (1.0, 2.0):
            records.append(
                {
                    "webview_id": 1,
                    "snapshot": {
                        "frames_received": 40,
                        "frames_queued": 40,
                        "frames_rendered": 0,
                        "import_failures": 0,
                        "scroll_events": 12,
                        "scroll_abs_dy_sum": 640.0,
                        "external_begin_frames_sent": 0,
                        "import_copy_cpu": {"count": 40},
                        "gtk_wait_cpu": {"count": 3},
                        "gc": {"num_goroutine": 41},
                        "gdk_pipeline": {
                            "schema_version": 1,
                            "frames_received": 3,
                            "pending_replaced": 1,
                            "frames_imported": 2,
                            "frames_swapped": 2,
                            "gtk_paint_cycles": 2,
                            "swaps_overwritten_before_paint": 0,
                            "feedback_available": 2,
                            "feedback_unavailable": 1,
                            "received_to_import_ms": {"samples": 3, "p50": p50, "p95": p50 * 2, "p99": p50 * 3},
                            "queue_wait_ms": {"samples": 3, "p50": p50, "p95": p50, "p99": p50},
                            "import_elapsed_ms": {"samples": 3, "p50": p50, "p95": p50, "p99": p50},
                            "received_to_swap_ms": {"samples": 3, "p50": p50, "p95": p50, "p99": p50},
                        }
                    },
                }
            )
        path.write_text("\n".join(json.dumps(record) for record in records) + "\n", encoding="utf-8")
        return path

    def test_report_marks_missing_metrics_unavailable(self) -> None:
        repo = build_fake_repo(Path(tempfile.mkdtemp(prefix="lab-report-")), variant="baseline")
        plan = render_lab.build_launch_plan(
            repo_root=repo,
            output_root=repo / "out",
            variant="baseline",
            fps_mode="monitor",
            render_path="current",
            stack=None,
            external_begin_frame=False,
            profile_enabled=True,
            trace_geometry=False,
            base_env=self.base_env,
            dry_run=False,
        )
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            REAL_PRINT_RUN_REPORT(
                plan,
                dict(plan.metadata(), termination="natural", exit_code=0),
                {"gdk_pipeline": None, "note": "profile output predates instrumentation"},
            )
        report = buffer.getvalue()
        self.assertIn("unavailable", report)
        self.assertNotIn("p50=0", report)
        self.assertNotIn("p50=None", report)


class ProfileSummaryTest(TimeoutTestCase):
    def _write(self, records: list[dict]) -> Path:
        path = Path(tempfile.mkdtemp(prefix="lab-profile-")) / "profile.jsonl"
        path.write_text("\n".join(json.dumps(r) for r in records) + "\n", encoding="utf-8")
        return path

    def test_missing_output_is_unavailable_not_zero(self) -> None:
        summary = render_lab.summarize_profile(Path("/nonexistent/profile.jsonl"))
        self.assertIsNone(summary["gdk_pipeline"])

    def test_baseline_snapshot_without_pipeline_is_unavailable(self) -> None:
        path = self._write([{"webview_id": 1, "snapshot": {"frames_received": 10}}])
        summary = render_lab.summarize_profile(path)
        self.assertIsNone(summary["gdk_pipeline"])
        self.assertIn("predate", summary["note"])

    def test_empty_profile_explains_frame_driven_emission(self) -> None:
        path = Path(tempfile.mkdtemp(prefix="lab-profile-")) / "profile.jsonl"
        path.write_text("", encoding="utf-8")
        summary = render_lab.summarize_profile(path)
        self.assertIsNone(summary["gdk_pipeline"])
        self.assertIn("accelerated frames", summary["note"])
        self.assertEqual(summary["windows"], 0)

    def test_candidate_snapshots_are_aggregated(self) -> None:
        def pipeline(samples: int, p50: float) -> dict:
            return {
                "gdk_pipeline": {
                    "schema_version": 1,
                    "frames_received": 10,
                    "pending_replaced": 2,
                    "frames_imported": 8,
                    "frames_swapped": 8,
                    "gtk_paint_cycles": 5,
                    "swaps_overwritten_before_paint": 1,
                    "feedback_available": 4,
                    "feedback_unavailable": 1,
                    "received_to_import_ms": {"samples": samples, "p50": p50, "p95": p50 * 2, "p99": p50 * 3},
                    "queue_wait_ms": {"samples": samples, "p50": p50 / 2, "p95": p50, "p99": p50},
                    "import_elapsed_ms": {"samples": samples, "p50": p50 / 10, "p95": p50 / 5, "p99": p50 / 5},
                    "received_to_swap_ms": {"samples": samples, "p50": p50, "p95": p50 * 2, "p99": p50 * 4},
                }
            }

        path = self._write(
            [
                {"webview_id": 1, "snapshot": pipeline(10, 4.0)},
                {"webview_id": 1, "snapshot": pipeline(6, 8.0)},
            ]
        )
        summary = render_lab.summarize_profile(path)
        data = summary["gdk_pipeline"]
        self.assertEqual(data["counters"]["frames_received"], 20)
        self.assertEqual(data["counters"]["pending_replaced"], 4)
        self.assertEqual(data["counters"]["gtk_paint_cycles"], 10)
        series = data["received_to_import_ms"]
        self.assertEqual(series["samples_total"], 16)
        self.assertEqual(series["windows_with_samples"], 2)
        self.assertEqual(series["p50"], 6.0)
        self.assertEqual(series["p95"], 12.0)
        self.assertEqual(series["p99"], 18.0)
        self.assertIn("median of per-window", series["quantile_note"])

    def test_present_and_input_counters_are_summed(self) -> None:
        records = []
        for index in range(3):
            records.append(
                {
                    "webview_id": 1,
                    "snapshot": {
                        "frames_received": 40,
                        "frames_queued": 40,
                        "frames_rendered": 0,
                        "import_failures": 1,
                        "scroll_events": 7,
                        "scroll_abs_dy_sum": 100.5,
                        "external_begin_frames_sent": 2,
                        "import_copy_cpu": {"count": 40},
                        "gtk_wait_cpu": {"count": 4},
                        "gc": {"num_goroutine": 30 + index},
                    },
                }
            )
        summary = render_lab.summarize_profile(self._write(records))
        self.assertEqual(summary["present"]["frames_received"], 120)
        self.assertEqual(summary["present"]["import_failures"], 3)
        self.assertEqual(summary["input"]["scroll_events"], 21)
        self.assertEqual(summary["input"]["external_begin_frames_sent"], 6)
        self.assertEqual(summary["import_copy_samples"], 120)
        self.assertEqual(summary["gtk_wait_samples"], 12)
        self.assertEqual(summary["goroutines_max"], 32)
        self.assertAlmostEqual(summary["scroll_abs_dy_sum"], 301.5)
        self.assertEqual(summary["windows"], 3)

    def test_trace_geometry_is_a_named_diagnostic(self) -> None:
        repo = build_fake_repo(Path(tempfile.mkdtemp(prefix="lab-trace-")))
        base_env = {"PATH": "/usr/bin", "PUREGO_CEF2GTK_TRACE_OSR": "inherited"}
        plan = render_lab.build_launch_plan(
            repo_root=repo,
            output_root=repo / "out",
            variant="candidate",
            fps_mode="monitor",
            render_path="current",
            stack=None,
            external_begin_frame=False,
            profile_enabled=False,
            trace_geometry=True,
            base_env=base_env,
            dry_run=True,
        )
        self.assertEqual(plan.run_env["PUREGO_CEF2GTK_TRACE_OSR"], "1")
        self.assertEqual(plan.run_env["PUREGO_CEF2GTK_TRACE_SCALE"], "1")
        self.assertTrue(plan.metadata()["trace_geometry"])

    def test_render_path_selects_the_stack_and_rejects_conflicts(self) -> None:
        self.assertEqual(render_lab.resolve_stack("current", None), "vulkan")
        self.assertEqual(render_lab.resolve_stack("dmabuf-copy", None), "egl")
        # An explicit stack that agrees is accepted.
        self.assertEqual(render_lab.resolve_stack("dmabuf-copy", "egl"), "egl")
        with self.assertRaises(render_lab.LabError):
            render_lab.resolve_stack("current", "egl")

    def test_render_path_reaches_the_generated_config_and_metadata(self) -> None:
        repo = build_fake_repo(Path(tempfile.mkdtemp(prefix="lab-path-")))
        plan = render_lab.build_launch_plan(
            repo_root=repo,
            output_root=repo / "out",
            variant="candidate",
            fps_mode="monitor",
            render_path="dmabuf-copy",
            stack=None,
            external_begin_frame=False,
            profile_enabled=False,
            trace_geometry=False,
            base_env={"PATH": "/usr/bin"},
            dry_run=True,
        )
        self.assertEqual(plan.stack, "egl")
        self.assertEqual(plan.metadata()["render_path"], "dmabuf-copy")
        config = plan.config_path.read_text(encoding="utf-8")
        self.assertIn('render_stack = "egl"', config)

    def test_series_without_quantiles_report_unavailable(self) -> None:
        path = self._write(
            [
                {
                    "webview_id": 1,
                    "snapshot": {
                        "gdk_pipeline": {
                            "schema_version": 1,
                            "frames_received": 1,
                            "received_to_import_ms": {"samples": 0},
                        }
                    },
                }
            ]
        )
        summary = render_lab.summarize_profile(path)
        series = summary["gdk_pipeline"]["received_to_import_ms"]
        self.assertEqual(series["samples_total"], 0)
        self.assertIsNone(series["p50"])
        self.assertIsNone(series["p95"])

    def test_render_change_note_is_explicit(self) -> None:
        self.assertIn("no rendering change", render_lab.render_change_note("baseline"))
        self.assertIn(
            "no rendering, ownership or pacing",
            render_lab.render_change_note("instrumentation-only"),
        )
        self.assertIn(
            "verified ownership change",
            render_lab.render_change_note("ownership-verified-candidate"),
        )
        self.assertNotIn(
            "no rendering",
            render_lab.render_change_note("ownership-verified-candidate"),
        )

    def test_malformed_lines_are_skipped(self) -> None:
        path = Path(tempfile.mkdtemp(prefix="lab-profile-")) / "profile.jsonl"
        path.write_text('not json\n{"webview_id":1,"snapshot":{}}\n', encoding="utf-8")
        summary = render_lab.summarize_profile(path)
        self.assertIsNone(summary["gdk_pipeline"])

    def test_keyboard_interrupt_maps_to_termination(self) -> None:
        repo = build_fake_repo(Path(tempfile.mkdtemp(prefix="lab-sigint-")))
        plan = render_lab.build_launch_plan(
            repo_root=repo,
            output_root=repo / "out",
            variant="candidate",
            fps_mode="monitor",
            render_path="current",
            stack=None,
            external_begin_frame=False,
            profile_enabled=False,
            trace_geometry=False,
            base_env=dict(os.environ, FAKE_SLEEP="60"),
            dry_run=False,
        )
        original_sleep = time.sleep
        interruptions = {"remaining": 1}

        def interrupting_sleep(seconds: float) -> None:
            original_sleep(min(seconds, 0.2))
            if interruptions["remaining"] > 0:
                interruptions["remaining"] -= 1
                raise KeyboardInterrupt

        with mock.patch.object(render_lab.time, "sleep", interrupting_sleep):
            render_lab.execute_plan(plan, None)
        metadata = json.loads((plan.run_root / "run.json").read_text(encoding="utf-8"))
        self.assertTrue(metadata["interrupted"])
        self.assertIn(metadata["termination"], {"graceful", "forced"})
        self.assertNotEqual(metadata["exit_code"], 0)


class CliTest(TimeoutTestCase):
    def test_invalid_duration_is_rejected(self) -> None:
        self.assertEqual(render_lab.main(["--duration", "-1"]), 1)

    def test_choices_reject_unknown_values(self) -> None:
        with self.assertRaises(SystemExit):
            render_lab.parse_args(["--fps", "144"])
        with self.assertRaises(SystemExit):
            render_lab.parse_args(["--stack", "auto"])

    def test_missing_display_is_an_environment_blocker(self) -> None:
        with (
            mock.patch.object(render_lab, "_display_available", return_value=False),
            mock.patch.object(render_lab, "execute_plan") as execute,
        ):
            code = render_lab.main(["--variant", "candidate"])
        self.assertEqual(code, 2)
        execute.assert_not_called()


if __name__ == "__main__":
    unittest.main()
