package port

import "bnema.dev/dumber/internal/domain/entity"

type RuntimeConfigProvider interface {
	Current() entity.RuntimeConfigSnapshot
	Watch() error
	OnChange(func(entity.RuntimeConfigSnapshot))
}
