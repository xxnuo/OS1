package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"os1/core/platform"
)

type testASR struct{}

func (t testASR) Transcribe(ctx context.Context, sessionID string, segment []byte, meta SegmentMeta) (ASRResult, error) {
	return ASRResult{Text: "hello", Confidence: 0.98, LatencyMs: 10}, nil
}

type testLLM struct{}

func (t testLLM) Respond(ctx context.Context, sessionID string, userText string) (LLMDecision, error) {
	return LLMDecision{Intent: "chat", ReplyText: "hi there", Actions: nil}, nil
}

type testTTS struct {
	wait time.Duration
}

func (t testTTS) Synthesize(ctx context.Context, sessionID string, text string) (<-chan TTSChunk, error) {
	ch := make(chan TTSChunk, 2)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
			return
		case <-time.After(t.wait):
			ch <- TTSChunk{Chunk: "hi", IsFinal: false}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(t.wait):
			ch <- TTSChunk{Chunk: "done", IsFinal: true}
		}
	}()
	return ch, nil
}

func collect(ch <-chan Event, n int, timeout time.Duration) []Event {
	out := make([]Event, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case e := <-ch:
			out = append(out, e)
		case <-deadline:
			return out
		}
	}
	return out
}

func TestRuntimeFlow(t *testing.T) {
	bus := NewBus()
	_, sub, unsub := bus.Subscribe(64)
	defer unsub()
	audit, err := NewAuditWriter(t.TempDir() + "/audit.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	r := NewRuntime(testASR{}, testLLM{}, testTTS{wait: 2 * time.Millisecond}, bus, audit, platform.NewStub())
	if err := r.StartSession("s1"); err != nil {
		t.Fatal(err)
	}
	if err := r.HandleUserAudio("s1", []byte("x"), SegmentMeta{SampleRate: 16000, DurationMs: 300}); err != nil {
		t.Fatal(err)
	}
	events := collect(sub, 12, time.Second)
	if len(events) < 12 {
		t.Fatalf("events too short: %d", len(events))
	}
	hasAsr := false
	hasLlm := false
	hasFinal := false
	for _, e := range events {
		if e.Type == AsrResultEventType {
			hasAsr = true
		}
		if e.Type == LlmDecisionEventType {
			hasLlm = true
		}
		if e.Type == TtsChunkEventType {
			m := e.Payload.(TTSChunk)
			if m.IsFinal {
				hasFinal = true
			}
		}
	}
	if !hasAsr || !hasLlm || !hasFinal {
		t.Fatal("missing required events")
	}
}

func TestBargeInInterrupt(t *testing.T) {
	bus := NewBus()
	_, sub, unsub := bus.Subscribe(128)
	defer unsub()
	r := NewRuntime(testASR{}, testLLM{}, testTTS{wait: 200 * time.Millisecond}, bus, nil, platform.NewStub())
	if err := r.StartSession("s2"); err != nil {
		t.Fatal(err)
	}
	if err := r.HandleUserAudio("s2", []byte("a"), SegmentMeta{SampleRate: 16000, DurationMs: 200}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := r.HandleUserAudio("s2", []byte("b"), SegmentMeta{SampleRate: 16000, DurationMs: 180}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	mu := sync.Mutex{}
	count := 0
	for {
		select {
		case e := <-sub:
			if e.Type == TtsChunkEventType {
				mu.Lock()
				count++
				mu.Unlock()
			}
		case <-deadline:
			if count == 0 {
				t.Fatal("expected tts events")
			}
			return
		}
	}
}

func TestRuntimeSnapshot(t *testing.T) {
	bus := NewBus()
	r := NewRuntime(testASR{}, testLLM{}, testTTS{wait: time.Millisecond}, bus, nil, platform.NewStub())
	state := r.Snapshot()
	if state.Mute {
		t.Fatal("mute should default false")
	}
	if !state.HUDVisible {
		t.Fatal("hud should default true")
	}
	if err := r.SetMute(true); err != nil {
		t.Fatal(err)
	}
	visible, err := r.ToggleHUD()
	if err != nil {
		t.Fatal(err)
	}
	if visible {
		t.Fatal("hud should be false after first toggle")
	}
	state = r.Snapshot()
	if !state.Mute {
		t.Fatal("mute should be true")
	}
	if state.HUDVisible {
		t.Fatal("hud should be false")
	}
}
