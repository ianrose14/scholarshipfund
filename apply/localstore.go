package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// localStorage implements FileStorage on the local filesystem, for development
// and tests.
type localStorage struct {
	base string
	mu   sync.Mutex
}

func (l *localStorage) path(p string) string { return filepath.Join(l.base, filepath.FromSlash(p)) }

func rev(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:8])
}

func (l *localStorage) Get(_ context.Context, p string) ([]byte, string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := os.ReadFile(l.path(p))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", errNotFound
	}
	return data, rev(data), err
}

func (l *localStorage) Put(_ context.Context, p string, data []byte, ifRev string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if ifRev != "" {
		cur, err := os.ReadFile(l.path(p))
		if err != nil || rev(cur) != ifRev {
			return errConflict
		}
	}
	return l.write(p, data)
}

func (l *localStorage) Create(_ context.Context, p string, data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := os.Stat(l.path(p)); err == nil {
		return errConflict
	}
	return l.write(p, data)
}

func (l *localStorage) write(p string, data []byte) error {
	full := l.path(p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func (l *localStorage) Delete(_ context.Context, p string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	err := os.Remove(l.path(p))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (l *localStorage) ListFolders(_ context.Context, dir string) ([]string, error) {
	entries, err := os.ReadDir(l.path(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, err
}
