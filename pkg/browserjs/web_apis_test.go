package browserjs

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/sobek"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebAPIsManager_XMLHttpRequest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"test","value":123}`))
		case "/text":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("Hello, World!"))
		case "/echo-headers":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(r.Header.Get("X-Custom-Header")))
		default:
			w.WriteHeader(404)
			w.Write([]byte("Not found"))
		}
	}))
	defer server.Close()

	vm := sobek.New()
	tasks := make(chan func(), 100)
	wm := NewWebAPIsManager(vm, tasks, server.Client(), time.Now(), server.URL, nil, "")
	require.NoError(t, wm.Install())

	// Process tasks in background
	go func() {
		for task := range tasks {
			task()
		}
	}()

	t.Run("basic GET request", func(t *testing.T) {
		vm.Set("serverURL", server.URL)
		_, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			var result = null;
			xhr.onload = function() {
				result = xhr.responseText;
			};
			xhr.open("GET", serverURL + "/text", false);
			xhr.send();
		`)
		require.NoError(t, err)

		val, err := vm.RunString(`result`)
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!", val.String())
	})

	t.Run("readyState changes", func(t *testing.T) {
		vm.Set("serverURL", server.URL)
		_, err := vm.RunString(`
			var states = [];
			var xhr = new XMLHttpRequest();
			states.push(xhr.readyState); // 0 - UNSENT
			xhr.onreadystatechange = function() {
				states.push(xhr.readyState);
			};
			xhr.open("GET", serverURL + "/text", false);
			xhr.send();
		`)
		require.NoError(t, err)

		// Check final readyState
		val, err := vm.RunString(`xhr.readyState`)
		require.NoError(t, err)
		assert.Equal(t, int64(4), val.ToInteger()) // DONE
	})

	t.Run("setRequestHeader", func(t *testing.T) {
		vm.Set("serverURL", server.URL)
		_, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			var result = null;
			xhr.onload = function() {
				result = xhr.responseText;
			};
			xhr.open("GET", serverURL + "/echo-headers", false);
			xhr.setRequestHeader("X-Custom-Header", "test-value");
			xhr.send();
		`)
		require.NoError(t, err)

		val, err := vm.RunString(`result`)
		require.NoError(t, err)
		assert.Equal(t, "test-value", val.String())
	})

	t.Run("getResponseHeader", func(t *testing.T) {
		vm.Set("serverURL", server.URL)
		_, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			xhr.open("GET", serverURL + "/text", false);
			xhr.send();
		`)
		require.NoError(t, err)

		val, err := vm.RunString(`xhr.getResponseHeader("content-type")`)
		require.NoError(t, err)
		assert.Contains(t, val.String(), "text/plain")
	})

	t.Run("status and statusText", func(t *testing.T) {
		vm.Set("serverURL", server.URL)
		_, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			xhr.open("GET", serverURL + "/text", false);
			xhr.send();
		`)
		require.NoError(t, err)

		statusVal, err := vm.RunString(`xhr.status`)
		require.NoError(t, err)
		assert.Equal(t, int64(200), statusVal.ToInteger())
	})

	t.Run("XHR constants", func(t *testing.T) {
		val, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			({ UNSENT: xhr.UNSENT, OPENED: xhr.OPENED, HEADERS_RECEIVED: xhr.HEADERS_RECEIVED, LOADING: xhr.LOADING, DONE: xhr.DONE });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.Equal(t, int64(0), obj.Get("UNSENT").ToInteger())
		assert.Equal(t, int64(1), obj.Get("OPENED").ToInteger())
		assert.Equal(t, int64(2), obj.Get("HEADERS_RECEIVED").ToInteger())
		assert.Equal(t, int64(3), obj.Get("LOADING").ToInteger())
		assert.Equal(t, int64(4), obj.Get("DONE").ToInteger())
	})

	t.Run("abort sets status to 0", func(t *testing.T) {
		_, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			xhr.open("GET", serverURL + "/text", true);
			xhr.abort();
		`)
		require.NoError(t, err)

		val, err := vm.RunString(`xhr.status`)
		require.NoError(t, err)
		assert.Equal(t, int64(0), val.ToInteger())
	})

	t.Run("onabort fires on abort", func(t *testing.T) {
		val, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			var abortCalled = false;
			xhr.onabort = function() {
				abortCalled = true;
			};
			xhr.open("GET", serverURL + "/text", true);
			xhr.abort();
			abortCalled;
		`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("upload object exists", func(t *testing.T) {
		val, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			xhr.upload !== null && xhr.upload !== undefined;
		`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("responseType accessor", func(t *testing.T) {
		_, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			xhr.responseType = "json";
		`)
		require.NoError(t, err)

		val, err := vm.RunString(`xhr.responseType`)
		require.NoError(t, err)
		assert.Equal(t, "json", val.String())
	})

	t.Run("timeout accessor", func(t *testing.T) {
		_, err := vm.RunString(`
			var xhr = new XMLHttpRequest();
			xhr.timeout = 5000;
		`)
		require.NoError(t, err)

		val, err := vm.RunString(`xhr.timeout`)
		require.NoError(t, err)
		assert.Equal(t, int64(5000), val.ToInteger())
	})
}

func TestWebAPIsManager_Storage(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	wm := NewWebAPIsManager(vm, tasks, nil, time.Now(), "", nil, "")
	require.NoError(t, wm.Install())

	t.Run("localStorage setItem and getItem", func(t *testing.T) {
		_, err := vm.RunString(`localStorage.setItem("key", "value")`)
		require.NoError(t, err)

		val, err := vm.RunString(`localStorage.getItem("key")`)
		require.NoError(t, err)
		assert.Equal(t, "value", val.String())
	})

	t.Run("localStorage removeItem", func(t *testing.T) {
		_, err := vm.RunString(`
			localStorage.setItem("toRemove", "value");
			localStorage.removeItem("toRemove");
		`)
		require.NoError(t, err)

		val, err := vm.RunString(`localStorage.getItem("toRemove")`)
		require.NoError(t, err)
		assert.True(t, val == sobek.Null())
	})

	t.Run("localStorage clear", func(t *testing.T) {
		_, err := vm.RunString(`
			localStorage.setItem("key1", "value1");
			localStorage.setItem("key2", "value2");
			localStorage.clear();
		`)
		require.NoError(t, err)

		val, err := vm.RunString(`localStorage.length`)
		require.NoError(t, err)
		assert.Equal(t, int64(0), val.ToInteger())
	})

	t.Run("sessionStorage exists", func(t *testing.T) {
		val, err := vm.RunString(`typeof sessionStorage.setItem`)
		require.NoError(t, err)
		assert.Equal(t, "function", val.String())
	})
}

func TestWebAPIsManager_StructuredClone(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	wm := NewWebAPIsManager(vm, tasks, nil, time.Now(), "", nil, "")
	require.NoError(t, wm.Install())

	t.Run("clones simple object", func(t *testing.T) {
		val, err := vm.RunString(`
			var obj = { a: 1, b: "hello", c: [1, 2, 3] };
			var cloned = structuredClone(obj);
			cloned.a === obj.a && cloned.b === obj.b && cloned.c[0] === obj.c[0];
		`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("clone is independent copy", func(t *testing.T) {
		val, err := vm.RunString(`
			var obj = { value: 1 };
			var cloned = structuredClone(obj);
			cloned.value = 2;
			obj.value;  // Should still be 1
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val.ToInteger())
	})
}

func TestWebAPIsManager_DOMException(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	wm := NewWebAPIsManager(vm, tasks, nil, time.Now(), "", nil, "")
	require.NoError(t, wm.Install())

	t.Run("creates DOMException", func(t *testing.T) {
		val, err := vm.RunString(`
			var exc = new DOMException("test message", "AbortError");
			({ message: exc.message, name: exc.name });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.Equal(t, "test message", obj.Get("message").String())
		assert.Equal(t, "AbortError", obj.Get("name").String())
	})
}

func TestWebAPIsManager_MessageChannel(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	wm := NewWebAPIsManager(vm, tasks, nil, time.Now(), "", nil, "")
	require.NoError(t, wm.Install())

	t.Run("creates MessageChannel with two ports", func(t *testing.T) {
		val, err := vm.RunString(`
			var channel = new MessageChannel();
			channel.port1 !== null && channel.port2 !== null;
		`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("postMessage between ports", func(t *testing.T) {
		val, err := vm.RunString(`
			var channel = new MessageChannel();
			var received = null;
			channel.port2.onmessage = function(e) {
				received = e.data;
			};
			channel.port1.postMessage("hello");
			received;
		`)
		require.NoError(t, err)
		assert.Equal(t, "hello", val.String())
	})
}

func TestWebAPIsManager_Intl(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	wm := NewWebAPIsManager(vm, tasks, nil, time.Now(), "", nil, "")
	require.NoError(t, wm.Install())

	t.Run("Intl.Collator compare", func(t *testing.T) {
		val, err := vm.RunString(`
			var collator = new Intl.Collator();
			collator.compare("a", "b");
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(-1), val.ToInteger())
	})

	t.Run("Intl.NumberFormat exists", func(t *testing.T) {
		val, err := vm.RunString(`
			var formatter = new Intl.NumberFormat();
			typeof formatter.format;
		`)
		require.NoError(t, err)
		assert.Equal(t, "function", val.String())
	})
}

func TestWebAPIsManager_MiscAPIs(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	wm := NewWebAPIsManager(vm, tasks, nil, time.Now(), "https://example.com", nil, "")
	require.NoError(t, wm.Install())

	t.Run("matchMedia returns object", func(t *testing.T) {
		val, err := vm.RunString(`
			var mq = matchMedia("(min-width: 600px)");
			mq.media;
		`)
		require.NoError(t, err)
		assert.Equal(t, "(min-width: 600px)", val.String())
	})

	t.Run("isSecureContext is true", func(t *testing.T) {
		val, err := vm.RunString(`isSecureContext`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("origin from config", func(t *testing.T) {
		val, err := vm.RunString(`origin`)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", val.String())
	})

	t.Run("screen dimensions", func(t *testing.T) {
		val, err := vm.RunString(`
			({ width: screen.width, height: screen.height });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.Equal(t, int64(1920), obj.Get("width").ToInteger())
		assert.Equal(t, int64(1080), obj.Get("height").ToInteger())
	})
}
