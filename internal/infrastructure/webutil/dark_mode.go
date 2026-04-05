package webutil

import "fmt"

const darkModeScriptTemplate = `(function() {
  var prefersDark = %t;
  window.%s = prefersDark;
  var originalMatchMedia = window.matchMedia.bind(window);

  if (prefersDark) {
    document.documentElement.classList.add('dark');
    document.documentElement.classList.remove('light');
  } else {
    document.documentElement.classList.add('light');
    document.documentElement.classList.remove('dark');
  }

  function isColorSchemeQuery(query, scheme) {
    if (typeof query !== 'string') return false;
    var normalized = query.replace(/\s+/g, '').toLowerCase();
    return normalized.indexOf('prefers-color-scheme:' + scheme) !== -1;
  }

  function createFakeMediaQueryList(query, matches) {
    var listeners = [];
    var onchangeHandler = null;
    return {
      matches: matches,
      media: query,
      get onchange() { return onchangeHandler; },
      set onchange(fn) { onchangeHandler = fn; },
      addListener: function(cb) { if (typeof cb === 'function') listeners.push(cb); },
      removeListener: function(cb) { var idx = listeners.indexOf(cb); if (idx !== -1) listeners.splice(idx, 1); },
      addEventListener: function(type, cb) { if (type === 'change' && typeof cb === 'function') listeners.push(cb); },
      removeEventListener: function(type, cb) {
        if (type === 'change') { var idx = listeners.indexOf(cb); if (idx !== -1) listeners.splice(idx, 1); }
      },
      dispatchEvent: function(event) {
        for (var i = 0; i < listeners.length; i++) { try { listeners[i](event); } catch (e) {} }
        if (onchangeHandler) { try { onchangeHandler(event); } catch (e) {} }
        return true;
      }
    };
  }

  window.matchMedia = function(query) {
    if (isColorSchemeQuery(query, 'dark')) return createFakeMediaQueryList(query, prefersDark);
    if (isColorSchemeQuery(query, 'light')) return createFakeMediaQueryList(query, !prefersDark);
    return originalMatchMedia(query);
  };
})();`

// DarkModeScript returns JS that patches matchMedia and sets dark/light
// classes on <html>. The globalVar parameter names the window property
// that stores the preference (engine-specific for debuggability).
func DarkModeScript(prefersDark bool, globalVar string) string {
	return fmt.Sprintf(darkModeScriptTemplate, prefersDark, globalVar)
}
