#!/usr/bin/env python3
"""Local CEF startup harness with synthetic fixtures (stdlib only).

Scenarios:
  process-warm   new process, isolated previously initialized profile
  profile-fresh  new process, new profile each sample
  relay-window   existing browser owns relay; launch browse with
                 DUMBER_BROWSER_FRESH_WINDOW=1 and the same isolated profile

Fixtures (loopback only): static, delayed, redirect, cache.

Protocol: the fixture pages embed a buffered PerformanceObserver that prints
single-line JSON records with opaque run/navigation tokens and numeric fields
only. The harness parses child stdout for those records plus the existing
startup_trace milestones. External spawn-to-summary is recorded as an
explicitly named observation duration, including output-delivery overhead; it
is never presented as an exact child event timestamp.

Clock domains are kept separate: child t_ms values are never subtracted from
the harness wall clock. GTK after-paint is labeled GTK paint, never compositor
presentation. Missing target FCP remains incomplete; the harness never stops
on a blank GTK summary.
"""

import argparse
import hashlib
import json
import os
import re
import secrets
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse

TOKEN_RE = re.compile(r"^[0-9a-f]{16}$")
NAV_RE = re.compile(r"^[0-9a-f]{16}$")
POST_NAV_CUTOFF_SECONDS = 5.0


class FixtureState:
    def __init__(self):
        self.lock = threading.Lock()
        self.document_requests = []
        self.subresource_requests = []


def build_fixture_html(kind, run_token, nav_token):
    observer = (
        "<script>(function(){"
        "var buf=[];"
        "function emit(e){console.log(JSON.stringify(e));}"
        "try{"
        "var po=new PerformanceObserver(function(list){"
        "list.getEntries().forEach(function(en){"
        "if(en.name==='first-contentful-paint'||en.entryType==='largest-contentful-paint'){"
        "buf.push({message:'cef-startup-fixture',event:en.name,"
        "run_token:'%s',nav_token:'%s',value_ms:Math.round(en.startTime)});"
        "}});"
        "po.observe({type:'paint',buffered:true});"
        "po.observe({type:'largest-contentful-paint',buffered:true});"
        "}catch(e){}"
        "window.addEventListener('load',function(){"
        "setTimeout(function(){buf.forEach(emit);"
        "emit({message:'cef-startup-fixture',event:'fixture-load',"
        "run_token:'%s',nav_token:'%s',value_ms:0});},100);});"
        "})();</script>" % (run_token, nav_token, run_token, nav_token)
    )
    if kind == "static":
        return (
            "<!doctype html><html><head><title>static</title>"
            '<link rel="stylesheet" href="/app.css">'
            "</head><body><h1>static</h1>"
            '<img src="/pixel.png">'
            + observer
            + "</body></html>"
        )
    if kind == "delayed":
        return (
            "<!doctype html><html><head><title>delayed</title></head>"
            "<body><h1>delayed</h1>" + observer + "</body></html>"
        )
    if kind == "redirect":
        return (
            "<!doctype html><html><head><title>target</title></head>"
            "<body><h1>target</h1>" + observer + "</body></html>"
        )
    if kind == "cache":
        return (
            "<!doctype html><html><head><title>cache</title>"
            '<script src="/cacheable.js"></script>'
            "</head><body><h1>cache</h1>"
            '<img src="/cacheable.png">'
            + observer
            + "</body></html>"
        )
    raise ValueError("unknown fixture: " + kind)


class Handler(BaseHTTPRequestHandler):
    state = None
    fixture = "static"
    run_token = ""
    nav_token = ""
    delay_seconds = 0.3

    def log_message(self, *args):
        pass

    def _record(self, document):
        with self.state.lock:
            entry = {"path": self.path, "time": time.time()}
            if document:
                self.state.document_requests.append(entry)
            else:
                self.state.subresource_requests.append(entry)

    def do_GET(self):
        parsed = urlparse(self.path)
        path = parsed.path
        if path in ("/", "/index.html", "/target"):
            self._record(True)
            if self.fixture == "delayed" and path == "/":
                time.sleep(self.delay_seconds)
            body = build_fixture_html(
                "static" if self.fixture == "redirect" and path == "/" else self.fixture,
                self.run_token,
                self.nav_token,
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if path == "/redirect":
            self._record(True)
            target = "/target?run=%s&nav=%s" % (self.run_token, self.nav_token)
            self.send_response(302)
            self.send_header("Location", target)
            self.end_headers()
            return
        if path in ("/app.css", "/pixel.png", "/cacheable.js", "/cacheable.png"):
            self._record(False)
            body = b"/* fixture */" if path.endswith((".css", ".js")) else bytes(64)
            self.send_response(200)
            if path.endswith(".css"):
                self.send_header("Content-Type", "text/css")
            elif path.endswith(".js"):
                self.send_header("Content-Type", "application/javascript")
            else:
                self.send_header("Content-Type", "image/png")
            if self.fixture == "cache":
                self.send_header("Cache-Control", "private, max-age=3600")
            else:
                self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self._record(False)
        self.send_response(404)
        self.end_headers()


def parse_child_output(lines, run_token):
    milestones = []
    fixture_events = []
    for line in lines:
        try:
            event = json.loads(line)
        except (json.JSONDecodeError, ValueError):
            continue
        if not isinstance(event, dict):
            continue
        if event.get("message") == "cef-startup-fixture":
            if (
                isinstance(event.get("run_token"), str)
                and TOKEN_RE.fullmatch(event["run_token"])
                and isinstance(event.get("nav_token"), str)
                and NAV_RE.fullmatch(event["nav_token"])
                and event["run_token"] == run_token
                and isinstance(event.get("value_ms"), int)
                and event.get("event") in ("first-contentful-paint", "largest-contentful-paint", "fixture-load")
            ):
                fixture_events.append(event)
            continue
        if event.get("message") == "startup_trace: milestone":
            milestones.append(event)
    return milestones, fixture_events


def validate_tokens_unique(records):
    seen = set()
    for record in records:
        key = (record.get("run_token"), record.get("nav_token"))
        if key in seen:
            return False
        seen.add(key)
    return True


def run_sample(binary, cef_dir, scenario, fixture, run_index, relay_proc, profile_dir, extra_env):
    run_token = secrets.token_hex(8)
    nav_token = secrets.token_hex(8)
    state = FixtureState()
    Handler.state = state
    Handler.fixture = fixture
    Handler.run_token = run_token
    Handler.nav_token = nav_token
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.05})
    thread.daemon = True
    thread.start()
    try:
        if fixture == "redirect":
            url = "http://127.0.0.1:%d/redirect?run=%s&nav=%s" % (port, run_token, nav_token)
        else:
            url = "http://127.0.0.1:%d/?run=%s&nav=%s" % (port, run_token, nav_token)
        run_dir = tempfile.mkdtemp(prefix="cef-startup-run-%02d-" % run_index)
        config_dir = os.path.join(run_dir, "config")
        os.makedirs(os.path.join(config_dir, "dumber"))
        with open(os.path.join(config_dir, "dumber", "config.toml"), "w") as config:
            config.write(
                '[logging]\nlevel = "debug"\nformat = "json"\nenable_file_log = false\n\n'
                '[engine.cef]\ncef_dir = "%s"\n' % cef_dir
            )
        env = {
            "HOME": os.environ.get("HOME", ""),
            "PATH": os.environ.get("PATH", ""),
            "LANG": os.environ.get("LANG", "C.UTF-8"),
            "DISPLAY": os.environ.get("DISPLAY", ""),
            "WAYLAND_DISPLAY": os.environ.get("WAYLAND_DISPLAY", ""),
            "XDG_CONFIG_HOME": config_dir,
            "XDG_DATA_HOME": os.path.join(run_dir, "data"),
            "XDG_STATE_HOME": os.path.join(run_dir, "state"),
            "XDG_CACHE_HOME": os.path.join(run_dir, "cache"),
            "CEF_DIR": cef_dir,
            "DUMBER_CEF_DIR": cef_dir,
            "DUMBER_CEF_ROOT_CACHE_PATH": os.path.join(run_dir, "cef-root-cache"),
        }
        if scenario == "relay-window":
            env["DUMBER_BROWSER_FRESH_WINDOW"] = "1"
        env.update(extra_env)
        for key in ("XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"):
            os.makedirs(env[key], exist_ok=True)
        if scenario == "process-warm" and profile_dir is not None:
            env["DUMBER_PROFILE_DIR"] = profile_dir
        start = time.monotonic()
        if scenario == "relay-window" and relay_proc is not None:
            proc = subprocess.Popen(
                [binary, "browse", url],
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                env=env,
            )
            relayed = relay_proc.poll() is None
        else:
            proc = subprocess.Popen(
                [binary, "browse", url],
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                env=env,
            )
            relayed = False
        try:
            stdout, _ = proc.communicate(timeout=POST_NAV_CUTOFF_SECONDS + 10)
        except subprocess.TimeoutExpired:
            proc.kill()
            stdout, _ = proc.communicate()
        observation_ms = int((time.monotonic() - start) * 1000)
        lines = stdout.splitlines()
        milestones, fixture_events = parse_child_output(lines, run_token)
        fcp = [e for e in fixture_events if e["event"] == "first-contentful-paint"]
        lcp = [e for e in fixture_events if e["event"] == "largest-contentful-paint"]
        with state.lock:
            document_count = len(state.document_requests)
            subresource_count = len(state.subresource_requests)
        if fixture == "redirect":
            complete = document_count >= 1 and len(fcp) > 0
        else:
            complete = document_count == 1 and len(fcp) > 0
        result = {
            "run": run_index,
            "scenario": scenario,
            "fixture": fixture,
            "run_token": run_token,
            "nav_token": nav_token,
            "observation_spawn_to_summary_ms": observation_ms,
            "document_requests": document_count,
            "subresource_requests": subresource_count,
            "target_fcp_ms": fcp[0]["value_ms"] if fcp else None,
            "target_lcp_through_cutoff_ms": lcp[-1]["value_ms"] if lcp else None,
            "complete": complete,
            "relayed": relayed,
            "fallback_spawn": scenario == "relay-window" and not relayed,
        }
        return result
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


def main():
    parser = argparse.ArgumentParser(description="Measure CEF startup against synthetic fixtures.")
    parser.add_argument("--binary", required=True)
    parser.add_argument("--cef-dir", required=True)
    parser.add_argument(
        "--scenario",
        required=True,
        choices=("process-warm", "profile-fresh", "relay-window"),
    )
    parser.add_argument(
        "--fixture",
        required=True,
        choices=("static", "delayed", "redirect", "cache"),
    )
    parser.add_argument("--runs", type=int, default=30)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    if args.runs < 1 or args.runs > 100:
        print("measure: --runs must be 1..100", file=sys.stderr)
        raise SystemExit(2)
    if not os.path.isabs(args.output) or os.path.exists(args.output) or os.path.islink(args.output):
        print("measure: --output must be a fresh absolute directory", file=sys.stderr)
        raise SystemExit(2)
    parent = os.path.dirname(args.output)
    if not os.path.isdir(parent) or os.path.islink(parent):
        print("measure: --output parent must be an existing directory", file=sys.stderr)
        raise SystemExit(2)
    if not (os.path.isfile(args.binary) and os.access(args.binary, os.X_OK)):
        print("measure: --binary must be executable", file=sys.stderr)
        raise SystemExit(2)
    if not os.path.isdir(args.cef_dir):
        print("measure: --cef-dir must be a directory", file=sys.stderr)
        raise SystemExit(2)
    with open(args.binary, "rb") as candidate:
        binary_sha256 = hashlib.file_digest(candidate, "sha256").hexdigest()

    os.makedirs(args.output)
    profile_dir = None
    if args.scenario == "process-warm":
        profile_dir = os.path.join(args.output, "warm-profile")
        os.makedirs(profile_dir)
    relay_proc = None
    if args.scenario == "relay-window":
        relay_proc = subprocess.Popen(
            [args.binary, "--relay-owner"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        time.sleep(0.5)
        if relay_proc.poll() is not None:
            relay_proc = None

    results = []
    try:
        for index in range(1, args.runs + 1):
            result = run_sample(
                args.binary, args.cef_dir, args.scenario, args.fixture, index, relay_proc, profile_dir, {}
            )
            results.append(result)
            with open(os.path.join(args.output, "run-%02d.json" % index), "w") as handle:
                json.dump(result, handle, indent=2, sort_keys=True)
    finally:
        if relay_proc is not None and relay_proc.poll() is None:
            relay_proc.terminate()
            try:
                relay_proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                relay_proc.kill()

    summary = {
        "binary_sha256": binary_sha256,
        "scenario": args.scenario,
        "fixture": args.fixture,
        "runs": len(results),
        "complete_runs": sum(1 for r in results if r["complete"]),
        "incomplete_runs": sum(1 for r in results if not r["complete"]),
        "duplicate_document_runs": sum(
            1 for r in results if r["document_requests"] != (1 if args.fixture != "redirect" else r["document_requests"])
        ),
    }
    with open(os.path.join(args.output, "summary.json"), "w") as handle:
        json.dump(summary, handle, indent=2, sort_keys=True)
    print("measure artifacts: %s" % args.output)


if __name__ == "__main__":
    main()
