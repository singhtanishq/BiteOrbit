// Package storage is the media object-store provider interface with a local
// filesystem adapter (see ADR 003). Files get generated object keys; uploads
// are validated (type allowlist + size cap) by the API layer, not here.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store is the provider interface. Production swaps in MinIO/S3.
type Store interface {
	Put(key string, r io.Reader, contentType string) error
	Open(key string) (io.ReadCloser, string, error) // content, mime, error
	Delete(key string) error
	Exists(key string) bool
}

// Filesystem keeps objects under a root directory, rejecting any key that
// escapes it (path traversal protection).
type Filesystem struct {
	Root string
}

func NewFilesystem(root string) (*Filesystem, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Filesystem{Root: root}, nil
}

func (f *Filesystem) path(key string) (string, error) {
	clean := filepath.Clean(key)
	if strings.Contains(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid object key")
	}
	return filepath.Join(f.Root, clean), nil
}

func (f *Filesystem) Put(key string, r io.Reader, contentType string) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	dst, err := os.Create(p)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, r)
	return err
}

func (f *Filesystem) Open(key string) (io.ReadCloser, string, error) {
	p, err := f.path(key)
	if err != nil {
		return nil, "", err
	}
	file, err := os.Open(p)
	if err != nil {
		return nil, "", err
	}
	ext := filepath.Ext(p)
	return file, mime.TypeByExtension(ext), nil
}

func (f *Filesystem) Delete(key string) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

func (f *Filesystem) Exists(key string) bool {
	p, err := f.path(key)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// NewObjectKey builds a safe, content-derived key: prefix/YYYY/MM/rand.ext.
func NewObjectKey(prefix, originalName string) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalName))
	if len(ext) > 10 {
		return "", fmt.Errorf("extension too long")
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	return fmt.Sprintf("%s/%04d/%02d/%s%s",
		prefix, now.Year(), int(now.Month()), hex.EncodeToString(rnd[:]), ext), nil
}
