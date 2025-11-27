package browserjs

import (
	"testing"
	"time"

	"github.com/grafana/sobek"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDOMManager_AbortController(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	dm := NewDOMManager(vm, tasks)
	require.NoError(t, dm.InstallConstructors())

	t.Run("basic abort", func(t *testing.T) {
		val, err := vm.RunString(`
			var controller = new AbortController();
			var signal = controller.signal;
			var result = { aborted: signal.aborted };
			controller.abort();
			result.abortedAfter = signal.aborted;
			result;
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.False(t, obj.Get("aborted").ToBoolean())
		assert.True(t, obj.Get("abortedAfter").ToBoolean())
	})

	t.Run("abort with custom reason", func(t *testing.T) {
		val, err := vm.RunString(`
			var controller = new AbortController();
			controller.abort("custom reason");
			controller.signal.reason;
		`)
		require.NoError(t, err)
		assert.Equal(t, "custom reason", val.String())
	})

	t.Run("abort fires event listeners", func(t *testing.T) {
		val, err := vm.RunString(`
			var controller = new AbortController();
			var callCount = 0;
			var listener = function(event) {
				callCount++;
			};
			controller.signal.addEventListener("abort", listener);
			controller.abort();
			callCount;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val.ToInteger())
	})

	t.Run("removeEventListener removes listener", func(t *testing.T) {
		val, err := vm.RunString(`
			var controller = new AbortController();
			var callCount = 0;
			var listener = function(event) {
				callCount++;
			};
			controller.signal.addEventListener("abort", listener);
			controller.signal.removeEventListener("abort", listener);
			controller.abort();
			callCount;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(0), val.ToInteger())
	})

	t.Run("onabort handler fires", func(t *testing.T) {
		val, err := vm.RunString(`
			var controller = new AbortController();
			var called = false;
			controller.signal.onabort = function() {
				called = true;
			};
			controller.abort();
			called;
		`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("duplicate listeners ignored", func(t *testing.T) {
		val, err := vm.RunString(`
			var controller = new AbortController();
			var callCount = 0;
			var listener = function(event) {
				callCount++;
			};
			controller.signal.addEventListener("abort", listener);
			controller.signal.addEventListener("abort", listener); // duplicate
			controller.abort();
			callCount;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val.ToInteger())
	})

	t.Run("abort only fires once", func(t *testing.T) {
		val, err := vm.RunString(`
			var controller = new AbortController();
			var callCount = 0;
			controller.signal.addEventListener("abort", function() {
				callCount++;
			});
			controller.abort();
			controller.abort(); // second call should do nothing
			callCount;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val.ToInteger())
	})
}

func TestDOMManager_AbortSignalStatic(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	dm := NewDOMManager(vm, tasks)
	require.NoError(t, dm.InstallConstructors())

	t.Run("AbortSignal.abort() creates aborted signal", func(t *testing.T) {
		val, err := vm.RunString(`
			var signal = AbortSignal.abort();
			signal.aborted;
		`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("AbortSignal.abort() with custom reason", func(t *testing.T) {
		val, err := vm.RunString(`
			var signal = AbortSignal.abort("custom reason");
			signal.reason;
		`)
		require.NoError(t, err)
		assert.Equal(t, "custom reason", val.String())
	})

	t.Run("AbortSignal.abort() default reason is AbortError", func(t *testing.T) {
		val, err := vm.RunString(`
			var signal = AbortSignal.abort();
			signal.reason.name;
		`)
		require.NoError(t, err)
		assert.Equal(t, "AbortError", val.String())
	})

	t.Run("AbortSignal.timeout() creates signal", func(t *testing.T) {
		val, err := vm.RunString(`
			var signal = AbortSignal.timeout(100);
			signal.aborted;
		`)
		require.NoError(t, err)
		assert.False(t, val.ToBoolean())
	})

	t.Run("AbortSignal.timeout() aborts after timeout", func(t *testing.T) {
		vm2 := sobek.New()
		tasks2 := make(chan func(), 10)
		dm2 := NewDOMManager(vm2, tasks2)
		require.NoError(t, dm2.InstallConstructors())

		_, err := vm2.RunString(`
			var signal = AbortSignal.timeout(50);
			var abortedAt = null;
			signal.addEventListener("abort", function() {
				abortedAt = Date.now();
			});
		`)
		require.NoError(t, err)

		// Wait for timeout and process task
		time.Sleep(100 * time.Millisecond)
		select {
		case task := <-tasks2:
			task()
		case <-time.After(200 * time.Millisecond):
			// Task may have run synchronously
		}

		val, err := vm2.RunString(`signal.aborted`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("AbortSignal.any() creates signal with addEventListener", func(t *testing.T) {
		// AbortSignal.any() returns a signal that can have listeners added
		val, err := vm.RunString(`
			var combined = AbortSignal.any([]);
			typeof combined.addEventListener === 'function';
		`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})
}

func TestDOMManager_DOMParser(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	dm := NewDOMManager(vm, tasks)
	require.NoError(t, dm.InstallConstructors())

	t.Run("parse basic HTML", func(t *testing.T) {
		val, err := vm.RunString(`
			var parser = new DOMParser();
			var doc = parser.parseFromString('<html><body><div id="test">Hello</div></body></html>', 'text/html');
			doc.documentElement.tagName;
		`)
		require.NoError(t, err)
		assert.Equal(t, "HTML", val.String())
	})

	t.Run("getElementById", func(t *testing.T) {
		val, err := vm.RunString(`
			var parser = new DOMParser();
			var doc = parser.parseFromString('<div id="myDiv">Content</div>', 'text/html');
			var elem = doc.getElementById("myDiv");
			elem !== null ? elem.tagName : null;
		`)
		require.NoError(t, err)
		assert.Equal(t, "DIV", val.String())
	})

	t.Run("querySelector", func(t *testing.T) {
		val, err := vm.RunString(`
			var parser = new DOMParser();
			var doc = parser.parseFromString('<div class="test">Content</div>', 'text/html');
			var elem = doc.querySelector(".test");
			elem !== null ? elem.className : null;
		`)
		require.NoError(t, err)
		assert.Equal(t, "test", val.String())
	})

	t.Run("querySelectorAll", func(t *testing.T) {
		val, err := vm.RunString(`
			var parser = new DOMParser();
			var doc = parser.parseFromString('<div class="item">1</div><div class="item">2</div>', 'text/html');
			var elems = doc.querySelectorAll(".item");
			elems.length;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(2), val.ToInteger())
	})

	t.Run("getAttribute", func(t *testing.T) {
		val, err := vm.RunString(`
			var parser = new DOMParser();
			var doc = parser.parseFromString('<div id="test" class="myclass">Content</div>', 'text/html');
			var elem = doc.querySelector("div");
			elem.getAttribute("id");
		`)
		require.NoError(t, err)
		assert.Equal(t, "test", val.String())
	})

	t.Run("getElementsByTagName", func(t *testing.T) {
		val, err := vm.RunString(`
			var parser = new DOMParser();
			var doc = parser.parseFromString('<div><span>1</span><span>2</span></div>', 'text/html');
			doc.getElementsByTagName("span").length;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(2), val.ToInteger())
	})

	t.Run("createElement", func(t *testing.T) {
		val, err := vm.RunString(`
			var parser = new DOMParser();
			var doc = parser.parseFromString('<div></div>', 'text/html');
			var elem = doc.createElement("span");
			elem.tagName;
		`)
		require.NoError(t, err)
		assert.Equal(t, "SPAN", val.String())
	})
}

func TestDOMManager_Blob(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	dm := NewDOMManager(vm, tasks)
	require.NoError(t, dm.InstallConstructors())

	t.Run("create Blob with content", func(t *testing.T) {
		val, err := vm.RunString(`
			var blob = new Blob(["hello world"], { type: "text/plain" });
			({ size: blob.size, type: blob.type });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.Equal(t, int64(11), obj.Get("size").ToInteger())
		assert.Equal(t, "text/plain", obj.Get("type").String())
	})

	t.Run("Blob slice", func(t *testing.T) {
		val, err := vm.RunString(`
			var blob = new Blob(["hello world"]);
			var sliced = blob.slice(0, 5);
			sliced.size;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(5), val.ToInteger())
	})
}

func TestDOMManager_FormData(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)
	dm := NewDOMManager(vm, tasks)
	require.NoError(t, dm.InstallConstructors())

	t.Run("append and get", func(t *testing.T) {
		val, err := vm.RunString(`
			var fd = new FormData();
			fd.append("name", "value");
			fd.get("name");
		`)
		require.NoError(t, err)
		assert.Equal(t, "value", val.String())
	})

	t.Run("has", func(t *testing.T) {
		val, err := vm.RunString(`
			var fd = new FormData();
			fd.append("name", "value");
			fd.has("name");
		`)
		require.NoError(t, err)
		assert.True(t, val.ToBoolean())
	})

	t.Run("delete", func(t *testing.T) {
		val, err := vm.RunString(`
			var fd = new FormData();
			fd.append("name", "value");
			fd.delete("name");
			fd.has("name");
		`)
		require.NoError(t, err)
		assert.False(t, val.ToBoolean())
	})

	t.Run("getAll", func(t *testing.T) {
		val, err := vm.RunString(`
			var fd = new FormData();
			fd.append("name", "value1");
			fd.append("name", "value2");
			fd.getAll("name").length;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(2), val.ToInteger())
	})

	t.Run("set", func(t *testing.T) {
		val, err := vm.RunString(`
			var fd = new FormData();
			fd.append("name", "value1");
			fd.set("name", "value2");
			fd.getAll("name").length;
		`)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val.ToInteger())

		// Verify the value was replaced
		val2, err := vm.RunString(`fd.get("name")`)
		require.NoError(t, err)
		assert.Equal(t, "value2", val2.String())
	})
}
