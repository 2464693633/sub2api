package creativestudio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageLockExclusiveAndReusable(t *testing.T) {
	root := t.TempDir()
	first, err := New(root, unusedExecutor(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if duplicate, err := New(root, unusedExecutor(), Options{}); !errors.Is(err, ErrBusy) {
		if duplicate != nil {
			_ = duplicate.Close()
		}
		t.Fatalf("second store must fail with ErrBusy: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, unusedExecutor(), Options{})
	if err != nil {
		t.Fatalf("storage lock was not released on close: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
}

func TestStorageLockReleasedAfterLoadFailure(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "1", "broken")
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	projectFile := filepath.Join(projectDir, "project.json")
	if err := os.WriteFile(projectFile, []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	failed, err := New(root, unusedExecutor(), Options{})
	if failed != nil {
		_ = failed.Close()
		t.Fatal("store must not open a corrupt project")
	}
	if err == nil || errors.Is(err, ErrBusy) {
		t.Fatalf("expected a project load error, got %v", err)
	}
	if err := os.Remove(projectFile); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, unusedExecutor(), Options{})
	if err != nil {
		t.Fatalf("storage lock was not released after load failure: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
}
