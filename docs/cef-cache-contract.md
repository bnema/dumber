# CEF cache contract

Current product behavior for the CEF engine. No configuration change is
proposed here; setting an explicit `CachePath` requires the P3.3
compatibility decision with data-preservation tests and approval.

## Configured settings

- `RootCachePath` is set to the resolved state root (`DUMBER_CEF_ROOT_CACHE_PATH`
  override, engine cache dir, data dir, or profile default, in that order).
- `CachePath` is intentionally left empty: with an empty `CachePath` CEF runs
  the global request context incognito-style, with in-memory-only storage.
  `RootCachePath` does NOT implicitly supply `CachePath` and does not grant
  persistent `localStorage`: HTML5 storage only persists across sessions with
  a non-empty `CachePath`. Locked by
  `TestPrepareCEFSettings_LeavesCachePathToCEFDefault`.
- Browsers are created with a nil request context (`factory.go`), so every
  window shares the global request context and its storage. No per-window
  isolation is claimed.

## Observed behavior

Orderly-restart runs once suggested HTTP asset reuse and localStorage
persistence across restarts with this configuration, but that observation is
unverified against CEF semantics: with an empty `CachePath` the global
context is incognito-style and profile data (including `localStorage`) is
expected to be memory-only, so restart persistence must NOT be assumed. The
seed/read-only probes below are the only accepted proof; until they run
green on a display host, treat this configuration as non-persistent.
Setting an explicit `CachePath` under `RootCachePath` requires the P3.3
compatibility decision with data-preservation tests and approval.

## Display-side verification (pending)

The binding exposes everything the probe needs; no new API is required:

1. `RequestContextGetGlobalContext()` plus
   `BrowserHost.GetRequestContext()` identity comparison proves windows
   share the global context.
2. `RequestContext.GetCachePath()` equality/containment against the
   configured state root proves the effective on-disk location. Export
   only equality and persistence assertions, never paths.
3. Seed/read-only probes: seed cacheable assets, localStorage, and
   synthetic cookies at a stable fixture origin, restart orderly, and read
   before any reseeding. Distinct-profile probes also read before writing.
   Include a no-store control and distinguish no-request hits from
   conditional revalidation. Assert no non-fixture destination is
   contacted (favicon-provider egress control included).
4. Never infer storage behavior from forcibly killed helpers; always shut
   down gracefully and let CEF stop its helpers.
