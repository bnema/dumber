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


if __name__ == "__main__":
    unittest.main()
