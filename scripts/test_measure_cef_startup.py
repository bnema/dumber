#!/usr/bin/env python3
"""Unit tests for measure_cef_startup.py (stdlib only).

Run headless: python3 -m unittest discover -s scripts -p 'test_measure_cef_startup.py'
Set DUMBER_MEASURE_CUTOFF_SECONDS=0.2 for fast streaming tests (default 5.0).
"""

import os
import subprocess
import sys
import tempfile
import threading
import unittest
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import measure_cef_startup as harness


class BeaconTest(unittest.TestCase):
    def setUp(self):
        self.state = harness.FixtureState()
        harness.Handler.state = self.state
        harness.Handler.fixture = "static"
        harness.Handler.run_token = "a" * 16
        harness.Handler.nav_token = "b" * 16
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), harness.Handler)
        self.port = self.server.server_address[1]
        self.thread = threading.Thread(target=self.server.serve_forever, kwargs={"poll_interval": 0.05})
        self.thread.daemon = True
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)

    def get(self, query):
        url = "http://127.0.0.1:%d/__beacon?%s" % (self.port, query)
        try:
            with urllib.request.urlopen(url, timeout=5) as response:
                return response.status
        except urllib.error.HTTPError as exc:
            exc.read()
            return exc.code

    def test_accepts_valid_beacon(self):
        status = self.get("run=%s&nav=%s&event=%s&value=12" % ("a" * 16, "b" * 16, "first-contentful-paint"))
        self.assertEqual(status, 204)
        self.assertEqual(len(self.state.beacons), 1)
        self.assertEqual(self.state.beacons[0]["value_ms"], 12)

    def test_rejects_wrong_token(self):
        status = self.get("run=%s&nav=%s&event=%s&value=12" % ("c" * 16, "d" * 16, "first-contentful-paint"))
        self.assertEqual(status, 400)
        self.assertEqual(self.state.beacons, [])

    def test_rejects_malformed_token(self):
        status = self.get("run=short&nav=%s&event=%s&value=12" % ("b" * 16, "first-contentful-paint"))
        self.assertEqual(status, 400)
        self.assertEqual(self.state.beacons, [])

    def test_rejects_non_numeric_value(self):
        status = self.get("run=%s&nav=%s&event=%s&value=abc" % ("a" * 16, "b" * 16, "first-contentful-paint"))
        self.assertEqual(status, 400)
        self.assertEqual(self.state.beacons, [])

    def test_rejects_unknown_event(self):
        status = self.get("run=%s&nav=%s&event=%s&value=1" % ("a" * 16, "b" * 16, "bogus"))
        self.assertEqual(status, 400)
        self.assertEqual(self.state.beacons, [])

    def test_milestones_still_parsed(self):
        lines = ['{"message":"startup_trace: milestone","milestone":"process_entry"}', "not json"]
        self.assertEqual(len(harness.parse_child_output(lines)), 1)

    def test_image_endpoint_serves_decodable_png(self):
        import struct
        import zlib
        url = "http://127.0.0.1:%d/pixel.png" % self.port
        with urllib.request.urlopen(url, timeout=5) as response:
            self.assertEqual(response.status, 200)
            self.assertEqual(response.headers.get_content_type(), "image/png")
            body = response.read()
        self.assertEqual(body[:8], bytes.fromhex("89504e470d0a1a0a"))
        width, height = struct.unpack(">II", body[16:24])
        self.assertEqual((width, height), (1, 1))
        idat_len = struct.unpack(">I", body[33:37])[0]
        self.assertEqual(body[37:41], b"IDAT")
        zlib.decompress(body[41:41 + idat_len])
        self.assertEqual(body[-8:-4], b"IEND")


class CliValidationTest(unittest.TestCase):
    def setUp(self):
        self.script = os.path.join(os.path.dirname(os.path.abspath(__file__)), "measure_cef_startup.py")
        self.temp = tempfile.mkdtemp()
        self.binary = os.path.join(self.temp, "fakebin")
        with open(self.binary, "w") as handle:
            handle.write("#!/bin/sh\nexit 0\n")
        os.chmod(self.binary, 0o755)
        self.cef_dir = os.path.join(self.temp, "cef")
        os.makedirs(self.cef_dir)

    def test_rejects_existing_output(self):
        existing = os.path.join(self.temp, "exists")
        os.makedirs(existing)
        result = subprocess.run(
            [sys.executable, self.script, "--binary", self.binary, "--cef-dir", self.cef_dir,
             "--scenario", "profile-fresh", "--fixture", "static",
             "--runs", "1", "--output", existing],
            capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("fresh absolute directory", result.stderr)

    def test_rejects_relative_output(self):
        result = subprocess.run(
            [sys.executable, self.script, "--binary", self.binary, "--cef-dir", self.cef_dir,
             "--scenario", "profile-fresh", "--fixture", "static",
             "--runs", "1", "--output", "relative/path"],
            capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)

    def test_rejects_missing_binary(self):
        result = subprocess.run(
            [sys.executable, self.script, "--binary", os.path.join(self.temp, "missing"),
             "--cef-dir", self.cef_dir, "--scenario", "profile-fresh", "--fixture", "static",
             "--runs", "1", "--output", os.path.join(self.temp, "out")],
            capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)

    def test_rejects_bad_runs(self):
        result = subprocess.run(
            [sys.executable, self.script, "--binary", self.binary, "--cef-dir", self.cef_dir,
             "--scenario", "profile-fresh", "--fixture", "static",
             "--runs", "0", "--output", os.path.join(self.temp, "out2")],
            capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)

    def test_rejects_bad_idle_timeout(self):
        for bad in ("-1", "300001"):
            result = subprocess.run(
                [sys.executable, self.script, "--binary", self.binary, "--cef-dir", self.cef_dir,
                 "--scenario", "profile-fresh", "--fixture", "static",
                 "--runs", "1", "--idle-timeout-ms", bad,
                 "--output", os.path.join(self.temp, "out-idle-" + bad)],
                capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("idle-timeout-ms", result.stderr)
        result = subprocess.run(
            [sys.executable, self.script, "--binary", self.binary, "--cef-dir", self.cef_dir,
             "--scenario", "profile-fresh", "--fixture", "static",
             "--runs", "0", "--output", os.path.join(self.temp, "out2")],
            capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)


class FakeBinaryLifetimeTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.mkdtemp()
        self.cef_dir = os.path.join(self.temp, "cef")
        os.makedirs(self.cef_dir)
        self.counter = 0

    def make_binary(self, body):
        self.counter += 1
        path = os.path.join(self.temp, "fake-%d" % self.counter)
        with open(path, "w") as handle:
            handle.write("#!/bin/sh\n" + body + "\n")
        os.chmod(path, 0o755)
        return path

    def make_fetch_binary(self):
        # Fetches the fixture URL passed as argv[2] (argv: browse <url>),
        # then exits: exercises document counting without a real browser.
        path = os.path.join(self.temp, "fetch-%d" % self.counter)
        self.counter += 1
        with open(path, "w") as handle:
            handle.write("#!/usr/bin/env python3\nimport sys, urllib.request\n"
                         "urllib.request.urlopen(sys.argv[2], timeout=10).read()\n")
        os.chmod(path, 0o755)
        return path

    def test_duplicate_documents_marked_incomplete(self):
        binary = self.make_binary("exit 0")
        result = harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1)
        self.assertFalse(result["complete"])
        self.assertIsNone(result["target_fcp_ms"])

    def test_absent_fcp_is_incomplete(self):
        binary = self.make_fetch_binary()
        result = harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1)
        self.assertEqual(result["document_requests"], 1)
        self.assertIsNone(result["target_fcp_ms"])
        self.assertFalse(result["complete"])
        # Same-clock spawn-to-request latency is still reported.
        self.assertIsNotNone(result["first_document_observation_ms"])
        self.assertGreaterEqual(result["first_document_observation_ms"], 0)

    def test_no_navigation_means_no_document_observation(self):
        binary = self.make_binary("exit 0")
        result = harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1)
        self.assertEqual(result["document_requests"], 0)
        self.assertIsNone(result["first_document_observation_ms"])

    def test_relay_fallback_recorded_without_owner(self):
        binary = self.make_binary("exit 0")
        result = harness.run_sample(binary, self.cef_dir, "relay-window", "static", 1)
        self.assertTrue(result["fallback_spawn"])
        self.assertFalse(result["relayed"])

    def test_second_window_uses_observation_not_process_summary(self):
        binary = self.make_binary("exit 0")
        first = harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1)
        second = harness.run_sample(binary, self.cef_dir, "relay-window", "static", 2)
        self.assertIn("observation_spawn_to_summary_ms", first)
        self.assertIn("observation_spawn_to_summary_ms", second)
        self.assertNotIn("total_ms", second)

    def test_run_dir_cleaned_up(self):
        before = set(os.listdir(tempfile.gettempdir()))
        binary = self.make_binary("exit 0")
        harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1)
        after = set(os.listdir(tempfile.gettempdir()))
        leaked = [name for name in (after - before) if name.startswith("cef-startup-run-")]
        self.assertEqual(leaked, [])

    def test_streaming_returns_without_process_exit(self):
        # A hanging GUI must not pin the harness: with a short cutoff the
        # sample decides from streamed output, then terminates the child.
        binary = self.make_binary("exec sleep 30")
        result = harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1)
        self.assertFalse(result["complete"])
        self.assertLess(result["observation_spawn_to_summary_ms"], 15000)


class ResidencyContractTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.mkdtemp()
        self.cef_dir = os.path.join(self.temp, "cef")
        os.makedirs(self.cef_dir)
        self.dirs = {
            "config": os.path.join(self.temp, "config"),
            "data": os.path.join(self.temp, "data"),
            "state": os.path.join(self.temp, "state"),
            "cache": os.path.join(self.temp, "cache"),
            "runtime": os.path.join(self.temp, "runtime"),
            "root_cache": os.path.join(self.temp, "cef-root-cache"),
        }

    def test_scenario_env_forces_json_debug_config(self):
        env = harness.scenario_env(self.cef_dir, self.dirs, idle_timeout_ms=120000)
        self.assertEqual(env["DUMBER_CEF_DIR"], self.cef_dir)
        with open(os.path.join(self.dirs["config"], "dumber", "config.toml")) as handle:
            content = handle.read()
        self.assertIn('format = "json"', content)
        self.assertIn('level = "debug"', content)
        self.assertIn(self.cef_dir, content)
        self.assertIn("idle_runtime_timeout_ms = 120000", content)

    def test_scenario_env_defaults_to_no_residency(self):
        harness.scenario_env(self.cef_dir, self.dirs)
        with open(os.path.join(self.dirs["config"], "dumber", "config.toml")) as handle:
            content = handle.read()
        self.assertIn("idle_runtime_timeout_ms = 0", content)

    def test_scenario_env_passes_display_vars_only(self):
        os.environ["DUMBER_MEASURE_TEST_MARKER"] = "must-not-propagate"
        try:
            env = harness.scenario_env(self.cef_dir, self.dirs)
        finally:
            del os.environ["DUMBER_MEASURE_TEST_MARKER"]
        self.assertNotIn("DUMBER_MEASURE_TEST_MARKER", env)
        for name in harness.DISPLAY_PASSTHROUGH_VARS:
            if os.environ.get(name):
                self.assertEqual(env[name], os.environ[name])

    def test_wayland_absolute_display_needs_nothing(self):
        previous = os.environ.get("WAYLAND_DISPLAY")
        runtime = os.environ.get("XDG_RUNTIME_DIR", "/run/user/%d" % os.getuid())
        display = previous or "wayland-1"
        absolute = display if "/" in display else os.path.join(runtime, os.path.basename(display))
        env = {"WAYLAND_DISPLAY": absolute}
        os.makedirs(self.dirs["runtime"], exist_ok=True)
        self.assertTrue(harness.ensure_wayland_socket(self.dirs["runtime"], env))
        self.assertEqual(env["WAYLAND_DISPLAY"], absolute)
        self.assertEqual(os.listdir(self.dirs["runtime"]), [])

    def test_wayland_relative_display_resolves_to_absolute(self):
        import socket as stdlib_socket
        fake_runtime = os.path.join(self.temp, "real-runtime")
        os.makedirs(fake_runtime)
        sock = stdlib_socket.socket(stdlib_socket.AF_UNIX, stdlib_socket.SOCK_STREAM)
        sock.bind(os.path.join(fake_runtime, "wayland-99"))
        previous_runtime = os.environ.get("XDG_RUNTIME_DIR")
        os.environ["XDG_RUNTIME_DIR"] = fake_runtime
        env = {"WAYLAND_DISPLAY": "wayland-99"}
        try:
            self.assertTrue(harness.ensure_wayland_socket(self.dirs["runtime"], env))
            self.assertEqual(env["WAYLAND_DISPLAY"],
                             os.path.join(fake_runtime, "wayland-99"))
            # No symlink: deep isolated paths exceed the unix-socket limit.
            self.assertNotIn("wayland-99", os.listdir(self.dirs["runtime"]))
        finally:
            if previous_runtime is None:
                os.environ.pop("XDG_RUNTIME_DIR", None)
            else:
                os.environ["XDG_RUNTIME_DIR"] = previous_runtime
            sock.close()

    def test_wayland_missing_socket_reports_false(self):
        previous_runtime = os.environ.get("XDG_RUNTIME_DIR")
        os.environ["XDG_RUNTIME_DIR"] = os.path.join(self.temp, "empty-runtime")
        env = {"WAYLAND_DISPLAY": "wayland-does-not-exist"}
        try:
            self.assertFalse(harness.ensure_wayland_socket(self.dirs["runtime"], env))
        finally:
            if previous_runtime is None:
                os.environ.pop("XDG_RUNTIME_DIR", None)
            else:
                os.environ["XDG_RUNTIME_DIR"] = previous_runtime
        self.assertEqual(env["WAYLAND_DISPLAY"], "wayland-does-not-exist")

    def test_count_cef_inits(self):
        init = '{"message":"startup_trace: milestone","milestone":"cef_initialized"}'
        other = '{"message":"startup_trace: milestone","milestone":"process_entry"}'
        noise = '{"message":"cef: InitWithApp returned OK"}'
        self.assertEqual(harness.count_cef_inits([]), 0)
        self.assertEqual(harness.count_cef_inits([other, "not json"]), 0)
        self.assertEqual(harness.count_cef_inits([other, init]), 1)
        self.assertEqual(harness.count_cef_inits([init, noise, init]), 2)

    def test_second_window_records_owner_init_count(self):
        binary = os.path.join(self.temp, "fakebin")
        with open(binary, "w") as handle:
            handle.write("#!/bin/sh\nexit 0\n")
        os.chmod(binary, 0o755)
        log_path = os.path.join(self.temp, "owner.log")
        with open(log_path, "w") as handle:
            handle.write('{"message":"startup_trace: milestone","milestone":"cef_initialized"}\n')
        owner_dirs = dict(self.dirs)
        owner_dirs["owner_log"] = log_path
        result = harness.run_sample(binary, self.cef_dir, "relay-window", "static", 1,
                                    owner=None, owner_dirs=owner_dirs, owner_socket=None)
        self.assertTrue(result["fallback_spawn"])
        self.assertEqual(result["owner_cef_init_count"], 1)

    def test_second_window_missing_owner_log_is_none(self):
        binary = os.path.join(self.temp, "fakebin")
        with open(binary, "w") as handle:
            handle.write("#!/bin/sh\nexit 0\n")
        os.chmod(binary, 0o755)
        owner_dirs = dict(self.dirs)
        owner_dirs["owner_log"] = os.path.join(self.temp, "absent.log")
        result = harness.run_sample(binary, self.cef_dir, "relay-window", "static", 1,
                                    owner=None, owner_dirs=owner_dirs, owner_socket=None)
        self.assertIsNone(result["owner_cef_init_count"])


class ReopenWindowTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.mkdtemp()
        self.cef_dir = os.path.join(self.temp, "cef")
        os.makedirs(self.cef_dir)

    def test_diagnostic_close_acknowledged(self):
        import json
        import socket
        socket_path = os.path.join(self.temp, "test.sock")
        listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        listener.bind(socket_path)
        listener.listen(1)
        listener.settimeout(5)

        def serve_once():
            conn, _ = listener.accept()
            with conn:
                conn.settimeout(5)
                data = b""
                while b"\n" not in data:
                    chunk = conn.recv(4096)
                    if not chunk:
                        break
                    data += chunk
                request = json.loads(data.decode())
                assert request["action"] == "close-all-windows"
                conn.sendall(json.dumps({"request_id": request["request_id"],
                                         "accepted": True}).encode() + b"\n")

        thread = threading.Thread(target=serve_once)
        thread.daemon = True
        thread.start()
        try:
            self.assertTrue(harness.send_diagnostic_close(socket_path))
        finally:
            thread.join(timeout=5)
            listener.close()

    def test_diagnostic_close_refused_without_server(self):
        self.assertFalse(harness.send_diagnostic_close(os.path.join(self.temp, "missing.sock"), 1.0))

    def test_proc_helpers_observe_current_process(self):
        self.assertIsInstance(harness.child_pids(os.getpid()), list)
        rss = harness.proc_rss_kb(os.getpid())
        self.assertIsNotNone(rss)
        self.assertGreater(rss, 0)
        self.assertIsNone(harness.proc_rss_kb(2 ** 30))

    def test_proc_starttime_identifies_current_process(self):
        first = harness.proc_starttime(os.getpid())
        self.assertIsNotNone(first)
        self.assertGreaterEqual(first, 0)
        self.assertEqual(first, harness.proc_starttime(os.getpid()))
        self.assertIsNone(harness.proc_starttime(2 ** 30))

    def test_child_quiescence_returns_stable_count(self):
        count = harness.wait_for_child_quiescence(os.getpid(), settle_seconds=0.2, timeout_seconds=5.0)
        self.assertIsInstance(count, int)
        self.assertGreaterEqual(count, 0)

    def test_reopen_without_owner_falls_back(self):
        binary = os.path.join(self.temp, "fakebin")
        with open(binary, "w") as handle:
            handle.write("#!/bin/sh\nexit 0\n")
        os.chmod(binary, 0o755)
        result = harness.run_sample(binary, self.cef_dir, "reopen-window", "static", 1,
                                    owner=None, owner_dirs=None, owner_socket=None)
        self.assertTrue(result["fallback_spawn"])
        self.assertFalse(result["relayed"])
        self.assertNotIn("reopen", result)


if __name__ == "__main__":
    unittest.main()
