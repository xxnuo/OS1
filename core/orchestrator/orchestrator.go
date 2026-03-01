package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"os1/core/platform"
)

type Runtime struct {
	mu               sync.Mutex
	asr              ASRProvider
	llm              LLMProvider
	tts              TTSProvider
	bus              *Bus
	audit            *AuditWriter
	platform         platform.Adapter
	capabilities     *CapabilityRegistry
	memory           *MemoryStore
	mute             bool
	hudVisible       bool
	operationsPaused bool
	speakingCancel   map[string]context.CancelFunc
	speakingGen      map[string]uint64
	signalMu         sync.Mutex
	signalBuffers    map[string]*signalBuffer
	signalWindow     time.Duration
}

func NewRuntime(asr ASRProvider, llm LLMProvider, tts TTSProvider, bus *Bus, audit *AuditWriter, platformAdapter platform.Adapter, memoryStore *MemoryStore) *Runtime {
	registry := NewCapabilityRegistry()
	for _, spec := range BuiltinCapabilitySpecs() {
		_, _ = registry.RegisterBuiltin(spec)
	}
	return &Runtime{
		asr:              asr,
		llm:              llm,
		tts:              tts,
		bus:              bus,
		audit:            audit,
		platform:         platformAdapter,
		capabilities:     registry,
		memory:           memoryStore,
		hudVisible:       true,
		operationsPaused: false,
		speakingCancel:   map[string]context.CancelFunc{},
		speakingGen:      map[string]uint64{},
		signalBuffers:    map[string]*signalBuffer{},
		signalWindow:     300 * time.Millisecond,
	}
}

func (r *Runtime) emit(e Event) {
	r.bus.Publish(e)
	if r.audit != nil {
		_ = r.audit.Write(e)
	}
}

func (r *Runtime) emitState(sessionID string, state HUDState) {
	r.emit(Event{
		Type:      HudStateEventType,
		SessionID: sessionID,
		Time:      time.Now(),
		Payload: map[string]any{
			"state": state,
		},
	})
}

func (r *Runtime) StartSession(sessionID string) error {
	if sessionID == "" {
		return errors.New("empty session")
	}
	r.emit(Event{Type: SessionEventType, SessionID: sessionID, Time: time.Now(), Payload: map[string]any{"status": "started"}})
	r.emitState(sessionID, HUDListening)
	return nil
}

func (r *Runtime) HandleUserAudio(sessionID string, segment []byte, meta SegmentMeta) error {
	if sessionID == "" {
		return errors.New("empty session")
	}
	r.mu.Lock()
	if r.mute {
		r.mu.Unlock()
		return errors.New("muted")
	}
	if cancel, ok := r.speakingCancel[sessionID]; ok {
		cancel()
		delete(r.speakingCancel, sessionID)
	}
	r.mu.Unlock()
	r.emitState(sessionID, HUDListening)
	r.emit(Event{Type: AudioFrameEventType, SessionID: sessionID, Time: time.Now(), Payload: map[string]any{"bytes": len(segment)}})
	r.emit(Event{Type: VadSegmentEventType, SessionID: sessionID, Time: time.Now(), Payload: meta})
	r.emitState(sessionID, HUDThinking)
	ctx := context.Background()
	asrRes, err := r.asr.Transcribe(ctx, sessionID, segment, meta)
	if err != nil {
		return err
	}
	r.emit(Event{Type: AsrResultEventType, SessionID: sessionID, Time: time.Now(), Payload: asrRes})
	llmRes, err := r.llm.Respond(ctx, sessionID, asrRes.Text)
	if err != nil {
		return err
	}
	r.emit(Event{Type: LlmDecisionEventType, SessionID: sessionID, Time: time.Now(), Payload: llmRes})
	r.emitState(sessionID, HUDSpeaking)
	ttsCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.speakingGen[sessionID]++
	gen := r.speakingGen[sessionID]
	r.speakingCancel[sessionID] = cancel
	r.mu.Unlock()
	stream, err := r.tts.Synthesize(ttsCtx, sessionID, llmRes.ReplyText)
	if err != nil {
		return err
	}
	go func() {
		for chunk := range stream {
			r.emit(Event{Type: TtsChunkEventType, SessionID: sessionID, Time: time.Now(), Payload: chunk})
			if chunk.IsFinal {
				r.mu.Lock()
				if currentGen, ok := r.speakingGen[sessionID]; ok && currentGen == gen {
					delete(r.speakingCancel, sessionID)
				}
				r.mu.Unlock()
				r.emitState(sessionID, HUDListening)
			}
		}
	}()
	return nil
}

func (r *Runtime) InterruptSpeaking(sessionID string) error {
	r.mu.Lock()
	cancel, ok := r.speakingCancel[sessionID]
	if ok {
		cancel()
		delete(r.speakingCancel, sessionID)
	}
	r.mu.Unlock()
	r.emitState(sessionID, HUDListening)
	return nil
}

func (r *Runtime) SetMute(enabled bool) error {
	r.mu.Lock()
	r.mute = enabled
	r.mu.Unlock()
	if r.platform != nil {
		_ = r.platform.SetTrayMute(enabled)
	}
	r.emit(Event{Type: AuditEventType, Time: time.Now(), Payload: map[string]any{"mute": enabled}})
	return nil
}

func (r *Runtime) ToggleHUD() (bool, error) {
	r.mu.Lock()
	r.hudVisible = !r.hudVisible
	visible := r.hudVisible
	r.operationsPaused = !visible
	paused := r.operationsPaused
	r.mu.Unlock()
	if r.platform != nil {
		if err := r.platform.SetHUDVisible(visible); err != nil {
			return visible, err
		}
	}
	r.emit(Event{Type: AuditEventType, Time: time.Now(), Payload: map[string]any{
		"hudVisible":       visible,
		"operationsPaused": paused,
	}})
	return visible, nil
}

func (r *Runtime) RegisterCapability(sessionID string, spec CapabilitySpec) (Capability, error) {
	capability, err := r.capabilities.Register(spec)
	if err != nil {
		return Capability{}, err
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		sid = "system"
	}
	r.emit(Event{
		Type:      CapabilityEventType,
		SessionID: sid,
		Time:      time.Now(),
		Payload: map[string]any{
			"action":     "registered",
			"capability": capability,
			"total":      r.capabilities.Count(),
		},
	})
	return capability, nil
}

func (r *Runtime) UnregisterCapability(sessionID string, capabilityID string) error {
	capability, err := r.capabilities.Unregister(capabilityID)
	if err != nil {
		return err
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		sid = "system"
	}
	r.emit(Event{
		Type:      CapabilityEventType,
		SessionID: sid,
		Time:      time.Now(),
		Payload: map[string]any{
			"action":     "unregistered",
			"capability": capability,
			"total":      r.capabilities.Count(),
		},
	})
	return nil
}

func (r *Runtime) ListCapabilities() []Capability {
	return r.capabilities.List()
}

func (r *Runtime) CreateMemory(sessionID string, input MemoryCreate) (MemoryItem, error) {
	if r.memory == nil {
		return MemoryItem{}, errors.New("memory store unavailable")
	}
	item, err := r.memory.Create(input)
	if err != nil {
		return MemoryItem{}, err
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		sid = "system"
	}
	r.emit(Event{
		Type:      MemoryEventType,
		SessionID: sid,
		Time:      time.Now(),
		Payload: map[string]any{
			"action": "created",
			"item":   item,
			"total":  r.memory.Count(false),
		},
	})
	return item, nil
}

func (r *Runtime) UpdateMemory(sessionID string, id string, patch MemoryPatch) (MemoryItem, error) {
	if r.memory == nil {
		return MemoryItem{}, errors.New("memory store unavailable")
	}
	item, err := r.memory.Update(id, patch)
	if err != nil {
		return MemoryItem{}, err
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		sid = "system"
	}
	r.emit(Event{
		Type:      MemoryEventType,
		SessionID: sid,
		Time:      time.Now(),
		Payload: map[string]any{
			"action": "updated",
			"item":   item,
			"total":  r.memory.Count(false),
		},
	})
	return item, nil
}

func (r *Runtime) DeleteMemory(sessionID string, id string) error {
	if r.memory == nil {
		return errors.New("memory store unavailable")
	}
	item, err := r.memory.Delete(id)
	if err != nil {
		return err
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		sid = "system"
	}
	r.emit(Event{
		Type:      MemoryEventType,
		SessionID: sid,
		Time:      time.Now(),
		Payload: map[string]any{
			"action": "deleted",
			"item":   item,
			"total":  r.memory.Count(false),
		},
	})
	return nil
}

func (r *Runtime) LockMemory(sessionID string, id string, locked bool) (MemoryItem, error) {
	if r.memory == nil {
		return MemoryItem{}, errors.New("memory store unavailable")
	}
	item, err := r.memory.Lock(id, locked)
	if err != nil {
		return MemoryItem{}, err
	}
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		sid = "system"
	}
	r.emit(Event{
		Type:      MemoryEventType,
		SessionID: sid,
		Time:      time.Now(),
		Payload: map[string]any{
			"action": "locked",
			"item":   item,
			"total":  r.memory.Count(false),
		},
	})
	return item, nil
}

func (r *Runtime) ListMemory(includeDeleted bool) []MemoryItem {
	if r.memory == nil {
		return nil
	}
	return r.memory.List(includeDeleted)
}

func (r *Runtime) Snapshot() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return State{
		Mute:             r.mute,
		HUDVisible:       r.hudVisible,
		OperationsPaused: r.operationsPaused,
	}
}
