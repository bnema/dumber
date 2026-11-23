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
		synctest.Wait()

		out := buf.String()
		assert.Equal(t, 1, strings.Count(out, "runtime watch enabled"))
		assert.Equal(t, 2, strings.Count(out, `"message":"runtime metrics"`))
	})
}

func TestStart_StopsOnContextDone(t *testing.T) {
	synctest.Test(t, func(_ *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		stop := Start(ctx, Config{MetricsInterval: time.Second, ForcedGCInterval: time.Second}, nil)
		cancel()
		synctest.Wait()
		stop()
	})
}
