package ui

import "github.com/bnema/dumber/internal/application/port"

type runtimeConfigState struct {
	provider port.RuntimeConfigProvider
	snapshot port.RuntimeConfigSnapshot
	loaded   bool
}

func newRuntimeConfigState(provider port.RuntimeConfigProvider) *runtimeConfigState {
	state := &runtimeConfigState{provider: provider}
	if provider != nil {
		state.Update(provider.Current())
	}
	return state
}

func (s *runtimeConfigState) Current() port.RuntimeConfigSnapshot {
	if s == nil {
		return port.RuntimeConfigSnapshot{}
	}
	if s.loaded {
		return s.snapshot
	}
	if s.provider != nil {
		return s.provider.Current()
	}
	return port.RuntimeConfigSnapshot{}
}

func (s *runtimeConfigState) Update(snapshot port.RuntimeConfigSnapshot) {
	if s == nil {
		return
	}
	s.snapshot = snapshot
	s.loaded = true
}
