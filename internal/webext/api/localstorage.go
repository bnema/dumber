package api

import (
	"context"
	"database/sql"
	"log"
	"sync"

	"github.com/bnema/dumber/internal/db"
)

// LocalStorageBackend defines the interface for persistent localStorage
type LocalStorageBackend interface {
	GetItem(key string) (string, bool)
	SetItem(key, value string) error
	RemoveItem(key string) error
	Clear() error
	Keys() []string
	Length() int
}

const storageTypeLocalStorage = "localStorage"

// SQLiteLocalStorage implements LocalStorageBackend using SQLite
type SQLiteLocalStorage struct {
	queries     *db.Queries
	extensionID string
	mu          sync.RWMutex
}

// NewSQLiteLocalStorage creates a new SQLite-backed localStorage
func NewSQLiteLocalStorage(dbConn *sql.DB, extensionID string) *SQLiteLocalStorage {
	return &SQLiteLocalStorage{
		queries:     db.New(dbConn),
		extensionID: extensionID,
	}
}

// GetItem retrieves a value from localStorage
func (s *SQLiteLocalStorage) GetItem(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, err := s.queries.GetExtensionStorageItem(context.Background(), s.extensionID, storageTypeLocalStorage, key)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		log.Printf("[localStorage] GetItem error for %s/%s: %v", s.extensionID, key, err)
		return "", false
	}

	return value, true
}

// SetItem stores a value in localStorage
func (s *SQLiteLocalStorage) SetItem(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[localStorage] SetItem for %s: key=%s, len(value)=%d", s.extensionID, key, len(value))

	err := s.queries.UpsertExtensionStorageItem(context.Background(), s.extensionID, storageTypeLocalStorage, key, value)
	if err != nil {
		log.Printf("[localStorage] SetItem error for %s/%s: %v", s.extensionID, key, err)
		return err
	}

	return nil
}

// RemoveItem removes a value from localStorage
func (s *SQLiteLocalStorage) RemoveItem(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.queries.DeleteExtensionStorageItem(context.Background(), s.extensionID, storageTypeLocalStorage, key)
	if err != nil {
		log.Printf("[localStorage] RemoveItem error for %s/%s: %v", s.extensionID, key, err)
		return err
	}

	return nil
}

// Clear removes all localStorage items for this extension
func (s *SQLiteLocalStorage) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.queries.ClearExtensionStorage(context.Background(), s.extensionID, storageTypeLocalStorage)
	if err != nil {
		log.Printf("[localStorage] Clear error for %s: %v", s.extensionID, err)
		return err
	}

	return nil
}

// Keys returns all localStorage keys for this extension
func (s *SQLiteLocalStorage) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys, err := s.queries.GetExtensionStorageKeys(context.Background(), s.extensionID, storageTypeLocalStorage)
	if err != nil {
		log.Printf("[localStorage] Keys error for %s: %v", s.extensionID, err)
		return nil
	}

	return keys
}

// Length returns the number of localStorage items for this extension
func (s *SQLiteLocalStorage) Length() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count, err := s.queries.CountExtensionStorageItems(context.Background(), s.extensionID, storageTypeLocalStorage)
	if err != nil {
		log.Printf("[localStorage] Length error for %s: %v", s.extensionID, err)
		return 0
	}

	return int(count)
}
