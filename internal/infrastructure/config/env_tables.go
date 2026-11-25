package config

import (
	"strings"
	"sync"

	"github.com/spf13/viper"
)

// tableSafeEnvReplacer is viper's env key replacer. Besides mapping "." to
// "_", it hides the env names of table keys (engine, engine.cef, ...).
//
// With AutomaticEnv, viper treats a set env var for a table key as the value
// of the whole table: DUMBER_ENGINE=webkit made "engine" a string and shadowed
// every engine.* key. Hiding those names means a scalar env value can never
// replace a table. DUMBER_ENGINE keeps its meaning as the engine smoke-test
// override through EngineConfig.ResolveEngineType; use DUMBER_ENGINE_TYPE to
// set engine.type through config.
type tableSafeEnvReplacer struct {
	base *strings.Replacer

	mu      sync.RWMutex
	blocked map[string]struct{}
}

func newTableSafeEnvReplacer() *tableSafeEnvReplacer {
	return &tableSafeEnvReplacer{base: strings.NewReplacer(".", "_")}
}

// Replace implements viper.StringReplacer. A blocked name maps to "", which
// os.LookupEnv never finds.
func (r *tableSafeEnvReplacer) Replace(s string) string {
	out := r.base.Replace(s)
	r.mu.RLock()
	_, blocked := r.blocked[out]
	r.mu.RUnlock()
	if blocked {
		return ""
	}
	return out
}

// refresh recomputes the blocked env names from the known leaf keys: every
// proper prefix of a leaf is a table. Names that are also a leaf's env name
// stay visible.
func (r *tableSafeEnvReplacer) refresh(prefix string, leafKeys []string) {
	envName := func(key string) string {
		name := strings.ToUpper(r.base.Replace(key))
		if prefix != "" {
			name = strings.ToUpper(prefix) + "_" + name
		}
		return name
	}
	leaves := make(map[string]struct{}, len(leafKeys))
	for _, key := range leafKeys {
		leaves[envName(key)] = struct{}{}
	}
	blocked := make(map[string]struct{})
	for _, key := range leafKeys {
		parts := strings.Split(key, ".")
		for i := 1; i < len(parts); i++ {
			name := envName(strings.Join(parts[:i], "."))
			if _, isLeaf := leaves[name]; !isLeaf {
				blocked[name] = struct{}{}
			}
		}
	}
	r.mu.Lock()
	r.blocked = blocked
	r.mu.Unlock()
}

// newConfigViper returns a viper instance using a tableSafeEnvReplacer.
func newConfigViper() (*viper.Viper, *tableSafeEnvReplacer) {
	replacer := newTableSafeEnvReplacer()
	return viper.NewWithOptions(viper.EnvKeyReplacer(replacer)), replacer
}

// refreshEnvTables updates the hidden table env names from the current
// defaults and config file keys.
func (m *Manager) refreshEnvTables() {
	if m == nil || m.viper == nil || m.envTables == nil {
		return
	}
	m.envTables.refresh(m.automaticEnv.prefix, m.viper.AllKeys())
}
