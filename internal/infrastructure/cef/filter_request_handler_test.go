package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/infrastructure/filtering/ceffilter"
)

type fakeFilterBackend struct {
	active bool
	block  bool
	seen   ceffilter.Request
}

func (b *fakeFilterBackend) HasActive() bool { return b.active }

func (b *fakeFilterBackend) ShouldBlock(req ceffilter.Request) bool {
	b.seen = req
	return b.block
}

type fakeCEFRequest struct {
	url          string
	resourceType purecef.ResourceType
}

func (r fakeCEFRequest) IsReadOnly() bool                                             { return true }
func (r fakeCEFRequest) GetURL() string                                               { return r.url }
func (r fakeCEFRequest) SetURL(string)                                                {}
func (r fakeCEFRequest) GetMethod() string                                            { return "GET" }
func (r fakeCEFRequest) SetMethod(string)                                             {}
func (r fakeCEFRequest) SetReferrer(string, purecef.ReferrerPolicy)                   {}
func (r fakeCEFRequest) GetReferrerURL() string                                       { return "" }
func (r fakeCEFRequest) GetReferrerPolicy() purecef.ReferrerPolicy                    { return 0 }
func (r fakeCEFRequest) GetPostData() purecef.PostData                                { return nil }
func (r fakeCEFRequest) SetPostData(purecef.PostData)                                 {}
func (r fakeCEFRequest) GetHeaderMap(purecef.StringMultimap)                          {}
func (r fakeCEFRequest) SetHeaderMap(purecef.StringMultimap)                          {}
func (r fakeCEFRequest) GetHeaderByName(string) string                                { return "" }
func (r fakeCEFRequest) SetHeaderByName(string, string, int32)                        {}
func (r fakeCEFRequest) Set(string, string, purecef.PostData, purecef.StringMultimap) {}
func (r fakeCEFRequest) GetFlags() int32                                              { return 0 }
func (r fakeCEFRequest) SetFlags(int32)                                               {}
func (r fakeCEFRequest) GetFirstPartyForCookies() string                              { return "" }
func (r fakeCEFRequest) SetFirstPartyForCookies(string)                               {}
func (r fakeCEFRequest) GetResourceType() purecef.ResourceType {
	return r.resourceType
}
func (r fakeCEFRequest) GetTransitionType() purecef.TransitionType { return 0 }
func (r fakeCEFRequest) GetIdentifier() uint64                     { return 1 }

func TestFilterResourceRequestHandlerCancelsBlockedRequests(t *testing.T) {
	backend := &fakeFilterBackend{active: true, block: true}
	handler := &filterResourceRequestHandler{
		backend:          backend,
		requestInitiator: "https://page.example",
	}

	result := handler.OnBeforeResourceLoad(nil, nil, fakeCEFRequest{
		url:          "https://ads.example/banner.png",
		resourceType: purecef.ResourceTypeRtImage,
	}, nil)

	assert.Equal(t, purecef.ReturnValueRvCancel, result)
	assert.Equal(t, "https://ads.example/banner.png", backend.seen.URL)
	assert.Equal(t, ceffilter.ResourceTypeImage, backend.seen.ResourceType)
	assert.Equal(t, "https://page.example", backend.seen.RequestInitiator)
}

func TestFilterResourceRequestHandlerContinuesAllowedRequests(t *testing.T) {
	backend := &fakeFilterBackend{active: true, block: false}
	handler := &filterResourceRequestHandler{backend: backend}

	result := handler.OnBeforeResourceLoad(nil, nil, fakeCEFRequest{
		url:          "https://example.test/app.js",
		resourceType: purecef.ResourceTypeRtScript,
	}, nil)

	assert.Equal(t, purecef.ReturnValueRvContinue, result)
	assert.Equal(t, ceffilter.ResourceTypeScript, backend.seen.ResourceType)
}

func TestFilterResourceRequestHandlerContinuesWhenInactive(t *testing.T) {
	backend := &fakeFilterBackend{active: false, block: true}
	handler := &filterResourceRequestHandler{backend: backend}

	result := handler.OnBeforeResourceLoad(nil, nil, fakeCEFRequest{
		url:          "https://ads.example/banner.png",
		resourceType: purecef.ResourceTypeRtImage,
	}, nil)

	assert.Equal(t, purecef.ReturnValueRvContinue, result)
	assert.Empty(t, backend.seen.URL)
}

func TestMapCEFResourceType(t *testing.T) {
	assert.Equal(t, ceffilter.ResourceTypeDocument, mapCEFResourceType(purecef.ResourceTypeRtMainFrame))
	assert.Equal(t, ceffilter.ResourceTypeStyleSheet, mapCEFResourceType(purecef.ResourceTypeRtStylesheet))
	assert.Equal(t, ceffilter.ResourceTypeScript, mapCEFResourceType(purecef.ResourceTypeRtScript))
	assert.Equal(t, ceffilter.ResourceTypeScript, mapCEFResourceType(purecef.ResourceTypeRtWorker))
	assert.Equal(t, ceffilter.ResourceTypeScript, mapCEFResourceType(purecef.ResourceTypeRtSharedWorker))
	assert.Equal(t, ceffilter.ResourceTypeScript, mapCEFResourceType(purecef.ResourceTypeRtServiceWorker))
	assert.Equal(t, ceffilter.ResourceTypeImage, mapCEFResourceType(purecef.ResourceTypeRtFavicon))
	assert.Equal(t, ceffilter.ResourceTypeFont, mapCEFResourceType(purecef.ResourceTypeRtFontResource))
	assert.Equal(t, ceffilter.ResourceTypeMedia, mapCEFResourceType(purecef.ResourceTypeRtMedia))
	assert.Equal(t, ceffilter.ResourceTypeXHR, mapCEFResourceType(purecef.ResourceTypeRtXhr))
	assert.Equal(t, ceffilter.ResourceTypeXHR, mapCEFResourceType(purecef.ResourceTypeRtPing))
	assert.Equal(t, ceffilter.ResourceTypeXHR, mapCEFResourceType(purecef.ResourceTypeRtCspReport))
	assert.Equal(t, ceffilter.ResourceTypeXHR, mapCEFResourceType(purecef.ResourceTypeRtPrefetch))
	assert.Equal(t, ceffilter.ResourceTypeRaw, mapCEFResourceType(purecef.ResourceTypeRtPluginResource))
	assert.Equal(t, ceffilter.ResourceTypeRaw, mapCEFResourceType(purecef.ResourceTypeRtSubResource))
}
