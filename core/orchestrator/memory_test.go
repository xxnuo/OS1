package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryStoreCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	store, err := NewMemoryStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Create(MemoryCreate{
		Kind:       "preference",
		Content:    "likes warm organic visuals",
		Tags:       []string{"visual", "style"},
		Confidence: 0.9,
		Source:     "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" {
		t.Fatal("missing id")
	}
	if store.Count(false) != 1 {
		t.Fatal("expected count 1")
	}
	newContent := "likes warm organic visuals and particles"
	updated, err := store.Update(item.ID, MemoryPatch{Content: &newContent})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != newContent {
		t.Fatal("content not updated")
	}
	_, err = store.Lock(item.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(item.ID, MemoryPatch{Content: &newContent})
	if err == nil {
		t.Fatal("expected locked update error")
	}
	_, err = store.Delete(item.ID)
	if err == nil {
		t.Fatal("expected locked delete error")
	}
	_, err = store.Lock(item.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Delete(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if store.Count(false) != 0 {
		t.Fatal("expected count 0 after delete")
	}
	if store.Count(true) != 1 {
		t.Fatal("expected total count 1")
	}
}

func TestMemoryStoreEncryptionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	store, err := NewMemoryStore(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(MemoryCreate{
		Kind:    "fact",
		Content: "user works on OS1",
		Source:  "system",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("missing file")
	}
	reloaded, err := NewMemoryStore(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Count(false) != 1 {
		t.Fatal("expected count 1")
	}
	_, err = NewMemoryStore(path, "")
	if err == nil {
		t.Fatal("expected error when key missing")
	}
}
