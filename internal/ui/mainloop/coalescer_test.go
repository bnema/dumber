package mainloop

import "testing"

func TestCoalescerMergesBurstIntoSingleIdle(t *testing.T) {
	queue := make([]func(), 0, 8)
	c := NewCoalescer(func(fn func()) { queue = append(queue, fn) })

	value := 0
	for i := 1; i <= 5; i++ {
		v := i
		c.Post("omnibox-search", func() { value = v })
	}

	if len(queue) != 1 {
		t.Fatalf("expected 1 scheduled callback, got %d", len(queue))
	}
	queue[0]()

	if value != 5 {
		t.Fatalf("expected latest callback to run, got %d", value)
	}
}

func TestCoalescerDropsWorkAfterDestroy(t *testing.T) {
	queue := make([]func(), 0, 4)
	c := NewCoalescer(func(fn func()) { queue = append(queue, fn) })

	ran := false
	c.Post("ghost-clear", func() { ran = true })
	c.Destroy()

	if len(queue) != 1 {
		t.Fatalf("expected one queued callback before destroy, got %d", len(queue))
	}
	queue[0]()

	if ran {
		t.Fatalf("expected queued work to be dropped after destroy")
	}

	c.Post("ghost-clear", func() { ran = true })
	if len(queue) != 1 {
		t.Fatalf("expected no new callback after destroy, got %d", len(queue))
	}
}

func TestNewCoalescerPanicsOnNilPost(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected NewCoalescer to panic when post is nil")
		}
	}()

	_ = NewCoalescer(nil)
}
