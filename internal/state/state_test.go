package state

import (
	"path/filepath"
	"testing"
)

func TestFileStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	fs, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	if got := fs.Get("missing"); got.Status != "" {
		t.Errorf("Get(missing) = %+v, want zero", got)
	}

	want := ProductState{Status: "available", LastChangeUnix: 100, LastNotifyUnix: 100}
	if err := fs.Set("slug1", want); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Reload from disk to confirm persistence.
	fs2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	if got := fs2.Get("slug1"); got != want {
		t.Errorf("after reload Get(slug1) = %+v, want %+v", got, want)
	}
}

func TestNewFileStoreMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	fs, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	if err := fs.Set("a", ProductState{Status: "sold_out"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
}
