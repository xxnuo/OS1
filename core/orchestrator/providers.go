package orchestrator

import "context"

type ASRProvider interface {
	Transcribe(ctx context.Context, sessionID string, segment []byte, meta SegmentMeta) (ASRResult, error)
}

type LLMProvider interface {
	Respond(ctx context.Context, sessionID string, userText string) (LLMDecision, error)
}

type TTSProvider interface {
	Synthesize(ctx context.Context, sessionID string, text string) (<-chan TTSChunk, error)
}
