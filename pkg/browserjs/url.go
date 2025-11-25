package browserjs

import "github.com/grafana/sobek"

// URLManager provides URL and URLSearchParams APIs.
type URLManager struct {
	vm *sobek.Runtime
}

// NewURLManager creates a new URL manager.
func NewURLManager(vm *sobek.Runtime) *URLManager {
	return &URLManager{vm: vm}
}

// Install registers URL and URLSearchParams on the VM.
func (um *URLManager) Install() error {
	// Use JavaScript implementation for URL since it's complex
	_, err := um.vm.RunString(urlJS)
	return err
}

// urlJS contains the JavaScript implementation of URL and URLSearchParams.
const urlJS = `
(function() {

	function URL(url, base) {
		if (base) {
			if (!url.match(/^[a-z]+:/i)) {
				if (url.startsWith('//')) {
					url = base.match(/^[a-z]+:/i)[0] + url;
				} else if (url.startsWith('/')) {
					var m = base.match(/^([a-z]+:\/\/[^\/]+)/i);
					url = (m ? m[1] : '') + url;
				} else {
					url = base.replace(/[^\/]*$/, '') + url;
				}
			}
		}

		// Handle special schemes without authority (about:, data:, javascript:, blob:)
		var specialMatch = url.match(/^(about|data|javascript|blob):(.*)$/i);
		if (specialMatch) {
			this.protocol = specialMatch[1].toLowerCase() + ':';
			this.username = '';
			this.password = '';
			this.hostname = '';
			this.port = '';
			this.pathname = specialMatch[2] || '';
			this.search = '';
			this.hash = '';
			this.host = '';
			this.origin = 'null';  // Special schemes have null origin
			this.href = url;
			this.searchParams = new URLSearchParams('');
			return;
		}

		var match = url.match(/^([a-z]+:)\/\/(?:([^:@]+)(?::([^@]*))?@)?([^:\/]+)(?::(\d+))?(\/[^?#]*)?(\?[^#]*)?(#.*)?$/i);
		if (!match) {
			throw new TypeError("Invalid URL: " + url);
		}

		this.protocol = match[1] || '';
		this.username = match[2] || '';
		this.password = match[3] || '';
		this.hostname = match[4] || '';
		this.port = match[5] || '';
		this.pathname = match[6] || '/';
		this.search = match[7] || '';
		this.hash = match[8] || '';
		this.host = this.hostname + (this.port ? ':' + this.port : '');
		this.origin = this.protocol + '//' + this.host;
		this.href = url;
		this.searchParams = new URLSearchParams(this.search);
	}

	URL.prototype.toString = function() { return this.href; };
	URL.prototype.toJSON = function() { return this.href; };

	URL.createObjectURL = function(blob) {
		return 'blob:' + Math.random().toString(36).substr(2, 9);
	};

	URL.revokeObjectURL = function(url) {};

	function URLSearchParams(init) {
		this._params = [];
		if (typeof init === 'string') {
			init = init.replace(/^\?/, '');
			var pairs = init.split('&');
			for (var i = 0; i < pairs.length; i++) {
				var pair = pairs[i].split('=');
				if (pair[0]) {
					this._params.push([decodeURIComponent(pair[0]), decodeURIComponent(pair[1] || '')]);
				}
			}
		} else if (init && typeof init === 'object') {
			if (Array.isArray(init)) {
				for (var j = 0; j < init.length; j++) {
					this._params.push([String(init[j][0]), String(init[j][1])]);
				}
			} else {
				for (var key in init) {
					this._params.push([key, String(init[key])]);
				}
			}
		}
	}

	URLSearchParams.prototype.append = function(name, value) {
		this._params.push([String(name), String(value)]);
	};

	URLSearchParams.prototype.delete = function(name) {
		this._params = this._params.filter(function(p) { return p[0] !== String(name); });
	};

	URLSearchParams.prototype.get = function(name) {
		for (var i = 0; i < this._params.length; i++) {
			if (this._params[i][0] === String(name)) return this._params[i][1];
		}
		return null;
	};

	URLSearchParams.prototype.getAll = function(name) {
		return this._params.filter(function(p) { return p[0] === String(name); }).map(function(p) { return p[1]; });
	};

	URLSearchParams.prototype.has = function(name) {
		return this._params.some(function(p) { return p[0] === String(name); });
	};

	URLSearchParams.prototype.set = function(name, value) {
		this.delete(name);
		this.append(name, value);
	};

	URLSearchParams.prototype.sort = function() {
		this._params.sort(function(a, b) { return a[0].localeCompare(b[0]); });
	};

	URLSearchParams.prototype.toString = function() {
		return this._params.map(function(p) {
			return encodeURIComponent(p[0]) + '=' + encodeURIComponent(p[1]);
		}).join('&');
	};

	URLSearchParams.prototype.forEach = function(callback, thisArg) {
		for (var i = 0; i < this._params.length; i++) {
			callback.call(thisArg, this._params[i][1], this._params[i][0], this);
		}
	};

	URLSearchParams.prototype.keys = function() {
		var index = 0;
		var params = this._params;
		return {
			next: function() {
				if (index < params.length) {
					return { value: params[index++][0], done: false };
				}
				return { done: true };
			},
			[Symbol.iterator]: function() { return this; }
		};
	};

	URLSearchParams.prototype.values = function() {
		var index = 0;
		var params = this._params;
		return {
			next: function() {
				if (index < params.length) {
					return { value: params[index++][1], done: false };
				}
				return { done: true };
			},
			[Symbol.iterator]: function() { return this; }
		};
	};

	URLSearchParams.prototype.entries = function() {
		var index = 0;
		var params = this._params;
		return {
			next: function() {
				if (index < params.length) {
					return { value: params[index++].slice(), done: false };
				}
				return { done: true };
			},
			[Symbol.iterator]: function() { return this; }
		};
	};

	URLSearchParams.prototype[Symbol.iterator] = URLSearchParams.prototype.entries;

	globalThis.URL = URL;
	globalThis.URLSearchParams = URLSearchParams;
})();
`
