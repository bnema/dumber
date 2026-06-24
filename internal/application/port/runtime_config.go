package port

import "github.com/bnema/dumber/internal/domain/entity"

type RuntimeConfigProvider interface {
	Current() entity.RuntimeConfigSnapshot
	Watch() error
	OnChange(func(entity.RuntimeConfigSnapshot))
}
