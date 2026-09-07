#!/usr/bin/env python3
"""Local CEF startup harness with synthetic fixtures (stdlib only).

Scenarios:
  process-warm   one unrecorded warm-up, then new processes sharing an
                 isolated previously initialized CEF root-cache profile
  profile-fresh  new process, new profile each sample
  relay-window   a real long-lived owner browser holds the relay; samples
                 launch browse with DUMBER_BROWSER_FRESH_WINDOW=1 in that
                 same isolated profile

Fixtures (loopback only): static, delayed, redirect, cache.

Protocol: fixture pages report PerformanceObserver results through a
token-authenticated loopback beacon (/__beacon with run/nav/event/value
parameters) served by the harness HTTP server. Beacon arrivals carry the
harness monotonic timestamp. External spawn-to-arrival is recorded as an
explicitly named observation duration, including output-delivery overhead;
it is never presented as an exact child event timestamp.

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
import shutil
import socket
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

TOKEN_RE = re.compile(r"^[0-9a-f]{16}$")
NAV_RE = re.compile(r"^[0-9a-f]{16}$")
BEACON_EVENTS = ("first-contentful-paint", "largest-contentful-paint", "fixture-load")
DEFAULT_POST_NAV_CUTOFF_SECONDS = 5.0


def post_nav_cutoff_seconds():
    try:
        value = float(os.environ.get("DUMBER_MEASURE_CUTOFF_SECONDS", DEFAULT_POST_NAV_CUTOFF_SECONDS))
    except ValueError:
        return DEFAULT_POST_NAV_CUTOFF_SECONDS
    return min(30.0, max(0.1, value))


class FixtureState:
    def __init__(self):
        self.lock = threading.Lock()
        self.document_requests = []
        self.subresource_requests = []
        self.beacons = []
        self.first_document_monotonic = None


def build_fixture_html(kind, run_token, nav_token):
    observer = (
        "<script>(function(){"
        "function beacon(event, value){"
        "fetch('/__beacon?run=%s&nav=%s&event='+event+'&value='+value,"
        "{cache:'no-store'}).catch(function(){});"
        "}"
        "try{"
        "var po=new PerformanceObserver(function(list){"
        "list.getEntries().forEach(function(en){"
        "if(en.name==='first-contentful-paint'){beacon('first-contentful-paint',Math.round(en.startTime));}"
        "else if(en.entryType==='largest-contentful-paint'){beacon('largest-contentful-paint',Math.round(en.startTime));}"
        "}});"
        "po.observe({type:'paint',buffered:true});"
        "po.observe({type:'largest-contentful-paint',buffered:true});"
        "}catch(e){}"
        "window.addEventListener('load',function(){"
        "setTimeout(function(){beacon('fixture-load',0);},100);});"
        "})();</script>" % (run_token, nav_token)
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
            entry = {"path": self.path, "time": time.monotonic()}
            if document:
                self.state.document_requests.append(entry)
                if self.state.first_document_monotonic is None:
                    self.state.first_document_monotonic = entry["time"]
            else:
                self.state.subresource_requests.append(entry)

    def do_GET(self):
        parsed = urlparse(self.path)
        path = parsed.path
        if path == "/__beacon":
            self._handle_beacon(parsed)
            return
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

    def _handle_beacon(self, parsed):
        query = parse_qs(parsed.query)
        run = query.get("run", [None])[0]
        nav = query.get("nav", [None])[0]
        event = query.get("event", [None])[0]
        value_raw = query.get("value", [None])[0]
        valid = (
            isinstance(run, str) and TOKEN_RE.fullmatch(run) and run == self.run_token
            and isinstance(nav, str) and NAV_RE.fullmatch(nav) and nav == self.nav_token
            and event in BEACON_EVENTS
        )
        try:
            value = int(value_raw) if valid else None
            valid = valid and value is not None and value >= 0
        except (TypeError, ValueError):
            valid = False
        if not valid:
            self.send_response(400)
            self.end_headers()
            return
        with self.state.lock:
            self.state.beacons.append({
                "event": event,
                "value_ms": value,
                "arrival_monotonic": time.monotonic(),
            })
        self.send_response(204)
        self.end_headers()


def parse_child_output(lines):
    milestones = []
    for line in lines:
        try:
            event = json.loads(line)
        except (json.JSONDecodeError, ValueError):
            continue
        if isinstance(event, dict) and event.get("message") == "startup_trace: milestone":
            milestones.append(event)
    return milestones


def scenario_env(cef_dir, dirs, extra=None):
    env = {
        "HOME": os.environ.get("HOME", ""),
        "PATH": os.environ.get("PATH", ""),
        "LANG": os.environ.get("LANG", "C.UTF-8"),
        "DISPLAY": os.environ.get("DISPLAY", ""),
        "WAYLAND_DISPLAY": os.environ.get("WAYLAND_DISPLAY", ""),
        "XDG_CONFIG_HOME": dirs["config"],
        "XDG_DATA_HOME": dirs["data"],
        "XDG_STATE_HOME": dirs["state"],
        "XDG_CACHE_HOME": dirs["cache"],
        "XDG_RUNTIME_DIR": dirs["runtime"],
        "CEF_DIR": cef_dir,
        "DUMBER_CEF_DIR": cef_dir,
        "DUMBER_CEF_ROOT_CACHE_PATH": dirs["root_cache"],
    }
    if extra:
        env.update(extra)
    for key in ("config", "data", "state", "cache", "runtime"):
        os.makedirs(dirs[key], exist_ok=True)
    os.makedirs(dirs["root_cache"], exist_ok=True)
    return env


def wait_for_relay_socket(runtime_dir, timeout_seconds=30.0):
    end = time.monotonic() + timeout_seconds
    while time.monotonic() < end:
        for root, _, files in os.walk(runtime_dir):
            if "browser-launch.sock" in files:
                path = os.path.join(root, "browser-launch.sock")
                sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                sock.settimeout(2.0)
                try:
                    sock.connect(path)
                    return path
                except OSError:
                    pass
                finally:
                    sock.close()
        time.sleep(0.2)
    return None


def start_relay_owner(binary, cef_dir, scenario_dir):
    dirs = {
        "config": os.path.join(scenario_dir, "owner-config"),
        "data": os.path.join(scenario_dir, "owner-data"),
        "state": os.path.join(scenario_dir, "owner-state"),
        "cache": os.path.join(scenario_dir, "owner-cache"),
        "runtime": os.path.join(scenario_dir, "owner-runtime"),
        "root_cache": os.path.join(scenario_dir, "shared-profile"),
    }
    env = scenario_env(cef_dir, dirs)
    try:
        proc = subprocess.Popen(
            [binary, "browse", "about:blank"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            env=env,
        )
    except OSError:
        return None, None
    time.sleep(0.5)
    if proc.poll() is not None:
        return None, None
    if wait_for_relay_socket(dirs["runtime"]) is None:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
        return None, None
    return proc, dirs


def stop_proc(proc):
    if proc is None or proc.poll() is not None:
        return
    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=5)


def run_sample(binary, cef_dir, scenario, fixture, run_index, owner=None, owner_dirs=None,
               shared_root_cache=None):
    run_token = secrets.token_hex(8)
    nav_token = secrets.token_hex(8)
    cutoff = post_nav_cutoff_seconds()
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
    run_dir = tempfile.mkdtemp(prefix="cef-startup-run-%02d-" % run_index)
    try:
        if fixture == "redirect":
            url = "http://127.0.0.1:%d/redirect?run=%s&nav=%s" % (port, run_token, nav_token)
        else:
            url = "http://127.0.0.1:%d/?run=%s&nav=%s" % (port, run_token, nav_token)
        if scenario == "relay-window" and owner_dirs is not None:
            dirs = dict(owner_dirs)
            extra = {"DUMBER_BROWSER_FRESH_WINDOW": "1"}
        else:
            dirs = {
                "config": os.path.join(run_dir, "config"),
                "data": os.path.join(run_dir, "data"),
                "state": os.path.join(run_dir, "state"),
                "cache": os.path.join(run_dir, "cache"),
                "runtime": os.path.join(run_dir, "runtime"),
                "root_cache": shared_root_cache or os.path.join(run_dir, "cef-root-cache"),
            }
            extra = {}
        env = scenario_env(cef_dir, dirs, extra)
        owner_alive_at_start = owner is not None and owner.poll() is None
        spawn_ts = time.monotonic()
        proc = subprocess.Popen(
            [binary, "browse", url],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
            env=env,
        )
        arrivals = []

        def pump():
            try:
                for line in proc.stdout:
                    arrivals.append((time.monotonic(), line))
            except ValueError:
                pass

        pump_thread = threading.Thread(target=pump)
        pump_thread.daemon = True
        pump_thread.start()
        hard_end = spawn_ts + cutoff + 25.0
        while True:
            now = time.monotonic()
            with state.lock:
                navigated_at = state.first_document_monotonic
                fixture_done = any(
                    b["event"] == "fixture-load" for b in state.beacons
                )
            exited = proc.poll() is not None
            if navigated_at is not None:
                if fixture_done and now >= navigated_at + cutoff:
                    break
                if now >= navigated_at + cutoff + 5.0:
                    break
            elif now >= spawn_ts + cutoff + 5.0 or exited:
                break
            if now >= hard_end:
                break
            time.sleep(0.05)
        self_exited = proc.poll() is not None
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=5)
        pump_thread.join(timeout=5)
        try:
            proc.stdout.close()
        except (AttributeError, ValueError):
            pass
        decision_ts = time.monotonic()
        milestones = parse_child_output([line for _, line in arrivals])
        with state.lock:
            beacons = list(state.beacons)
            document_count = len(state.document_requests)
            subresource_count = len(state.subresource_requests)
        fcp = [b for b in beacons if b["event"] == "first-contentful-paint"]
        lcp = [b for b in beacons if b["event"] == "largest-contentful-paint"]
        if fcp:
            observation_ms = int((fcp[0]["arrival_monotonic"] - spawn_ts) * 1000)
        elif beacons:
            observation_ms = int((beacons[-1]["arrival_monotonic"] - spawn_ts) * 1000)
        else:
            observation_ms = int((decision_ts - spawn_ts) * 1000)
        if fixture == "redirect":
            complete = document_count >= 1 and len(fcp) > 0
        else:
            complete = document_count == 1 and len(fcp) > 0
        if scenario == "relay-window":
            relayed = bool(owner_alive_at_start and self_exited)
        else:
            relayed = False
        return {
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
            "startup_milestones": len(milestones),
            "complete": complete,
            "relayed": relayed,
            "fallback_spawn": scenario == "relay-window" and not relayed,
        }
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)
        shutil.rmtree(run_dir, ignore_errors=True)


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
    owner = None
    owner_dirs = None
    shared_root_cache = None
    warmup_discarded = False
    if args.scenario == "process-warm":
        shared_root_cache = os.path.join(args.output, "warm-profile", "cef-root-cache")
        os.makedirs(shared_root_cache)
    if args.scenario == "relay-window":
        scenario_dir = os.path.join(args.output, "relay-scenario")
        os.makedirs(scenario_dir)
        owner, owner_dirs = start_relay_owner(args.binary, args.cef_dir, scenario_dir)

    results = []
    try:
        if args.scenario == "process-warm":
            # One unrecorded warm-up initializes the shared profile; only the
            # recorded samples below count toward the report.
            run_sample(args.binary, args.cef_dir, args.scenario, args.fixture,
                       0, shared_root_cache=shared_root_cache)
            warmup_discarded = True
        for index in range(1, args.runs + 1):
            result = run_sample(
                args.binary, args.cef_dir, args.scenario, args.fixture, index,
                owner=owner, owner_dirs=owner_dirs, shared_root_cache=shared_root_cache,
            )
            results.append(result)
            with open(os.path.join(args.output, "run-%02d.json" % index), "w") as handle:
                json.dump(result, handle, indent=2, sort_keys=True)
    finally:
        stop_proc(owner)

    summary = {
        "binary_sha256": binary_sha256,
        "scenario": args.scenario,
        "fixture": args.fixture,
        "runs": len(results),
        "complete_runs": sum(1 for r in results if r["complete"]),
        "incomplete_runs": sum(1 for r in results if not r["complete"]),
        "duplicate_document_runs": sum(
            1 for r in results if args.fixture != "redirect" and r["document_requests"] != 1
        ),
        "warmup_discarded": warmup_discarded,
        "relay_owner_ready": owner is not None,
    }
    with open(os.path.join(args.output, "summary.json"), "w") as handle:
        json.dump(summary, handle, indent=2, sort_keys=True)
    print("measure artifacts: %s" % args.output)


if __name__ == "__main__":
    main()
