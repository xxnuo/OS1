export type EventType =
  | 'session'
  | 'audio_frame'
  | 'vad_segment'
  | 'asr_result'
  | 'llm_decision'
  | 'tts_chunk'
  | 'hud_state'
  | 'audit'

export type HudState = 'idle' | 'listening' | 'thinking' | 'speaking'

export type SessionEvent = {
  type: 'session'
  sessionId: string
  time: string
  payload: { status: 'started' | 'stopped' }
}

export type AudioFrameEvent = {
  type: 'audio_frame'
  sessionId: string
  time: string
  payload: { bytes: number }
}

export type VadSegmentEvent = {
  type: 'vad_segment'
  sessionId: string
  time: string
  payload: { sampleRate: number; durationMs: number }
}

export type AsrResultEvent = {
  type: 'asr_result'
  sessionId: string
  time: string
  payload: { text: string; confidence: number; latencyMs: number }
}

export type LlmDecisionEvent = {
  type: 'llm_decision'
  sessionId: string
  time: string
  payload: { intent: string; replyText: string; actions: string[] }
}

export type TtsChunkEvent = {
  type: 'tts_chunk'
  sessionId: string
  time: string
  payload: { chunk: string; isFinal: boolean }
}

export type HudStateEvent = {
  type: 'hud_state'
  sessionId: string
  time: string
  payload: { state: HudState }
}

export type AuditEvent = {
  type: 'audit'
  time: string
  payload: Record<string, unknown>
}

export type Os1Event =
  | SessionEvent
  | AudioFrameEvent
  | VadSegmentEvent
  | AsrResultEvent
  | LlmDecisionEvent
  | TtsChunkEvent
  | HudStateEvent
  | AuditEvent
