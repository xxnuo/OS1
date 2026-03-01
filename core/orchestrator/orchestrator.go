package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"

	"os1/core/platform"
)

type Runtime struct {
	mu             sync.Mutex
	asr            ASRProvider
	llm            LLMProvider
	tts            TTSProvider
	bus            *Bus
	audit          *AuditWriter
	platform       platform.Adapter
	mute           bool
	hudVisible     bool
	speakingCancel map[string]context.CancelFunc
	speakingGen    map[string]uint64
}

func NewRuntime(asr ASRProvider, llm LLMProvider, tts TTSProvider, bus *Bus, audit *AuditWriter, platformAdapter platform.Adapter) *Runtime {
	return &Runtime{
		asr:            asr,
		llm:            llm,
		tts:            tts,
		bus:            bus,
		audit:          audit,
		platform:       platformAdapter,
		hudVisible:     true,
		speakingCancel: map[string]context.CancelFunc{},
		speakingGen:    map[string]uint64{},
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
	r.mu.Unlock()
	if r.platform != nil {
		if err := r.platform.SetHUDVisible(visible); err != nil {
			return visible, err
		}
	}
	r.emit(Event{Type: AuditEventType, Time: time.Now(), Payload: map[string]any{"hud_visible": visible}})
	return visible, nil
}

func (r *Runtime) Snapshot() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return State{
		Mute:       r.mute,
		HUDVisible: r.hudVisible,
	}
}
