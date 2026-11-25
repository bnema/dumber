package webkit

import (
	"context"
	"net/http"
	"testing"

	"bnema.dev/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type stubHomeIconResolver map[string][]byte

func (s stubHomeIconResolver) ResolveHomeIconByID(_ context.Context, id string) ([]byte, string, error) {
	if data, ok := s[id]; ok {
		return data, "image/png", nil
	}
	return nil, "", port.ErrHomeIconNotFound
}

func newHomeIconTestHandler() *DumbSchemeHandler {
	h := NewDumbSchemeHandler(context.Background())
	h.SetHomeIconResolver(stubHomeIconResolver{"jellyfin": []byte("png")})
	return h
}

func homeIconReq(query string) *SchemeRequest {
	return &SchemeRequest{URI: "dumb://home/api/home-icon" + query, Path: homeIconAPIPath, Method: httpGET, Origin: "dumb://home"}
}

func TestWebKitHomeIconServesPNG(t *testing.T) {
	immediate, resolve := newHomeIconTestHandler().homeIconRequest(homeIconReq("?service=jellyfin"))
	require.Nil(t, immediate)
	require.NotNil(t, resolve)
	resp := resolve(context.Background())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "image/png", resp.ContentType)
	require.Equal(t, []byte("png"), resp.Data)
	require.Equal(t, "max-age=3600", resp.Headers["Cache-Control"])
}

func TestWebKitHomeIconUnknownServiceIs404(t *testing.T) {
	_, resolve := newHomeIconTestHandler().homeIconRequest(homeIconReq("?service=nope"))
	require.Equal(t, http.StatusNotFound, resolve(context.Background()).StatusCode)
}

func TestWebKitHomeIconRejectsBadID(t *testing.T) {
	h := newHomeIconTestHandler()
	for _, q := range []string{"", "?service=", "?service=../etc", "?service=UPPER"} {
		immediate, resolve := h.homeIconRequest(homeIconReq(q))
		require.Nil(t, resolve, q)
		require.Equal(t, http.StatusBadRequest, immediate.StatusCode, q)
	}
}

func TestWebKitHomeIconAppliesTrustGate(t *testing.T) {
	req := homeIconReq("?service=jellyfin")
	req.Origin = "https://evil.example"
	immediate, resolve := newHomeIconTestHandler().homeIconRequest(req)
	require.Nil(t, resolve)
	require.Equal(t, http.StatusForbidden, immediate.StatusCode)
}

func TestWebKitHomeIconUnavailableWithoutResolver(t *testing.T) {
	immediate, resolve := NewDumbSchemeHandler(context.Background()).homeIconRequest(homeIconReq("?service=jellyfin"))
	require.Nil(t, resolve)
	require.Equal(t, http.StatusNotFound, immediate.StatusCode)
}
