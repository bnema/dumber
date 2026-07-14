package core

import "github.com/bnema/puregotk/internal/core"

var (
	GetPaths            = core.GetPaths
	TryGetPaths         = core.TryGetPaths
	ByteSlice           = core.ByteSlice
	GoStringSlice       = core.GoStringSlice
	GoString            = core.GoString
	GStrdup             = core.GStrdup
	GStrdupNullable     = core.GStrdupNullable
	GFree               = core.GFree
	GFreeNullable       = core.GFreeNullable
	NullableStringToPtr = core.NullableStringToPtr
	PtrToNullableString = core.PtrToNullableString
	SetPackageName      = core.SetPackageName
	SetSharedLibraries  = core.SetSharedLibraries
	PuregoSafeRegister  = core.PuregoSafeRegister
)
