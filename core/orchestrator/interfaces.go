package orchestrator

type Orchestrator interface {
	StartSession(sessionID string) error
	HandleUserAudio(sessionID string, segment []byte, meta SegmentMeta) error
	InterruptSpeaking(sessionID string) error
	SetMute(enabled bool) error
	ToggleHUD() (bool, error)
	RegisterCapability(sessionID string, spec CapabilitySpec) (Capability, error)
	UnregisterCapability(sessionID string, capabilityID string) error
	ListCapabilities() []Capability
	CreateMemory(sessionID string, input MemoryCreate) (MemoryItem, error)
	UpdateMemory(sessionID string, id string, patch MemoryPatch) (MemoryItem, error)
	DeleteMemory(sessionID string, id string) error
	LockMemory(sessionID string, id string, locked bool) (MemoryItem, error)
	ListMemory(includeDeleted bool) []MemoryItem
	IngestUserSignal(sessionID string, signal UserSignal) error
	Snapshot() State
}

type SegmentMeta struct {
	SampleRate int `json:"sampleRate"`
	DurationMs int `json:"durationMs"`
}

type State struct {
	Mute             bool `json:"mute"`
	HUDVisible       bool `json:"hudVisible"`
	OperationsPaused bool `json:"operationsPaused"`
}
