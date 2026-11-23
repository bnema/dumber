package runtimewatch

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestGoroutinesGrowing(t *testing.T) {
	tests := []struct {
		name    string
		samples []uint64
		want    bool
	}{
		{name: "short window", samples: []uint64{10}, want: false},
		{name: "flat", samples: []uint64{10, 10, 10, 10, 10}, want: false},
		{name: "one dip", samples: []uint64{10, 30, 25, 50, 70}, want: false},
		{name: "growth below minimum", samples: []uint64{10, 20, 30, 40, 59}, want: false},
		{name: "growth at minimum", samples: []uint64{10, 20, 30, 40, 60}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, goroutinesGrowing(tt.samples, 50))
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	defaults := Config{MetricsInterval: 60 * time.Second, LeakWindow: 5, LeakMinGrowth: 50}
	verbose := Config{
		MetricsInterval: 10 * time.Second, ForcedGCInterval: 2 * time.Second,
		LeakWindow: 5, LeakMinGrowth: 50, Verbose: true,
	}
	tests := []struct {
		name  string
		value string
		want  Config
	}{
		{name: "unset", value: "", want: defaults},
		{name: "one", value: "1", want: verbose},
		{name: "on", value: " ON ", want: verbose},
		{name: "zero", value: "0", want: defaults},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == EnvVar {
					return tt.value
				}
				return ""
			}
			assert.Equal(t, tt.want, ConfigFromEnv(getenv))
		})
	}
}

func TestStart_LogsMetricsOnEachTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		logger := zerolog.New(&buf).Level(zerolog.DebugLevel)
		cfg := Config{MetricsInterval: 10 * time.Second, LeakWindow: 5, LeakMinGrowth: 50, Verbose: true}

		stop := Start(context.Background(), cfg, &logger)
		time.Sleep(25 * time.Second)
		stop()
		stop() // idempotent

		out := buf.String()
		assert.Equal(t, 1, strings.Count(out, "runtime watch enabled"))
		assert.Equal(t, 2, strings.Count(out, `"message":"runtime metrics"`))
	})
}

func TestRun_StopsOnContextDone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := zerolog.Nop()
		cfg := Config{MetricsInterval: time.Second, ForcedGCInterval: time.Second, LeakWindow: 5}
		w := newWatcher(cfg, &logger)
		ctx, cancel := context.WithCancel(context.Background())
		exited := make(chan struct{})
		go func() {
			w.run(ctx, nil)
			close(exited)
		}()

		cancel()
		synctest.Wait()

		select {
		case <-exited:
		default:
			t.Fatal("run did not exit after ctx was canceled")
		}
	})
}

type observation struct {
	warn        bool
	first, last uint64
}

func TestWatcherObserve(t *testing.T) {
	warnAt := func(first, last uint64) observation { return observation{true, first, last} }
	quiet := observation{}
	tests := []struct {
		name    string
		samples []uint64
		want    []observation
	}{
		{
			name:    "warns on the 5th growing sample",
			samples: []uint64{10, 20, 30, 40, 60},
			want:    []observation{quiet, quiet, quiet, quiet, warnAt(10, 60)},
		},
		{
			name:    "ring resets after a warning",
			samples: []uint64{10, 20, 30, 40, 60, 70, 80, 90, 100, 120},
			want: []observation{
				quiet, quiet, quiet, quiet, warnAt(10, 60),
				quiet, quiet, quiet, quiet, warnAt(70, 120),
			},
		},
		{
			name:    "a dip delays the warning",
			samples: []uint64{10, 20, 15, 30, 40, 50, 70},
			want:    []observation{quiet, quiet, quiet, quiet, quiet, quiet, warnAt(15, 70)},
		},
		{
			name:    "growth below the minimum never warns",
			samples: []uint64{10, 11, 12, 13, 14, 15, 16, 17},
			want:    []observation{quiet, quiet, quiet, quiet, quiet, quiet, quiet, quiet},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zerolog.Nop()
			w := newWatcher(Config{LeakWindow: 5, LeakMinGrowth: 50}, &logger)
			got := make([]observation, 0, len(tt.samples))
			for _, n := range tt.samples {
				warn, first, last := w.observe(n)
				got = append(got, observation{warn, first, last})
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
