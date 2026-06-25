package usecase

import (
	"context"
	"reflect"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
)

type noopHistoryChangeSink struct{}

func (noopHistoryChangeSink) OnHistoryChanged(_ context.Context, _ dto.HistoryChange) {}

func normalizeHistoryChangeSink(sink port.HistoryChangeSink) port.HistoryChangeSink {
	if isNilHistoryChangeSink(sink) {
		return noopHistoryChangeSink{}
	}
	return sink
}

func isNilHistoryChangeSink(sink port.HistoryChangeSink) bool {
	return isNilInterface(sink)
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
