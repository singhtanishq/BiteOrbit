package storage

import (
	"strings"
	"testing"
)

func TestObjectKeyTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFilesystem(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for _, key := range []string{"../escape.txt", "a/../../escape.txt", "/abs/path.txt"} {
		if err := fs.Put(key, strings.NewReader("x"), "text/plain"); err == nil {
			t.Fatalf("key %q must be rejected", key)
		}
	}
	// valid nested key works and stays inside root
	if err := fs.Put("2026/11/file.txt", strings.NewReader("hello"), "text/plain"); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	rc, mime, err := fs.Open("2026/11/file.txt")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rc.Close()
	if mime == "" {
		t.Fatal("mime type should be inferred from extension")
	}
}

func TestNewObjectKeyFormat(t *testing.T) {
	k1, err := NewObjectKey("menus", "photo.PNG")
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	if !strings.HasPrefix(k1, "menus/") || !strings.HasSuffix(k1, ".png") {
		t.Fatalf("unexpected key format: %s", k1)
	}
	k2, _ := NewObjectKey("menus", "photo.PNG")
	if k1 == k2 {
		t.Fatal("object keys must be unique per call")
	}
}
