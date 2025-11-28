package debugger

import (
	"fmt"
	"strings"

	"github.com/grafana/sobek"
)

// FunctionalTests defines all functional tests to run.
var FunctionalTests = []TestCase{
	// Storage tests
	{
		Name: "storage.local.set",
		Code: `browser.storage.local.set({_dbg: "test"})`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			return err == nil, ""
		},
		Cleanup: `browser.storage.local.remove("_dbg")`,
	},
	{
		Name: "storage.local.get",
		Code: `browser.storage.local.get(["_dbg"])`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			return true, "retrieved key"
		},
	},

	// Runtime tests
	{
		Name: "runtime.getManifest",
		Code: `browser.runtime.getManifest()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) || sobek.IsNull(v) {
				return false, "returned nil"
			}
			obj := v.ToObject(vm)
			name := obj.Get("name")
			if name == nil || sobek.IsUndefined(name) {
				return false, "no name field"
			}
			return true, fmt.Sprintf("name=%s", name.String())
		},
	},
	{
		Name: "runtime.getURL",
		Code: `browser.runtime.getURL("test.js")`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			url := v.String()
			if !strings.Contains(url, "test.js") {
				return false, "invalid URL"
			}
			return true, url
		},
	},
	{
		Name: "runtime.getBrowserInfo",
		Code: `browser.runtime.getBrowserInfo()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) || sobek.IsNull(v) {
				return false, "returned nil"
			}
			obj := v.ToObject(vm)
			name := obj.Get("name")
			if name == nil || sobek.IsUndefined(name) {
				return false, "no name field"
			}
			return true, fmt.Sprintf("name=%s", name.String())
		},
	},
	{
		Name: "runtime.getPlatformInfo",
		Code: `browser.runtime.getPlatformInfo()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) || sobek.IsNull(v) {
				return false, "returned nil"
			}
			obj := v.ToObject(vm)
			os := obj.Get("os")
			arch := obj.Get("arch")
			if os == nil || sobek.IsUndefined(os) {
				return false, "no os field"
			}
			return true, fmt.Sprintf("os=%s, arch=%s", os.String(), arch.String())
		},
	},

	// Tabs tests
	{
		Name: "tabs.query",
		Code: `browser.tabs.query({})`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			return true, "returned array"
		},
	},

	// i18n tests
	{
		Name: "i18n.getUILanguage",
		Code: `browser.i18n.getUILanguage()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			lang := v.String()
			if lang == "" {
				return false, "empty language"
			}
			return true, lang
		},
	},

	// Web API tests
	{
		Name: "TextEncoder/Decoder",
		Code: `new TextDecoder().decode(new TextEncoder().encode("test"))`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			if v.String() != "test" {
				return false, "encoding mismatch"
			}
			return true, "roundtrip OK"
		},
	},
	{
		Name: "URL parsing",
		Code: `new URL("https://example.com/path?q=1").hostname`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			if v.String() != "example.com" {
				return false, "wrong hostname"
			}
			return true, "example.com"
		},
	},
	{
		Name: "atob/btoa",
		Code: `atob(btoa("hello"))`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			if v.String() != "hello" {
				return false, "encoding mismatch"
			}
			return true, "roundtrip OK"
		},
	},
	{
		Name: "DOMParser",
		Code: `new DOMParser().parseFromString("<div>test</div>", "text/html").body.textContent`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			if !strings.Contains(v.String(), "test") {
				return false, "parse failed"
			}
			return true, "parsed OK"
		},
	},
	{
		Name: "Blob",
		Code: `new Blob(["test"]).size`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			size := v.ToInteger()
			if size != 4 {
				return false, fmt.Sprintf("wrong size: %d", size)
			}
			return true, "size=4"
		},
	},
	{
		Name: "FormData",
		Code: `(function() { var fd = new FormData(); fd.append("key", "value"); return fd.get("key"); })()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			if v.String() != "value" {
				return false, "wrong value"
			}
			return true, "get OK"
		},
	},
	{
		Name: "AbortController",
		Code: `(function() { var ac = new AbortController(); return ac.signal.aborted; })()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			if v.ToBoolean() != false {
				return false, "should not be aborted"
			}
			return true, "signal OK"
		},
	},
	{
		Name: "structuredClone",
		Code: `JSON.stringify(structuredClone({a: 1, b: [2, 3]}))`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			expected := `{"a":1,"b":[2,3]}`
			if v.String() != expected {
				return false, "clone mismatch"
			}
			return true, "clone OK"
		},
	},
	{
		Name: "performance.now",
		Code: `typeof performance.now()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			if v.String() != "number" {
				return false, "not a number"
			}
			return true, "returns number"
		},
	},
	{
		Name: "crypto.getRandomValues",
		Code: `(function() { var arr = new Uint8Array(4); crypto.getRandomValues(arr); return arr.length; })()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			if v == nil || sobek.IsUndefined(v) {
				return false, "returned nil"
			}
			if v.ToInteger() != 4 {
				return false, "wrong length"
			}
			return true, "filled OK"
		},
	},
	{
		Name: "console.log",
		Code: `(function() { console.log("debugger test"); return true; })()`,
		Check: func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) {
			if err != nil {
				return false, err.Error()
			}
			return true, "logged"
		},
	},
}
