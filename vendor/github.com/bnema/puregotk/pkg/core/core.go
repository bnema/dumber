package core

import "github.com/bnema/puregotk/internal/core"

type (
	LibraryOpener       = core.LibraryOpener
	LibraryPathResolver = core.LibraryPathResolver
	SymbolResolver      = core.SymbolResolver
	LazyResolver        = core.LazyResolver
)

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
	NewLazyResolver     = core.NewLazyResolver
	LazyRegister        = core.LazyRegister
	LibraryAvailable    = core.LibraryAvailable
)
