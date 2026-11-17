package systemviews

import "sync"

// runConcurrently calls fn(0..n-1) each in its own goroutine and returns the
// errors by index. In wasm each bridge call posts its message before it
// blocks on the reply, so starting them together posts every request while
// the pagehide handler yields, instead of only the first one.
func runConcurrently(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { errs[i] = fn(i) })
	}
	wg.Wait()
	return errs
}
