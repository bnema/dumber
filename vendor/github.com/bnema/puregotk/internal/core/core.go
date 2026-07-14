// package core implements core functionality for the generated files
// this core lib is imported by the generated code
package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/bnema/purego"
)

// LibraryOpener opens a shared library path and returns its handle.
type LibraryOpener func(path string) (uintptr, error)

// LibraryPathResolver returns the candidate paths for a named library.
type LibraryPathResolver func(library string) ([]string, error)

// SymbolResolver resolves and publishes a symbol into target using library handles.
type SymbolResolver func(target any, libraries []uintptr, symbol string) error

// LazyResolver safely caches library opens and symbol publication. It is safe for
// concurrent use. Its injected dependencies make resolution deterministic in tests.
type LazyResolver struct {
	paths   LibraryPathResolver
	opener  LibraryOpener
	resolve SymbolResolver

	mu        sync.Mutex
	libraries map[string]*lazyLibrary
	symbols   map[lazySymbolKey]*lazySymbol
}

type lazyLibrary struct {
	once    sync.Once
	handles []uintptr
	err     error
}

type lazySymbolKey struct {
	library string
	symbol  string
	target  uintptr
}

type lazySymbol struct {
	once sync.Once
	err  error
}

// NewLazyResolver constructs an isolated lazy resolver. All dependencies must
// be non-nil.
func NewLazyResolver(paths LibraryPathResolver, opener LibraryOpener, resolve SymbolResolver) *LazyResolver {
	if paths == nil || opener == nil || resolve == nil {
		panic("core: lazy resolver dependencies must not be nil")
	}
	return &LazyResolver{
		paths:     paths,
		opener:    opener,
		resolve:   resolve,
		libraries: make(map[string]*lazyLibrary),
		symbols:   make(map[lazySymbolKey]*lazySymbol),
	}
}

// Register resolves and publishes symbol into target once. Library and symbol
// failures are cached as well as successful resolutions.
func (r *LazyResolver) Register(target any, library, symbol string) error {
	if r == nil {
		return fmt.Errorf("core: nil lazy resolver")
	}
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Ptr || value.IsNil() {
		return fmt.Errorf("core: lazy symbol target must be a non-nil pointer")
	}
	key := lazySymbolKey{library: library, symbol: symbol, target: value.Pointer()}

	r.mu.Lock()
	state := r.symbols[key]
	if state == nil {
		// Generated binding tests and embedders may provide a function before
		// the first call. Do not replace that explicit implementation with a
		// symbol. Once a resolution has started, use its sync.Once rather than
		// inspecting a function that the resolver may be publishing.
		if value.Elem().Kind() == reflect.Func && !value.Elem().IsNil() {
			r.mu.Unlock()
			return nil
		}
		state = &lazySymbol{}
		r.symbols[key] = state
	}
	r.mu.Unlock()

	state.once.Do(func() {
		libraries, err := r.open(library)
		if err != nil {
			state.err = err
			return
		}
		state.err = r.resolve(target, libraries, symbol)
	})
	return state.err
}

// RegisterOptional is Register for optional libraries. It reports whether the
// symbol was published and caches an unavailable-library failure.
func (r *LazyResolver) RegisterOptional(target any, library, symbol string) bool {
	return r.Register(target, library, symbol) == nil
}

func (r *LazyResolver) open(name string) ([]uintptr, error) {
	r.mu.Lock()
	state := r.libraries[name]
	if state == nil {
		state = &lazyLibrary{}
		r.libraries[name] = state
	}
	r.mu.Unlock()

	state.once.Do(func() {
		paths, err := r.paths(name)
		if err != nil {
			state.err = err
			return
		}
		if len(paths) == 0 {
			state.err = fmt.Errorf("core: no paths found for library %s", name)
			return
		}
		var lastErr error
		for _, path := range paths {
			handle, err := r.opener(path)
			if err != nil {
				lastErr = err
				continue
			}
			state.handles = append(state.handles, handle)
		}
		if len(state.handles) == 0 {
			state.err = fmt.Errorf("core: open library %s: %w", name, lastErr)
		}
	})
	return state.handles, state.err
}

var defaultLazyResolver = NewLazyResolver(
	func(name string) ([]string, error) {
		paths := tryFindPaths(name)
		if len(paths) == 0 {
			return nil, fmt.Errorf("core: no paths found for library %s", name)
		}
		return paths, nil
	},
	func(path string) (uintptr, error) {
		return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	},
	func(target any, libraries []uintptr, symbol string) error {
		var lastErr error
		for _, library := range libraries {
			address, err := purego.Dlsym(library, symbol)
			if err != nil {
				lastErr = err
				continue
			}
			registerFuncSafe(target, address)
			return nil
		}
		return fmt.Errorf("core: resolve symbol %s: %w", symbol, lastErr)
	},
)

// LazyRegister publishes a generated function's native symbol on its first
// call. Required-library failures panic; optional-library failures return false.
func LazyRegister(target any, library, symbol string, optional bool) bool {
	if err := defaultLazyResolver.Register(target, library, symbol); err != nil {
		if optional {
			return false
		}
		panic(err)
	}
	return true
}

// LibraryAvailable reports whether an optional library can be opened. The open
// result, including failure, is shared with LazyRegister.
func LibraryAvailable(library string) bool {
	_, err := defaultLazyResolver.open(library)
	return err == nil
}

func PuregoSafeRegister(fptr interface{}, libs []uintptr, name string) {
	for _, lib := range libs {
		sym, err := purego.Dlsym(lib, name)
		if err == nil {
			registerFuncSafe(fptr, sym)
			return
		}
	}
}

// registerFuncSafe wraps purego.RegisterFunc with panic recovery for signatures
// that exceed purego's current ABI support. The function pointer stays nil and
// only fails if the unsupported function is actually called.
func registerFuncSafe(fptr interface{}, sym uintptr) {
	defer func() { recover() }()
	purego.RegisterFunc(fptr, sym)
}

// paths to where the shared object files should be located
// this is unique per architecture
// Debian/Ubuntu has it split into specific arch folder, Fedora is just /usr/lib64
// Flatpak uses /app/lib for application libraries and runtimes don't vendor `pkg-config` as the fallback
// see:
// https://fedora.pkgs.org/38/fedora-x86_64/gtk4-4.10.1-1.fc38.x86_64.rpm.html
// https://fedora.pkgs.org/38/fedora-aarch64/gtk4-4.10.1-1.fc38.aarch64.rpm.html
// https://ubuntu.pkgs.org/23.04/ubuntu-main-amd64/libgtk-4-1_4.10.1+ds-2ubuntu1_amd64.deb.html
// https://ubuntu.pkgs.org/23.04/ubuntu-main-arm64/libgtk-4-1_4.10.1+ds-2ubuntu1_arm64.deb.html
// https://docs.flatpak.org/en/latest/flatpak-builder-command-reference.html (see --libdir)
var paths = map[string][]string{
	"linux_amd64":  {"/app/lib/", "/usr/lib/x86_64-linux-gnu/", "/usr/lib64/", "/usr/lib/"},
	"linux_arm64":  {"/app/lib/", "/usr/lib/aarch64-linux-gnu/", "/usr/lib64/", "/usr/lib/"},
	"darwin_arm64": {"/opt/homebrew/lib/", "/opt/local/lib/"},
	"darwin_amd64": {"/usr/local/lib/", "/opt/local/lib/"},
}

// names is a lookup from library names to shared object filenames
// This is populated dynamically via SetSharedLibrary
var (
	libraryConfigMu sync.RWMutex
	names           = map[string][]string{}
)

// pkgConfNames is a lookup from library names to pkg-config library names
// This is populated dynamically via SetPackageName
var pkgConfNames = map[string]string{}

// SetPackageName registers a pkg-config package name for a library.
// This is used by the code generator to set package names from GIR files.
// It won't override existing entries to preserve defaults.
func SetPackageName(libName, pkgName string) {
	libraryConfigMu.Lock()
	defer libraryConfigMu.Unlock()
	if _, exists := pkgConfNames[libName]; !exists && pkgName != "" {
		pkgConfNames[libName] = pkgName
	}
}

// SetSharedLibraries registers shared library names for a library.
// This is used by the code generator to set library names from GIR files.
// It won't override existing entries to preserve defaults.
func SetSharedLibraries(libName string, sharedLibs []string) {
	libraryConfigMu.Lock()
	defer libraryConfigMu.Unlock()
	if _, exists := names[libName]; !exists && len(sharedLibs) > 0 {
		names[libName] = append([]string(nil), sharedLibs...)
	}
}

// findSos tries to find all shared objects from a path and a library name
// It does this by mapping the library name to all suitable shared object filenames and then trying some suffixes
func findSos(path string, name string) []string {
	libraryConfigMu.RLock()
	libraryNames := append([]string(nil), names[name]...)
	libraryConfigMu.RUnlock()

	sos := []string{}
	for _, n := range libraryNames {
		suffixes := []string{"", ".0", ".1", ".2"}
		fn := filepath.Join(path, n)
		for _, s := range suffixes {
			if _, err := os.Stat(fn + s); err == nil {
				sos = append(sos, fn+s)
			}
		}
	}
	return sos
}

// findPkgConf finds all shared object files with pkg-config
// it does this by running pkg-config --libs-only-L libname
// and then it loops over the directories returned and finds all suitable ones
func findPkgConf(name string) []string {
	libraryConfigMu.RLock()
	pkgName := pkgConfNames[name]
	libraryConfigMu.RUnlock()
	cmd := exec.Command("pkg-config", "--libs-only-L", pkgName)
	var out, outerr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &outerr
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pkg-config, failed with: %v and stderr: %s\n", err, outerr.String())
		return []string{}
	}
	outs := strings.Split(out.String(), "-L")
	for _, v := range outs {
		c := strings.TrimSpace(v)
		if c == "" {
			continue
		}
		g := findSos(c, name)
		if len(g) > 0 {
			return g
		}
	}
	return []string{}
}

// tryFindPaths gets all shared object files from a library name.
// It does it in the following order:
// see if PUREGOTK_LIBNAME_PATH is set (full path to the lib)
// - e.g. PUREGOTK_GTK_PATH
// see if PUREGOTK_LIB_FOLDER is set (root folder where to look for libs)
// go over the hardcoded paths
// find a library name with pkg-config
func tryFindPaths(name string) []string {
	// try to get from env var
	ev := fmt.Sprintf("PUREGOTK_%s_PATH", name)
	if v := os.Getenv(ev); v != "" {
		return []string{v}
	}

	// Or if a general folder is set where everywhere is located, return that.
	ep := os.Getenv("PUREGOTK_LIB_FOLDER")
	if ep != "" {
		return findSos(ep, name)
	}

	// fallback to lookup a path if no env var is found
	gp, ok := paths[runtime.GOOS+"_"+runtime.GOARCH]
	if ok {
		for _, p := range gp {
			g := findSos(p, name)
			if len(g) > 0 {
				return g
			}
		}
	}

	return findPkgConf(name)
}

// GetPaths gets all shared object files from a library name and panics if no
// matching library can be found.
// TODO: Hardcode a library shared object with linker -X flag.
// This is useful for packaging.
func GetPaths(name string) []string {
	g := tryFindPaths(name)
	if len(g) > 0 {
		return g
	}

	ev := fmt.Sprintf("PUREGOTK_%s_PATH", name)
	panic(fmt.Sprintf("Path for library: %s not found. Please set the path to this library shared object file manually with env variable: %s or PUREGOTK_LIB_FOLDER. Or make sure pkg-config is setup correctly", strings.ToLower(name), ev))
}

// TryGetPaths is like GetPaths but returns an empty slice instead of panicking.
// Use this for optional libraries that may not be installed.
func TryGetPaths(name string) []string {
	return tryFindPaths(name)
}

// hasSuffix tests whether the string s ends with suffix.
// This function was copied from purego
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// ByteSlice creates a pointer to a byte slice of C strings
// This function was copied from purego
func ByteSlice(name []string) **byte {
	if name == nil {
		return nil
	}
	res := make([]*byte, len(name)+1)
	for i, v := range name {
		res[i] = CString(v)
	}

	// the last element is NULL terminated for GTK
	res[len(name)] = nil
	return &res[0]
}

// CString converts a go string to *byte that can be passed to C code.
// This function was copied from purego
func CString(name string) *byte {
	if hasSuffix(name, "\x00") {
		return &(*(*[]byte)(unsafe.Pointer(&name)))[0]
	}
	b := make([]byte, len(name)+1)
	copy(b, name)
	return &b[0]
}

// GoStringSlice gets a string slice from a char** array
// This function was copied from purego
func GoStringSlice(c uintptr) []string {
	var ret []string
	for i := 0; ; i++ {
		ptrAddr := c + uintptr(i)*unsafe.Sizeof(uintptr(0))
		addr := *(*unsafe.Pointer)(unsafe.Pointer(&ptrAddr))
		// We take the address and then dereference it to trick go vet from creating a possible misuse of unsafe.Pointer
		ptr := *(*uintptr)(addr)
		if ptr == 0 {
			break
		}
		ret = append(ret, GoString(ptr))
	}

	return ret
}

// GoString copies a char* to a Go string.
// This function was copied from purego
func GoString(c uintptr) string {
	// We take the address and then dereference it to trick go vet from creating a possible misuse of unsafe.Pointer
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&c))
	if ptr == nil {
		return ""
	}
	var length int
	for {
		if *(*byte)(unsafe.Add(ptr, uintptr(length))) == '\x00' {
			break
		}
		length++
	}
	return string(unsafe.Slice((*byte)(ptr), length))
}

var (
	xGStrdup    func(string) uintptr
	gstrdupOnce sync.Once
	xGFree      func(uintptr)
	gfreeOnce   sync.Once
)

// GStrdup allocates a C-owned copy of a Go string using g_strdup.
// The returned pointer must be freed with g_free by the receiver or callee.
func GStrdup(s string) uintptr {
	gstrdupOnce.Do(func() {
		var libs []uintptr
		for _, libPath := range GetPaths("GLIB") {
			lib, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				continue
			}
			libs = append(libs, lib)
		}
		PuregoSafeRegister(&xGStrdup, libs, "g_strdup")
	})
	return xGStrdup(s)
}

// GStrdupNullable is like GStrdup but accepts a nullable *string.
func GStrdupNullable(s *string) uintptr {
	if s == nil {
		return 0
	}
	return GStrdup(*s)
}

// GFree frees memory allocated by GLib allocation APIs.
func GFree(ptr uintptr) {
	if ptr == 0 {
		return
	}
	gfreeOnce.Do(func() {
		var libs []uintptr
		for _, libPath := range GetPaths("GLIB") {
			lib, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				continue
			}
			libs = append(libs, lib)
		}
		PuregoSafeRegister(&xGFree, libs, "g_free")
	})
	xGFree(ptr)
}

// GFreeNullable frees a nullable GLib-allocated pointer.
func GFreeNullable(ptr uintptr) {
	if ptr == 0 {
		return
	}
	GFree(ptr)
}

// NullableStringToPtr converts a nullable Go string to a C string pointer.
// The caller must call runtime.KeepAlive(backing) after the C call completes.
func NullableStringToPtr(s *string) (uintptr, []byte) {
	if s == nil {
		return 0, nil
	}
	b := append([]byte(*s), 0)
	return uintptr(unsafe.Pointer(&b[0])), b
}

// PtrToNullableString converts a nullable char* to a Go *string.
func PtrToNullableString(ptr uintptr) *string {
	if ptr == 0 {
		return nil
	}
	str := GoString(ptr)
	return &str
}
