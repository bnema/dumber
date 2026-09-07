#!/usr/bin/env python3
"""Unit tests for measure_cef_startup.py (stdlib only)."""

import json
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import measure_cef_startup as harness


class ParseTest(unittest.TestCase):
    def test_accepts_valid_fixture_event(self):
        token = "a" * 16
        nav = "b" * 16
        lines = [
            json.dumps({"message": "cef-startup-fixture", "event": "first-contentful-paint",
                        "run_token": token, "nav_token": nav, "value_ms": 12}),
        ]
        _, events = harness.parse_child_output(lines, token)
        self.assertEqual(len(events), 1)

    def test_rejects_wrong_token(self):
        lines = [
            json.dumps({"message": "cef-startup-fixture", "event": "first-contentful-paint",
                        "run_token": "c" * 16, "nav_token": "d" * 16, "value_ms": 12}),
        ]
        _, events = harness.parse_child_output(lines, "a" * 16)
        self.assertEqual(events, [])

    def test_rejects_non_numeric_value(self):
        token = "a" * 16
        lines = [
            json.dumps({"message": "cef-startup-fixture", "event": "first-contentful-paint",
                        "run_token": token, "nav_token": "b" * 16, "value_ms": "12"}),
        ]
        _, events = harness.parse_child_output(lines, token)
        self.assertEqual(events, [])

    def test_rejects_malformed_token(self):
        lines = [
            json.dumps({"message": "cef-startup-fixture", "event": "first-contentful-paint",
                        "run_token": "short", "nav_token": "b" * 16, "value_ms": 12}),
        ]
        _, events = harness.parse_child_output(lines, "short")
        self.assertEqual(events, [])


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

    def run_harness(self, *extra):
        cmd = [sys.executable, self.script, "--binary", self.binary, "--cef-dir", self.cef_dir,
               "--scenario", "profile-fresh", "--fixture", "static", "--runs", "1"] + list(extra)
        return subprocess.run(cmd, capture_output=True, text=True)

    def test_rejects_existing_output(self):
        existing = os.path.join(self.temp, "exists")
        os.makedirs(existing)
        result = self.run_harness("--output", existing)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("fresh absolute directory", result.stderr)

    def test_rejects_relative_output(self):
        result = self.run_harness("--output", "relative/path")
        self.assertNotEqual(result.returncode, 0)

    def test_rejects_missing_binary(self):
        result = subprocess.run(
            [sys.executable, self.script, "--binary", os.path.join(self.temp, "missing"),
             "--cef-dir", self.cef_dir, "--scenario", "profile-fresh", "--fixture", "static",
             "--runs", "1", "--output", os.path.join(self.temp, "out")],
            capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)

    def test_rejects_bad_runs(self):
        result = self.run_harness("--output", os.path.join(self.temp, "out"), "--runs", "0")
        # argparse passes --runs twice; last wins; use explicit override instead
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

    def make_binary(self, body):
        path = os.path.join(self.temp, "fake-%d" % len(os.listdir(self.temp)))
        with open(path, "w") as handle:
            handle.write("#!/bin/sh\n" + body + "\n")
        os.chmod(path, 0o755)
        return path

    def test_duplicate_documents_marked(self):
        binary = self.make_binary("exit 0")
        result = harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1, None, None, {})
        # No fixture events and no document requests through the real binary:
        # the sample must report incomplete, never synthesized success.
        self.assertFalse(result["complete"])
        self.assertIsNone(result["target_fcp_ms"])

    def test_absent_fcp_is_incomplete(self):
        binary = self.make_binary("echo '{\"message\":\"startup_trace: milestone\"}'; exit 0")
        result = harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1, None, None, {})
        self.assertFalse(result["complete"])

    def test_relay_fallback_recorded(self):
        binary = self.make_binary("exit 0")
        result = harness.run_sample(binary, self.cef_dir, "relay-window", "static", 1, None, None, {})
        self.assertTrue(result["fallback_spawn"])
        self.assertFalse(result["relayed"])

    def test_second_window_uses_observation_not_process_summary(self):
        binary = self.make_binary("exit 0")
        first = harness.run_sample(binary, self.cef_dir, "profile-fresh", "static", 1, None, None, {})
        second = harness.run_sample(binary, self.cef_dir, "relay-window", "static", 2, None, None, {})
        self.assertIn("observation_spawn_to_summary_ms", first)
        self.assertIn("observation_spawn_to_summary_ms", second)
        self.assertNotIn("total_ms", second)


if __name__ == "__main__":
    unittest.main()
