package mainloop

import "sync"

// Coalescer merges bursts of same-key main-loop tasks.
type Coalescer struct {
	mu        sync.Mutex
	pending   map[string]bool
	callbacks map[string]func()
	post      func(func())
	destroyed bool
}

func NewCoalescer(post func(func())) *Coalescer {
	if post == nil {
		panic("mainloop.NewCoalescer: post function cannot be nil")
	}

	return &Coalescer{
		pending:   make(map[string]bool),
		callbacks: make(map[string]func()),
		post:      post,
	}
}

func (c *Coalescer) Post(key string, fn func()) {
	if fn == nil || key == "" {
		return
	}

	c.mu.Lock()
	if c.destroyed {
		c.mu.Unlock()
		return
	}
	c.callbacks[key] = fn
	if c.pending[key] {
		c.mu.Unlock()
		return
	}
	c.pending[key] = true
	post := c.post
	c.mu.Unlock()

	post(func() {
		c.mu.Lock()
		if c.destroyed {
			delete(c.pending, key)
			delete(c.callbacks, key)
			c.mu.Unlock()
			return
		}
		fn := c.callbacks[key]
		delete(c.pending, key)
		delete(c.callbacks, key)
		c.mu.Unlock()

		if fn != nil {
			fn()
		}
	})
}

func (c *Coalescer) Destroy() {
	c.mu.Lock()
	c.destroyed = true
	c.pending = map[string]bool{}
	c.callbacks = map[string]func(){}
	c.mu.Unlock()
}
