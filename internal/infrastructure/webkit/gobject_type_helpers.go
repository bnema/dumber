package webkit

import (
	"sync"

	"github.com/jwijenbergh/purego"
	"github.com/jwijenbergh/puregotk/pkg/core"
	gtypes "github.com/jwijenbergh/puregotk/v4/gobject/types"
)

var (
	loadGObjectTypeFnsOnce sync.Once
	typeCheckInstanceIsA   func(uintptr, gtypes.GType) bool
	typeNameFromInstance   func(uintptr) string
)

func gobjectTypeCheckInstanceIsAByPtr(instancePtr uintptr, requestType gtypes.GType) bool {
	if instancePtr == 0 {
		return false
	}
	loadGObjectTypeFns()
	if typeCheckInstanceIsA == nil {
		return false
	}
	return typeCheckInstanceIsA(instancePtr, requestType)
}

func permissionRequestTypeName(instancePtr uintptr) string {
	if instancePtr == 0 {
		return ""
	}
	loadGObjectTypeFns()
	if typeNameFromInstance == nil {
		return ""
	}
	return typeNameFromInstance(instancePtr)
}

func loadGObjectTypeFns() {
	loadGObjectTypeFnsOnce.Do(func() {
		libs := make([]uintptr, 0, 2)
		for _, libPath := range core.GetPaths("GOBJECT") {
			lib, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				continue
			}
			libs = append(libs, lib)
		}
		if len(libs) == 0 {
			return
		}

		core.PuregoSafeRegister(&typeCheckInstanceIsA, libs, "g_type_check_instance_is_a")
		core.PuregoSafeRegister(&typeNameFromInstance, libs, "g_type_name_from_instance")
	})
}
