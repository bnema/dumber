package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewConnectionConcurrentFirstOpen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")
	const openers = 8
	var wg sync.WaitGroup
	errors := make(chan error, openers)
	for range openers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := NewConnection(context.Background(), dbPath)
			if err == nil {
				err = db.Close()
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
