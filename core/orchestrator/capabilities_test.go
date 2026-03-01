package orchestrator

import (
	"encoding/json"
	"testing"
	"time"

	"os1/core/platform"
)

func TestCapabilityRegistryRegisterAndUnregister(t *testing.T) {
	registry := NewCapabilityRegistry()
	spec := CapabilitySpec{
		Name:         "tool.calendar",
		Version:      "1.2.3",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
		SideEffects:  []string{"calendar_read"},
		Source:       "runtime",
	}
	signature, err := SignCapability(spec)
	if err != nil {
		t.Fatal(err)
	}
	spec.Signature = signature
	capability, err := registry.Register(spec)
	if err != nil {
		t.Fatal(err)
	}
	if capability.ID != "tool.calendar@1.2.3" {
		t.Fatalf("unexpected capability id: %s", capability.ID)
	}
	if registry.Count() != 1 {
		t.Fatalf("unexpected capability count: %d", registry.Count())
	}
	removed, err := registry.Unregister(capability.ID)
	if err != nil {
		t.Fatal(err)
	}
	if removed.ID != capability.ID {
		t.Fatalf("unexpected removed capability id: %s", removed.ID)
	}
	if registry.Count() != 0 {
		t.Fatalf("unexpected capability count after unregister: %d", registry.Count())
	}
}

func TestCapabilityRegistryRejectsInvalidSignature(t *testing.T) {
	registry := NewCapabilityRegistry()
	spec := CapabilitySpec{
		Name:         "tool.mail",
		Version:      "1.0.0",
		InputSchema:  json.RawMessage(`{"type":"object"}`),
		OutputSchema: json.RawMessage(`{"type":"object"}`),
		SideEffects:  []string{"mail_send"},
		Source:       "runtime",
		Signature:    "sha256:deadbeef",
	}
	if _, err := registry.Register(spec); err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestRuntimeBuiltinCapabilities(t *testing.T) {
	runtime := NewRuntime(testASR{}, testLLM{}, testTTS{wait: time.Millisecond}, NewBus(), nil, platform.NewStub(), nil)
	capabilities := runtime.ListCapabilities()
	if len(capabilities) != 6 {
		t.Fatalf("unexpected builtin capability count: %d", len(capabilities))
	}
}

func TestRuntimeCapabilityEvents(t *testing.T) {
	bus := NewBus()
	_, sub, unsub := bus.Subscribe(32)
	defer unsub()
	runtime := NewRuntime(testASR{}, testLLM{}, testTTS{wait: time.Millisecond}, bus, nil, platform.NewStub(), nil)
	spec := CapabilitySpec{
		Name:         "tool.notes",
		Version:      "1.0.0",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"result":{"type":"string"}}}`),
		SideEffects:  []string{"notes_read"},
		Source:       "runtime",
	}
	signature, err := SignCapability(spec)
	if err != nil {
		t.Fatal(err)
	}
	spec.Signature = signature
	capability, err := runtime.RegisterCapability("s-cap", spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.UnregisterCapability("s-cap", capability.ID); err != nil {
		t.Fatal(err)
	}
	events := collect(sub, 2, time.Second)
	if len(events) != 2 {
		t.Fatalf("unexpected events length: %d", len(events))
	}
	if events[0].Type != CapabilityEventType || events[1].Type != CapabilityEventType {
		t.Fatal("expected capability events")
	}
	firstPayload, ok := events[0].Payload.(map[string]any)
	if !ok {
		t.Fatal("invalid payload type")
	}
	if firstPayload["action"] != "registered" {
		t.Fatal("first action should be registered")
	}
	secondPayload, ok := events[1].Payload.(map[string]any)
	if !ok {
		t.Fatal("invalid payload type")
	}
	if secondPayload["action"] != "unregistered" {
		t.Fatal("second action should be unregistered")
	}
}
