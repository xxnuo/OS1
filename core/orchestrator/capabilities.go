package orchestrator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type CapabilitySpec struct {
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
	SideEffects  []string        `json:"sideEffects"`
	Source       string          `json:"source"`
	Signature    string          `json:"signature"`
}

type Capability struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
	SideEffects  []string        `json:"sideEffects"`
	Source       string          `json:"source"`
	Signature    string          `json:"signature"`
	AddedAt      time.Time       `json:"addedAt"`
}

type CapabilityRegistry struct {
	mu    sync.RWMutex
	items map[string]Capability
}

func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{
		items: map[string]Capability{},
	}
}

func CapabilityID(name string, version string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "@" + strings.TrimSpace(version)
}

func SignCapability(spec CapabilitySpec) (string, error) {
	canonical, err := canonicalizeSpec(spec)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"name":         canonical.Name,
		"version":      canonical.Version,
		"inputSchema":  json.RawMessage(canonical.InputSchema),
		"outputSchema": json.RawMessage(canonical.OutputSchema),
		"sideEffects":  canonical.SideEffects,
		"source":       canonical.Source,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func VerifyCapabilitySignature(spec CapabilitySpec) (bool, error) {
	canonical, err := canonicalizeSpec(spec)
	if err != nil {
		return false, err
	}
	signature := strings.TrimSpace(canonical.Signature)
	if signature == "" {
		return false, errors.New("missing signature")
	}
	expected, err := SignCapability(canonical)
	if err != nil {
		return false, err
	}
	if signature != expected {
		return false, errors.New("invalid signature")
	}
	return true, nil
}

func (r *CapabilityRegistry) Register(spec CapabilitySpec) (Capability, error) {
	canonical, err := canonicalizeSpec(spec)
	if err != nil {
		return Capability{}, err
	}
	ok, err := VerifyCapabilitySignature(canonical)
	if !ok {
		return Capability{}, err
	}
	id := CapabilityID(canonical.Name, canonical.Version)
	capability := Capability{
		ID:           id,
		Name:         canonical.Name,
		Version:      canonical.Version,
		InputSchema:  canonical.InputSchema,
		OutputSchema: canonical.OutputSchema,
		SideEffects:  canonical.SideEffects,
		Source:       canonical.Source,
		Signature:    canonical.Signature,
		AddedAt:      time.Now().UTC(),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[id]; exists {
		return Capability{}, fmt.Errorf("capability exists: %s", id)
	}
	r.items[id] = capability
	return capability, nil
}

func (r *CapabilityRegistry) RegisterBuiltin(spec CapabilitySpec) (Capability, error) {
	canonical, err := canonicalizeSpec(spec)
	if err != nil {
		return Capability{}, err
	}
	signature, err := SignCapability(canonical)
	if err != nil {
		return Capability{}, err
	}
	canonical.Signature = signature
	return r.Register(canonical)
}

func (r *CapabilityRegistry) Unregister(id string) (Capability, error) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return Capability{}, errors.New("empty capability id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.items[key]
	if !ok {
		return Capability{}, fmt.Errorf("capability not found: %s", key)
	}
	delete(r.items, key)
	return current, nil
}

func (r *CapabilityRegistry) List() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (r *CapabilityRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

func canonicalizeSpec(spec CapabilitySpec) (CapabilitySpec, error) {
	out := spec
	out.Name = strings.TrimSpace(out.Name)
	out.Version = strings.TrimSpace(out.Version)
	out.Source = strings.TrimSpace(out.Source)
	out.Signature = strings.TrimSpace(out.Signature)
	if out.Name == "" {
		return CapabilitySpec{}, errors.New("empty capability name")
	}
	if !semverPattern.MatchString(out.Version) {
		return CapabilitySpec{}, fmt.Errorf("invalid semver version: %s", out.Version)
	}
	if out.Source == "" {
		return CapabilitySpec{}, errors.New("empty source")
	}
	inputSchema, err := normalizeJSON(out.InputSchema)
	if err != nil {
		return CapabilitySpec{}, fmt.Errorf("invalid input schema: %w", err)
	}
	outputSchema, err := normalizeJSON(out.OutputSchema)
	if err != nil {
		return CapabilitySpec{}, fmt.Errorf("invalid output schema: %w", err)
	}
	out.InputSchema = inputSchema
	out.OutputSchema = outputSchema
	out.SideEffects = normalizeSideEffects(out.SideEffects)
	return out, nil
}

func normalizeJSON(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return json.RawMessage("{}"), nil
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func normalizeSideEffects(input []string) []string {
	set := map[string]struct{}{}
	for _, item := range input {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func BuiltinCapabilitySpecs() []CapabilitySpec {
	return []CapabilitySpec{
		{
			Name:         "notify",
			Version:      "1.0.0",
			InputSchema:  json.RawMessage(`{"type":"object","required":["title","body"],"properties":{"title":{"type":"string"},"body":{"type":"string"}}}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"delivered":{"type":"boolean"}}}`),
			SideEffects:  []string{"show_system_notification"},
			Source:       "builtin",
		},
		{
			Name:         "screenshot",
			Version:      "1.0.0",
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"region":{"type":"string"}}}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"sampled":{"type":"boolean"}}}`),
			SideEffects:  []string{"capture_screen"},
			Source:       "builtin",
		},
		{
			Name:         "audio_io",
			Version:      "1.0.0",
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"mode":{"type":"string","enum":["capture","playback"]}}}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
			SideEffects:  []string{"microphone_capture", "speaker_playback"},
			Source:       "builtin",
		},
		{
			Name:         "window_focus",
			Version:      "1.0.0",
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"app":{"type":"string"},"windowTitle":{"type":"string"}}}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"foregroundApp":{"type":"string"},"windowTitle":{"type":"string"}}}`),
			SideEffects:  []string{"switch_foreground_window"},
			Source:       "builtin",
		},
		{
			Name:         "file_io",
			Version:      "1.0.0",
			InputSchema:  json.RawMessage(`{"type":"object","required":["op","path"],"properties":{"op":{"type":"string","enum":["read","write","create","move","delete"]},"path":{"type":"string"}}}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"},"bytes":{"type":"integer"}}}`),
			SideEffects:  []string{"filesystem_read", "filesystem_write"},
			Source:       "builtin",
		},
		{
			Name:         "clipboard",
			Version:      "1.0.0",
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"op":{"type":"string","enum":["read","write"]},"text":{"type":"string"}}}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
			SideEffects:  []string{"clipboard_read", "clipboard_write"},
			Source:       "builtin",
		},
	}
}
