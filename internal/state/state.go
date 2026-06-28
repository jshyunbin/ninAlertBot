// Package state persists the last-known availability of each product so the
// service does not re-alert across restarts.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ProductState is the persisted record for one product.
type ProductState struct {
	// Status is the last-known availability: "available", "sold_out", or "".
	Status string `json:"status"`
	// LastChangeUnix is when Status last changed.
	LastChangeUnix int64 `json:"last_change_unix"`
	// LastNotifyUnix is when we last sent an availability alert.
	LastNotifyUnix int64 `json:"last_notify_unix"`
}

// Store is the persistence interface used by the monitor.
type Store interface {
	Get(slug string) ProductState
	Set(slug string, st ProductState) error
}

// FileStore is a JSON-file-backed Store, safe for concurrent use.
type FileStore struct {
	path string
	mu   sync.Mutex
	data map[string]ProductState
}

// NewFileStore loads existing state from path (an absent file is fine).
func NewFileStore(path string) (*FileStore, error) {
	fs := &FileStore{path: path, data: make(map[string]ProductState)}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fs, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &fs.data); err != nil {
			return nil, fmt.Errorf("parse state: %w", err)
		}
	}
	return fs, nil
}

// Get returns the stored state for slug, or a zero value if absent.
func (fs *FileStore) Get(slug string) ProductState {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.data[slug]
}

// Set stores state for slug and persists the whole store atomically.
func (fs *FileStore) Set(slug string, st ProductState) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.data[slug] = st
	return fs.flushLocked()
}

func (fs *FileStore) flushLocked() error {
	b, err := json.MarshalIndent(fs.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := fs.path + ".tmp"
	if dir := filepath.Dir(fs.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, fs.path)
}
