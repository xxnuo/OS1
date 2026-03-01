package orchestrator

type Orchestrator interface {
	StartSession(sessionID string) error
	HandleUserAudio(sessionID string, segment []byte, meta SegmentMeta) error
	InterruptSpeaking(sessionID string) error
	SetMute(enabled bool) error
	ToggleHUD() (bool, error)
	Snapshot() State
}

type SegmentMeta struct {
	SampleRate int
	DurationMs int
}

type State struct {
	Mute       bool `json:"mute"`
	HUDVisible bool `json:"hud_visible"`
}
