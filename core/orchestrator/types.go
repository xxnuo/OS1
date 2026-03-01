package orchestrator

import "time"

type EventType string

const (
	SessionEventType     EventType = "session"
	AudioFrameEventType  EventType = "audio_frame"
	VadSegmentEventType  EventType = "vad_segment"
	AsrResultEventType   EventType = "asr_result"
	LlmDecisionEventType EventType = "llm_decision"
	TtsChunkEventType    EventType = "tts_chunk"
	HudStateEventType    EventType = "hud_state"
	AuditEventType       EventType = "audit"
)

type HUDState string

const (
	HUDIdle      HUDState = "idle"
	HUDListening HUDState = "listening"
	HUDThinking  HUDState = "thinking"
	HUDSpeaking  HUDState = "speaking"
)

type Event struct {
	Type      EventType `json:"type"`
	SessionID string    `json:"session_id,omitempty"`
	Time      time.Time `json:"time"`
	Payload   any       `json:"payload,omitempty"`
}

type ASRResult struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	LatencyMs  int     `json:"latency_ms"`
}

type LLMDecision struct {
	Intent    string   `json:"intent"`
	ReplyText string   `json:"reply_text"`
	Actions   []string `json:"actions"`
}

type TTSChunk struct {
	Chunk   string `json:"chunk"`
	IsFinal bool   `json:"is_final"`
}
