package ui

import (
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

type runtimeConfigState struct {
	provider port.RuntimeConfigProvider
	snapshot entity.RuntimeConfigSnapshot
	loaded   bool
}

func newRuntimeConfigState(provider port.RuntimeConfigProvider) *runtimeConfigState {
	state := &runtimeConfigState{provider: provider}
	if provider != nil {
		state.Update(provider.Current())
	}
	return state
}

func (s *runtimeConfigState) Current() entity.RuntimeConfigSnapshot {
	if s == nil {
		return entity.RuntimeConfigSnapshot{}
	}
	if s.loaded {
		return s.snapshot
	}
	if s.provider != nil {
		return s.provider.Current()
	}
	return entity.RuntimeConfigSnapshot{}
}

func (s *runtimeConfigState) Update(snapshot entity.RuntimeConfigSnapshot) {
	if s == nil {
		return
	}
	s.snapshot = snapshot
	s.loaded = true
}
