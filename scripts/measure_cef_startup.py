#!/usr/bin/env python3
"""Local CEF startup harness with synthetic fixtures (stdlib only).

Scenarios:
  process-warm   one unrecorded warm-up, then new processes sharing an
                 isolated previously initialized CEF root-cache profile
  profile-fresh  new process, new profile each sample
  relay-window   a real long-lived owner browser holds the relay; samples
                 launch browse with DUMBER_BROWSER_FRESH_WINDOW=1 in that
                 same isolated profile
  reopen-window  like relay-window, but each sample first closes all owner
                 windows through the diagnostic relay action, then reopens
                 through the relay; verifies the same owner process serves
                 the reopen without a second CEF initialization

Second-window contract (relay-window, reopen-window): the owner captures
its own log, readiness is its first presentation (never a fixed sleep),
and every sample records owner_cef_init_count, which must stay at 1.
Cold-process numbers (process-warm, profile-fresh) and resident-reopen
observations are reported separately and never averaged together.
--idle-timeout-ms forwards the product residency knob into generated
configs (default 0 = exit with last window; reopen-window only reuses
the same process with a nonzero timeout).

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

# Deterministic 1x1 RGBA PNG so image endpoints exercise a successful decode
# and paint instead of the decode-failure path.
FIXTURE_PNG = bytes.fromhex(
    "89504e470d0a1a0a"
    "0000000d49484452000000010000000108060000001f15c489"
    "0000000b4944415478da6360000300000700012122db13"
    "0000000049454e44ae426082"
)


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
            if path.endswith(".png"):
                body = FIXTURE_PNG
            else:
                body = b"/* fixture */"
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


DISPLAY_PASSTHROUGH_VARS = ("DBUS_SESSION_BUS_ADDRESS", "XAUTHORITY")


def _escape_toml_basic(value):
    return value.replace("\\", "\\\\").replace('"', '\\"')


def write_child_config(config_home, cef_dir, idle_timeout_ms=0):
    """Force JSON debug logging so milestone parsing works.

    Without this, isolated XDG homes fall back to text logs and
    parse_child_output finds zero milestones. Idempotent: relay owners
    share one config across samples. idle_timeout_ms selects the product
    residency knob (0 = product default, exit with last window)."""
    config_dir = os.path.join(config_home, "dumber")
    os.makedirs(config_dir, exist_ok=True)
    path = os.path.join(config_dir, "config.toml")
    content = (
        '[logging]\nlevel = "debug"\nformat = "json"\n'
        'enable_file_log = false\n\n[engine.cef]\ncef_dir = "%s"\n'
        'idle_runtime_timeout_ms = %d\n'
        % (_escape_toml_basic(cef_dir), idle_timeout_ms)
    )
    with open(path, "w") as handle:
        handle.write(content)
    return path


def ensure_wayland_socket(runtime_dir, env):
    """Point a relative Wayland display at the real compositor socket.

    Isolating XDG_RUNTIME_DIR breaks relative WAYLAND_DISPLAY values
    (the compositor socket lives in the real runtime dir). Symlinking the
    socket into the isolated dir is not an option: isolated test paths
    easily exceed the 108-byte unix-socket limit and CEF aborts with
    "File name too long". Instead, when the display is a relative name
    whose socket exists in the real runtime dir, override WAYLAND_DISPLAY
    with its absolute path, which Wayland clients use as-is. The isolated
    runtime dir stays untouched. Returns True when rendering connectivity
    is preserved or Wayland is not in use."""
    display = env.get("WAYLAND_DISPLAY", "")
    if not display or "/" in display:
        return True
    os.makedirs(runtime_dir, exist_ok=True)
    real_runtime = os.environ.get("XDG_RUNTIME_DIR", "/run/user/%d" % os.getuid())
    source = os.path.join(real_runtime, display)
    if os.path.exists(source):
        env["WAYLAND_DISPLAY"] = source
        return True
    return False


def count_cef_inits(lines):
    """Count CEF initialization completions in child output lines.

    Each process initializes CEF at most once: a resident owner serving
    N second windows must still show exactly one cef_initialized
    milestone. Only the milestone event counts; the InitWithApp log line
    describes the same init and is ignored."""
    count = 0
    for line in lines:
        try:
            event = json.loads(line)
        except (json.JSONDecodeError, ValueError):
            continue
        if (
            isinstance(event, dict)
            and event.get("message") == "startup_trace: milestone"
            and event.get("milestone") == "cef_initialized"
        ):
            count += 1
    return count


def wait_for_owner_log_line(log_path, needle, timeout_seconds=30.0):
    """Wait until the owner log contains needle. Bounded; False on timeout."""
    end = time.monotonic() + timeout_seconds
    while time.monotonic() < end:
        try:
            with open(log_path, errors="replace") as handle:
                for line in handle:
                    if needle in line:
                        return True
        except OSError:
            pass
        time.sleep(0.2)
    return False


def scenario_env(cef_dir, dirs, extra=None, idle_timeout_ms=0):
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
    for name in DISPLAY_PASSTHROUGH_VARS:
        value = os.environ.get(name)
        if value:
            env[name] = value
    for key in ("config", "data", "state", "cache", "runtime"):
        os.makedirs(dirs[key], exist_ok=True)
    os.makedirs(dirs["root_cache"], exist_ok=True)
    write_child_config(dirs["config"], cef_dir, idle_timeout_ms)
    ensure_wayland_socket(dirs["runtime"], env)
    return env


def wait_for_relay_socket(search_root, timeout_seconds=30.0):
    # The relay socket lives under XDG_STATE_HOME
    # (<state>/[dumber/]runtime/<engine>/browser-launch.sock), not the
    # XDG_RUNTIME dir: walk the state tree. Keep scenario outputs shallow:
    # unix-socket paths are limited to 108 bytes and deep evidence dirs
    # make the relay unreachable.
    end = time.monotonic() + timeout_seconds
    while time.monotonic() < end:
        for root, _, files in os.walk(search_root):
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


def start_relay_owner(binary, cef_dir, scenario_dir, diagnostic_close=False,
                      idle_timeout_ms=0):
    dirs = {
        "config": os.path.join(scenario_dir, "owner-config"),
        "data": os.path.join(scenario_dir, "owner-data"),
        "state": os.path.join(scenario_dir, "owner-state"),
        "cache": os.path.join(scenario_dir, "owner-cache"),
        "runtime": os.path.join(scenario_dir, "owner-runtime"),
        "root_cache": os.path.join(scenario_dir, "shared-profile"),
        "owner_log": os.path.join(scenario_dir, "owner.log"),
    }
    # scenario_env only reads the six XDG/root-cache keys; owner_log
    # rides along for CEF-init counting without affecting the child env.
    env = scenario_env(cef_dir, dirs, idle_timeout_ms=idle_timeout_ms)
    if diagnostic_close:
        # Owned window-close mechanism for the reopen scenario only: the
        # relay honors it solely in processes carrying this variable. It is
        # never a general unauthenticated shutdown command.
        env = dict(env)
        env["DUMBER_DIAGNOSTIC_WINDOW_CLOSE"] = "1"
    log_file = open(dirs["owner_log"], "w")
    try:
        proc = subprocess.Popen(
            [binary, "browse", "about:blank"],
            stdout=log_file,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
            env=env,
        )
    except OSError:
        log_file.close()
        return None, None, None
    if proc.poll() is not None:
        log_file.close()
        return None, None, None
    # Readiness is the owner's first presentation, not a fixed sleep:
    # only then is the relay guaranteed to serve second windows.
    ready = wait_for_owner_log_line(
        dirs["owner_log"], "startup_trace: first presentation")
    log_file.close()
    if not ready or proc.poll() is not None:
        stop_proc(proc)
        return None, None, None
    socket_path = wait_for_relay_socket(dirs["state"])
    if socket_path is None:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
        return None, None, None
    return proc, dirs, socket_path


def stop_proc(proc):
    if proc is None or proc.poll() is not None:
        return
    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=5)


def send_diagnostic_close(socket_path, timeout_seconds=10.0):
    """Ask the relay owner to close all windows. Returns True only on an
    explicit acknowledgement. Never a general shutdown command: owners
    honor it solely with DUMBER_DIAGNOSTIC_WINDOW_CLOSE=1."""
    request = json.dumps({"request_id": secrets.token_hex(8), "action": "close-all-windows"}) + "\n"
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.settimeout(timeout_seconds)
    try:
        sock.connect(socket_path)
        sock.sendall(request.encode())
        received = b""
        while b"\n" not in received:
            chunk = sock.recv(4096)
            if not chunk:
                break
            received += chunk
        try:
            response = json.loads(received.decode())
        except ValueError:
            return False
        return bool(response.get("accepted")) and not response.get("error")
    except OSError:
        return False
    finally:
        sock.close()


def child_pids(root_pid):
    """Direct child pids of root_pid via /proc (stdlib only)."""
    children = []
    try:
        entries = os.listdir("/proc")
    except OSError:
        return children
    for entry in entries:
        if not entry.isdigit():
            continue
        try:
            with open("/proc/%s/stat" % entry) as handle:
                parts = handle.read().rsplit(")", 1)[1].split()
                if int(parts[1]) == root_pid:
                    children.append(int(entry))
        except (OSError, ValueError, IndexError):
            continue
    return children


def proc_starttime(pid):
    """Process start time (jiffies since boot) via /proc, or None.
    Combined with the pid it forms an independently observed identity that
    survives Popen-handle reuse and detects pid recycling."""
    try:
        with open("/proc/%d/stat" % pid) as handle:
            parts = handle.read().rsplit(")", 1)[1].split()
            return int(parts[19])
    except (OSError, ValueError, IndexError):
        return None


def wait_for_child_quiescence(owner_pid, settle_seconds=2.0, timeout_seconds=15.0):
    """Wait until the owner's child count is stable across settle_seconds.
    Window close is asynchronous: the relay acknowledgement only means the
    request was accepted, so reopening must wait for an observable steady
    state instead of a fixed sleep. Returns the stable child count."""
    end = time.monotonic() + timeout_seconds
    last_change = time.monotonic()
    last_count = len(child_pids(owner_pid))
    while time.monotonic() < end:
        time.sleep(0.5)
        count = len(child_pids(owner_pid))
        if count != last_count:
            last_count = count
            last_change = time.monotonic()
        elif time.monotonic() - last_change >= settle_seconds:
            return count
    return len(child_pids(owner_pid))


def proc_rss_kb(pid):
    """Resident memory of pid in KiB via /proc, or None."""
    try:
        with open("/proc/%d/status" % pid) as handle:
            for line in handle:
                if line.startswith("VmRSS:"):
                    return int(line.split()[1])
    except (OSError, ValueError, IndexError):
        pass
    return None


def run_sample(binary, cef_dir, scenario, fixture, run_index, owner=None, owner_dirs=None,
               shared_root_cache=None, owner_socket=None, idle_timeout_ms=0):
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
        if scenario in ("relay-window", "reopen-window") and owner_dirs is not None:
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
        env = scenario_env(cef_dir, dirs, extra, idle_timeout_ms)
        owner_alive_at_start = owner is not None and owner.poll() is None
        reopen = None
        if scenario == "reopen-window" and owner is not None and owner_socket:
            owner_pid = owner.pid
            owner_start = proc_starttime(owner_pid)
            children_before = child_pids(owner_pid)
            rss_before = proc_rss_kb(owner_pid)
            acknowledged = send_diagnostic_close(owner_socket)
            # The acknowledgement only means accepted: wait for an
            # observable steady state before measuring the reopen.
            quiescent_children = wait_for_child_quiescence(owner_pid)
            reopen = {
                "owner_pid": owner_pid,
                "owner_starttime": owner_start,
                "close_acknowledged": acknowledged,
                "browser_child_count_before": len(children_before),
                "browser_child_count_quiescent": quiescent_children,
                "owner_rss_kb_before": rss_before,
            }
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
        expected_document_requests = 2 if fixture == "redirect" else 1
        complete = document_count == expected_document_requests and len(fcp) > 0
        if scenario in ("relay-window", "reopen-window"):
            relayed = bool(owner_alive_at_start and self_exited)
        else:
            relayed = False
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
            "startup_milestones": len(milestones),
            "complete": complete,
            "relayed": relayed,
            "fallback_spawn": scenario in ("relay-window", "reopen-window") and not relayed,
        }
        if scenario in ("relay-window", "reopen-window") and owner_dirs is not None:
            # Residency proof: the owner must initialize CEF exactly once
            # no matter how many second windows it serves. A second init
            # here means the sample paid a full cold start, not a reopen.
            try:
                with open(owner_dirs.get("owner_log", ""), errors="replace") as owner_log:
                    result["owner_cef_init_count"] = count_cef_inits(owner_log)
            except OSError:
                result["owner_cef_init_count"] = None
        if reopen is not None:
            # Owner identity is verified independently of the Popen handle:
            # the same pid with a different start time is a recycled pid,
            # not the same process. Unverifiable identity counts as different.
            start_now = proc_starttime(reopen["owner_pid"]) if owner is not None else None
            owner_alive = owner is not None and owner.poll() is None
            same = bool(
                owner_alive
                and reopen["owner_starttime"] is not None
                and start_now == reopen["owner_starttime"]
            )
            reopen["owner_alive_after"] = owner_alive
            reopen["same_process"] = same
            reopen["browser_child_count_after"] = len(child_pids(owner.pid)) if owner_alive else 0
            reopen["owner_rss_kb_after"] = proc_rss_kb(owner.pid) if owner_alive else None
            result["reopen"] = reopen
        return result
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
        choices=("process-warm", "profile-fresh", "relay-window", "reopen-window"),
    )
    parser.add_argument(
        "--fixture",
        required=True,
        choices=("static", "delayed", "redirect", "cache"),
    )
    parser.add_argument("--runs", type=int, default=30)
    parser.add_argument("--output", required=True)
    parser.add_argument("--idle-timeout-ms", type=int, default=0,
                        help="resident CEF idle timeout for generated configs "
                             "(0..300000, 0 is the product default)")
    args = parser.parse_args()

    if args.idle_timeout_ms < 0 or args.idle_timeout_ms > 300000:
        print("measure: --idle-timeout-ms must be 0..300000", file=sys.stderr)
        raise SystemExit(2)

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
        # Chunked streaming hash: hashlib.file_digest needs 3.11+, this
        # stays compatible with older 3.x interpreters.
        digest = hashlib.sha256()
        for chunk in iter(lambda: candidate.read(65536), b""):
            digest.update(chunk)
        binary_sha256 = digest.hexdigest()

    os.makedirs(args.output)
    # Privacy: only summary.json and run-*.json are safe to share (timings,
    # counts, hashes). owner.log and generated configs are machine-local:
    # raw child logs and the selected --cef-dir path may contain local
    # usernames or paths. Never publish the whole directory.
    owner = None
    owner_dirs = None
    owner_socket = None
    shared_root_cache = None
    warmup_discarded = False
    if args.scenario == "process-warm":
        shared_root_cache = os.path.join(args.output, "warm-profile", "cef-root-cache")
        os.makedirs(shared_root_cache)
    if args.scenario == "relay-window":
        scenario_dir = os.path.join(args.output, "relay-scenario")
        os.makedirs(scenario_dir)
        owner, owner_dirs, owner_socket = start_relay_owner(
            args.binary, args.cef_dir, scenario_dir,
            idle_timeout_ms=args.idle_timeout_ms)
    if args.scenario == "reopen-window":
        scenario_dir = os.path.join(args.output, "reopen-scenario")
        os.makedirs(scenario_dir)
        owner, owner_dirs, owner_socket = start_relay_owner(
            args.binary, args.cef_dir, scenario_dir, diagnostic_close=True,
            idle_timeout_ms=args.idle_timeout_ms)

    results = []
    try:
        if args.scenario == "process-warm":
            # One unrecorded warm-up initializes the shared profile; only the
            # recorded samples below count toward the report. A failed
            # warm-up aborts the scenario: later runs must never use an
            # uninitialized profile.
            warmup = run_sample(args.binary, args.cef_dir, args.scenario, args.fixture,
                       0, shared_root_cache=shared_root_cache,
                       idle_timeout_ms=args.idle_timeout_ms)
            if not warmup["complete"]:
                print("measure: process-warm warm-up was incomplete", file=sys.stderr)
                raise SystemExit(1)
            warmup_discarded = True
        for index in range(1, args.runs + 1):
            result = run_sample(
                args.binary, args.cef_dir, args.scenario, args.fixture, index,
                owner=owner, owner_dirs=owner_dirs, shared_root_cache=shared_root_cache,
                owner_socket=owner_socket, idle_timeout_ms=args.idle_timeout_ms,
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
            1
            for r in results
            if r["document_requests"] != (2 if args.fixture == "redirect" else 1)
        ),
        "warmup_discarded": warmup_discarded,
        "relay_owner_ready": owner is not None,
        "idle_timeout_ms": args.idle_timeout_ms,
    }
    if args.scenario in ("relay-window", "reopen-window") and owner_dirs is not None:
        # Second-window contract, kept separate from cold-process numbers:
        # one owner CEF init total, plus the request-to-FCP distribution
        # of resident reopens. Missing FCP stays missing, never averaged in.
        try:
            with open(owner_dirs.get("owner_log", ""), errors="replace") as owner_log:
                summary["owner_cef_init_count"] = count_cef_inits(owner_log)
        except OSError:
            summary["owner_cef_init_count"] = None
        fcp_observations = sorted(
            r["observation_spawn_to_summary_ms"]
            for r in results
            if r["complete"] and r["target_fcp_ms"] is not None
        )
        summary["second_window_complete_runs"] = len(fcp_observations)
        if fcp_observations:
            summary["second_window_observation_ms"] = {
                "min": fcp_observations[0],
                "median": fcp_observations[len(fcp_observations) // 2],
                "max": fcp_observations[-1],
            }
    with open(os.path.join(args.output, "summary.json"), "w") as handle:
        json.dump(summary, handle, indent=2, sort_keys=True)
    print("measure artifacts: %s" % args.output)


if __name__ == "__main__":
    main()
