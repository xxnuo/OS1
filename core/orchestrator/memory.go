package orchestrator

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryItem struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Content    string    `json:"content"`
	Tags       []string  `json:"tags,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Locked     bool      `json:"locked"`
	Deleted    bool      `json:"deleted"`
}

type MemoryCreate struct {
	Kind       string   `json:"kind"`
	Content    string   `json:"content"`
	Tags       []string `json:"tags,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Source     string   `json:"source"`
}

type MemoryPatch struct {
	Content    *string   `json:"content,omitempty"`
	Tags       *[]string `json:"tags,omitempty"`
	Confidence *float64  `json:"confidence,omitempty"`
	Source     *string   `json:"source,omitempty"`
}

type memoryFile struct {
	Version   int          `json:"version"`
	Encrypted bool         `json:"encrypted"`
	Nonce     string       `json:"nonce,omitempty"`
	Data      string       `json:"data,omitempty"`
	Items     []MemoryItem `json:"items,omitempty"`
}

type MemoryStore struct {
	mu        sync.Mutex
	path      string
	key       []byte
	encrypt   bool
	items     map[string]MemoryItem
	updatedAt time.Time
}

func NewMemoryStore(path string, passphrase string) (*MemoryStore, error) {
	store := &MemoryStore{
		path:    path,
		items:   map[string]MemoryItem{},
		encrypt: strings.TrimSpace(passphrase) != "",
	}
	if strings.TrimSpace(passphrase) != "" {
		sum := sha256.Sum256([]byte(passphrase))
		store.key = sum[:]
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *MemoryStore) Create(input MemoryCreate) (MemoryItem, error) {
	kind := strings.TrimSpace(input.Kind)
	content := strings.TrimSpace(input.Content)
	source := strings.TrimSpace(input.Source)
	if kind == "" {
		return MemoryItem{}, errors.New("empty kind")
	}
	if content == "" {
		return MemoryItem{}, errors.New("empty content")
	}
	if source == "" {
		return MemoryItem{}, errors.New("empty source")
	}
	now := time.Now().UTC()
	item := MemoryItem{
		ID:         newMemoryID(),
		Kind:       kind,
		Content:    content,
		Tags:       normalizeTags(input.Tags),
		Confidence: clamp01(input.Confidence),
		Source:     source,
		CreatedAt:  now,
		UpdatedAt:  now,
		Locked:     false,
		Deleted:    false,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[item.ID] = item
	s.updatedAt = now
	if err := s.saveLocked(); err != nil {
		delete(s.items, item.ID)
		return MemoryItem{}, err
	}
	return item, nil
}

func (s *MemoryStore) Update(id string, patch MemoryPatch) (MemoryItem, error) {
	key := strings.TrimSpace(id)
	if key == "" {
		return MemoryItem{}, errors.New("empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.items[key]
	if !ok {
		return MemoryItem{}, errors.New("not found")
	}
	if current.Locked {
		return MemoryItem{}, errors.New("locked")
	}
	if current.Deleted {
		return MemoryItem{}, errors.New("deleted")
	}
	if patch.Content != nil {
		value := strings.TrimSpace(*patch.Content)
		if value == "" {
			return MemoryItem{}, errors.New("empty content")
		}
		current.Content = value
	}
	if patch.Tags != nil {
		current.Tags = normalizeTags(*patch.Tags)
	}
	if patch.Confidence != nil {
		current.Confidence = clamp01(*patch.Confidence)
	}
	if patch.Source != nil {
		value := strings.TrimSpace(*patch.Source)
		if value == "" {
			return MemoryItem{}, errors.New("empty source")
		}
		current.Source = value
	}
	current.UpdatedAt = time.Now().UTC()
	s.items[key] = current
	s.updatedAt = current.UpdatedAt
	if err := s.saveLocked(); err != nil {
		return MemoryItem{}, err
	}
	return current, nil
}

func (s *MemoryStore) Delete(id string) (MemoryItem, error) {
	key := strings.TrimSpace(id)
	if key == "" {
		return MemoryItem{}, errors.New("empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.items[key]
	if !ok {
		return MemoryItem{}, errors.New("not found")
	}
	if current.Locked {
		return MemoryItem{}, errors.New("locked")
	}
	if current.Deleted {
		return current, nil
	}
	current.Deleted = true
	current.UpdatedAt = time.Now().UTC()
	s.items[key] = current
	s.updatedAt = current.UpdatedAt
	if err := s.saveLocked(); err != nil {
		return MemoryItem{}, err
	}
	return current, nil
}

func (s *MemoryStore) Lock(id string, locked bool) (MemoryItem, error) {
	key := strings.TrimSpace(id)
	if key == "" {
		return MemoryItem{}, errors.New("empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.items[key]
	if !ok {
		return MemoryItem{}, errors.New("not found")
	}
	current.Locked = locked
	current.UpdatedAt = time.Now().UTC()
	s.items[key] = current
	s.updatedAt = current.UpdatedAt
	if err := s.saveLocked(); err != nil {
		return MemoryItem{}, err
	}
	return current, nil
}

func (s *MemoryStore) List(includeDeleted bool) []MemoryItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]MemoryItem, 0, len(s.items))
	for _, item := range s.items {
		if !includeDeleted && item.Deleted {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *MemoryStore) Count(includeDeleted bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if includeDeleted {
		return len(s.items)
	}
	n := 0
	for _, item := range s.items {
		if !item.Deleted {
			n++
		}
	}
	return n
}

func (s *MemoryStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var file memoryFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return err
	}
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Encrypted {
		if len(s.key) == 0 {
			return errors.New("memory encrypted but key missing")
		}
		nonce, err := base64.RawStdEncoding.DecodeString(file.Nonce)
		if err != nil {
			return err
		}
		data, err := base64.RawStdEncoding.DecodeString(file.Data)
		if err != nil {
			return err
		}
		plain, err := decryptAESGCM(s.key, nonce, data)
		if err != nil {
			return err
		}
		var items []MemoryItem
		if err := json.Unmarshal(plain, &items); err != nil {
			return err
		}
		s.items = map[string]MemoryItem{}
		for _, item := range items {
			if item.ID == "" {
				continue
			}
			s.items[item.ID] = item
		}
		return nil
	}
	s.items = map[string]MemoryItem{}
	for _, item := range file.Items {
		if item.ID == "" {
			continue
		}
		s.items[item.ID] = item
	}
	return nil
}

func (s *MemoryStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	items := make([]MemoryItem, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	file := memoryFile{
		Version:   1,
		Encrypted: false,
		Items:     items,
	}
	if s.encrypt {
		if len(s.key) == 0 {
			return errors.New("encryption enabled but key missing")
		}
		plain, err := json.Marshal(items)
		if err != nil {
			return err
		}
		nonce := make([]byte, 12)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		ciphertext, err := encryptAESGCM(s.key, nonce, plain)
		if err != nil {
			return err
		}
		file = memoryFile{
			Version:   1,
			Encrypted: true,
			Nonce:     base64.RawStdEncoding.EncodeToString(nonce),
			Data:      base64.RawStdEncoding.EncodeToString(ciphertext),
		}
	}
	raw, err := json.Marshal(file)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func encryptAESGCM(key []byte, nonce []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size: %d", len(nonce))
	}
	return gcm.Seal(nil, nonce, plaintext, nil), nil
}

func decryptAESGCM(key []byte, nonce []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size: %d", len(nonce))
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func newMemoryID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "mem_" + hex.EncodeToString(b[:])
}

func normalizeTags(input []string) []string {
	set := map[string]struct{}{}
	for _, tag := range input {
		value := strings.TrimSpace(tag)
		if value == "" {
			continue
		}
		set[strings.ToLower(value)] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for tag := range set {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
