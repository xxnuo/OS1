package orchestrator

import (
	"testing"
	"time"

	"os1/core/platform"
)

func TestRuntimeIngestUserSignalRawAndBatch(t *testing.T) {
	bus := NewBus()
	_, sub, unsub := bus.Subscribe(64)
	defer unsub()
	r := NewRuntime(testASR{}, testLLM{}, testTTS{wait: time.Millisecond}, bus, nil, platform.NewStub(), nil)
	r.signalWindow = 30 * time.Millisecond
	if err := r.IngestUserSignal("s-signal", UserSignal{
		Type: WindowSignalType,
		Window: &WindowSignal{
			App:         "Finder",
			WindowTitle: "Desktop",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.IngestUserSignal("s-signal", UserSignal{
		Type: FileSignalType,
		File: &FileSignal{
			Path:      "/tmp/a.txt",
			EventType: "write",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.IngestUserSignal("s-signal", UserSignal{
		Type: ClipboardSignalType,
		Clipboard: &ClipboardSignal{
			Timestamp: time.Now(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	events := collect(sub, 4, time.Second)
	if len(events) < 4 {
		t.Fatalf("events too short: %d", len(events))
	}
	if events[0].Type != WindowChangedEventType {
		t.Fatal("missing window changed event")
	}
	if events[1].Type != FileChangedEventType {
		t.Fatal("missing file changed event")
	}
	if events[2].Type != ClipboardChangedEventType {
		t.Fatal("missing clipboard changed event")
	}
	if events[3].Type != UserSignalBatchEventType {
		t.Fatal("missing signal batch event")
	}
	batch, ok := events[3].Payload.(UserSignalBatch)
	if !ok {
		t.Fatal("invalid batch payload")
	}
	if batch.SignalCount != 3 {
		t.Fatalf("unexpected signal count: %d", batch.SignalCount)
	}
	if batch.Window == nil || batch.Window.App != "Finder" {
		t.Fatal("window payload mismatch")
	}
	if !batch.ClipboardChanged {
		t.Fatal("clipboard flag should be true")
	}
	if len(batch.FileChanges) != 1 {
		t.Fatal("expected one file change in batch")
	}
}

func TestRuntimeIngestUserSignalValidation(t *testing.T) {
	r := NewRuntime(testASR{}, testLLM{}, testTTS{wait: time.Millisecond}, NewBus(), nil, platform.NewStub(), nil)
	if err := r.IngestUserSignal("s", UserSignal{Type: WindowSignalType}); err == nil {
		t.Fatal("expected validation error for missing window payload")
	}
	if err := r.IngestUserSignal("s", UserSignal{Type: UserSignalType("x")}); err == nil {
		t.Fatal("expected unsupported signal type error")
	}
}
