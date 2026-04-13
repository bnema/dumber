package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
)

type schemeRegistrarStub struct {
	scheme  string
	options int32
	calls   int
}

func (s *schemeRegistrarStub) AddCustomScheme(schemeName string, options int32) int32 {
	s.calls++
	s.scheme = schemeName
	s.options = options
	return 1
}

func TestRegisterDumbScheme(t *testing.T) {
	t.Parallel()

	stub := &schemeRegistrarStub{}
	registerDumbScheme(stub)

	wantOptions := purecef.SchemeOptionsSchemeOptionStandard |
		purecef.SchemeOptionsSchemeOptionSecure |
		purecef.SchemeOptionsSchemeOptionCorsEnabled |
		purecef.SchemeOptionsSchemeOptionCspBypassing |
		purecef.SchemeOptionsSchemeOptionFetchEnabled

	if stub.calls != 1 {
		t.Fatalf("AddCustomScheme call count = %d, want 1", stub.calls)
	}
	if stub.scheme != dumbSchemeName {
		t.Fatalf("scheme = %q, want %q", stub.scheme, dumbSchemeName)
	}
	if stub.options != wantOptions {
		t.Fatalf("options = %d, want %d", stub.options, wantOptions)
	}
}
