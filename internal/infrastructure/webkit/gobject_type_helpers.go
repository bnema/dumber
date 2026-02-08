package webkit

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/purego"
	"github.com/jwijenbergh/puregotk/pkg/core"
	gtypes "github.com/jwijenbergh/puregotk/v4/gobject/types"
)

var (
	loadGObjectTypeFnsOnce sync.Once
	typeCheckInstanceIsA   func(uintptr, gtypes.GType) bool
	typeNameFromInstance   func(uintptr) string
)

func gobjectTypeCheckInstanceIsAByPtr(ctx context.Context, instancePtr uintptr, requestType gtypes.GType) bool {
	if instancePtr == 0 {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loadGObjectTypeFns(ctx)
	if typeCheckInstanceIsA == nil {
		return false
	}
	return typeCheckInstanceIsA(instancePtr, requestType)
}

func permissionRequestTypeName(ctx context.Context, instancePtr uintptr) string {
	if instancePtr == 0 {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	loadGObjectTypeFns(ctx)
	if typeNameFromInstance == nil {
		return ""
	}
	return typeNameFromInstance(instancePtr)
}

func loadGObjectTypeFns(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	loadGObjectTypeFnsOnce.Do(func() {
		log := logging.FromContext(ctx)
		libs := make([]uintptr, 0, 2)
		for _, libPath := range core.GetPaths("GOBJECT") {
			lib, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				log.Debug().Str("path", libPath).Err(err).Msg("failed to load GObject library")
				continue
			}
			libs = append(libs, lib)
		}
		if len(libs) == 0 {
			log.Debug().Msg("no GObject libraries loaded; permission type helpers unavailable")
			return
		}

		core.PuregoSafeRegister(&typeCheckInstanceIsA, libs, "g_type_check_instance_is_a")
		if typeCheckInstanceIsA == nil {
			log.Debug().Msg("failed to register g_type_check_instance_is_a")
		}
		core.PuregoSafeRegister(&typeNameFromInstance, libs, "g_type_name_from_instance")
		if typeNameFromInstance == nil {
			log.Debug().Msg("failed to register g_type_name_from_instance")
		}
	})
}
