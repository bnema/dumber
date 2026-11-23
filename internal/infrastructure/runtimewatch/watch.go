// Package runtimewatch samples Go runtime metrics periodically and warns when
// the goroutine count keeps growing. It can also force GC cycles so heap
// corruption surfaces early as a runtime throw.
package runtimewatch

import (
	"context"
	"runtime"
	"runtime/metrics"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// EnvVar enables verbose runtime watching when truthy.
const EnvVar = "DUMBER_RUNTIME_WATCH"

const (
	defaultMetricsInterval = 60 * time.Second
	defaultLeakWindow      = 5
	defaultLeakMinGrowth   = 50
	watchMetricsInterval   = 10 * time.Second
	watchForcedGCInterval  = 2 * time.Second
)

const (
	metricGoroutines  = "/sched/goroutines:goroutines"
	metricHeapObjects = "/memory/classes/heap/objects:bytes"
	metricStacks      = "/memory/classes/heap/stacks:bytes"
	metricGCCycles    = "/gc/cycles/total:gc-cycles"
)

// Config controls the runtime watch.
type Config struct {
	MetricsInterval  time.Duration
	ForcedGCInterval time.Duration
	LeakWindow       int
	LeakMinGrowth    uint64
	Verbose          bool
}

// ConfigFromEnv returns the default config, switched to verbose watching when
// DUMBER_RUNTIME_WATCH is truthy (1, true, yes, on).
func ConfigFromEnv(getenv func(string) string) Config {
	cfg := Config{
		MetricsInterval: defaultMetricsInterval,
		LeakWindow:      defaultLeakWindow,
		LeakMinGrowth:   defaultLeakMinGrowth,
	}
	switch strings.ToLower(strings.TrimSpace(getenv(EnvVar))) {
	case "1", "true", "yes", "on":
		cfg.MetricsInterval = watchMetricsInterval
		cfg.ForcedGCInterval = watchForcedGCInterval
		cfg.Verbose = true
	}
	return cfg
}

// Start runs the watch in one goroutine until stop is called or ctx is done.
// stop is idempotent and waits for the goroutine to exit.
func Start(ctx context.Context, cfg Config, logger *zerolog.Logger) (stop func()) {
	if logger == nil {
		nop := zerolog.Nop()
		logger = &nop
	}
	if cfg.Verbose {
		logger.Info().
			Dur("metrics_interval", cfg.MetricsInterval).
			Dur("forced_gc_interval", cfg.ForcedGCInterval).
			Msg("runtime watch enabled")
	}

	done := make(chan struct{})
	finished := make(chan struct{})
	w := newWatcher(cfg, logger)
	go func() {
		defer close(finished)
		w.run(ctx, done)
	}()

	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
		<-finished
	}
}

type watcher struct {
	cfg     Config
	logger  *zerolog.Logger
	samples []metrics.Sample
	ring    []uint64
}

func newWatcher(cfg Config, logger *zerolog.Logger) *watcher {
	window := max(cfg.LeakWindow, 0)
	return &watcher{
		cfg:    cfg,
		logger: logger,
		samples: []metrics.Sample{
			{Name: metricGoroutines},
			{Name: metricHeapObjects},
			{Name: metricStacks},
			{Name: metricGCCycles},
		},
		ring: make([]uint64, 0, window),
	}
}

func (w *watcher) run(ctx context.Context, done <-chan struct{}) {
	var metricsC, gcC <-chan time.Time
	if w.cfg.MetricsInterval > 0 {
		t := time.NewTicker(w.cfg.MetricsInterval)
		defer t.Stop()
		metricsC = t.C
	}
	if w.cfg.ForcedGCInterval > 0 {
		t := time.NewTicker(w.cfg.ForcedGCInterval)
		defer t.Stop()
		gcC = t.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-metricsC:
			w.sample()
		case <-gcC:
			// No logging: on heap corruption the runtime throws into the
			// fatal output file.
			runtime.GC()
		}
	}
}

func (w *watcher) sample() {
	metrics.Read(w.samples)
	goroutines := sampleUint64(w.samples[0])

	level := zerolog.DebugLevel
	if w.cfg.Verbose {
		level = zerolog.InfoLevel
	}
	w.logger.WithLevel(level).
		Uint64("goroutines", goroutines).
		Uint64("heap_objects_bytes", sampleUint64(w.samples[1])).
		Uint64("stack_bytes", sampleUint64(w.samples[2])).
		Uint64("gc_cycles", sampleUint64(w.samples[3])).
		Msg("runtime metrics")

	if warn, first, last := w.observe(goroutines); warn {
		w.logger.Warn().
			Uint64("first", first).
			Uint64("last", last).
			Int("window", w.cfg.LeakWindow).
			Msg("runtime: goroutine count keeps growing")
	}
}

// observe adds a goroutine count to the ring. It reports a warning (with the
// window's first and last counts) when the full window keeps growing, then
// clears the ring so the next warning needs a full new window.
func (w *watcher) observe(n uint64) (warn bool, first, last uint64) {
	if w.cfg.LeakWindow <= 0 {
		return false, 0, 0
	}
	if len(w.ring) == w.cfg.LeakWindow {
		copy(w.ring, w.ring[1:])
		w.ring = w.ring[:len(w.ring)-1]
	}
	w.ring = append(w.ring, n)
	if len(w.ring) < w.cfg.LeakWindow || !goroutinesGrowing(w.ring, w.cfg.LeakMinGrowth) {
		return false, 0, 0
	}
	first, last = w.ring[0], w.ring[len(w.ring)-1]
	w.ring = w.ring[:0]
	return true, first, last
}

func sampleUint64(s metrics.Sample) uint64 {
	if s.Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return s.Value.Uint64()
}

// goroutinesGrowing reports whether every step in samples strictly increases
// and the total growth is at least minGrowth. The caller passes a full window;
// fewer than two samples never count as growth.
func goroutinesGrowing(samples []uint64, minGrowth uint64) bool {
	if len(samples) < 2 {
		return false
	}
	for i := 1; i < len(samples); i++ {
		if samples[i] <= samples[i-1] {
			return false
		}
	}
	return samples[len(samples)-1]-samples[0] >= minGrowth
}
