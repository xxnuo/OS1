import type { Capability } from './capabilities'
import type { ClipboardSignal, FileSignal, InputSignal, UserSignalBatch, WindowSignal } from './signals'
import type { MemoryItem } from './memory'

export type EventType =
  | 'session'
  | 'audio_frame'
  | 'vad_segment'
  | 'asr_result'
  | 'llm_decision'
  | 'tts_chunk'
  | 'hud_state'
  | 'capability'
  | 'memory'
  | 'window_changed'
  | 'input_activity'
  | 'file_changed'
  | 'clipboard_changed'
  | 'user_signal_batch'
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

export type CapabilityEvent = {
  type: 'capability'
  sessionId: string
  time: string
  payload: {
    action: 'registered' | 'unregistered'
    capability: Capability
    total: number
  }
}

export type MemoryEvent = {
  type: 'memory'
  sessionId: string
  time: string
  payload: {
    action: 'created' | 'updated' | 'deleted' | 'locked'
    item: MemoryItem
    total: number
  }
}

export type WindowChangedEvent = {
  type: 'window_changed'
  sessionId: string
  time: string
  payload: WindowSignal
}

export type InputActivityEvent = {
  type: 'input_activity'
  sessionId: string
  time: string
  payload: InputSignal
}

export type FileChangedEvent = {
  type: 'file_changed'
  sessionId: string
  time: string
  payload: FileSignal
}

export type ClipboardChangedEvent = {
  type: 'clipboard_changed'
  sessionId: string
  time: string
  payload: ClipboardSignal
}

export type UserSignalBatchEvent = {
  type: 'user_signal_batch'
  sessionId: string
  time: string
  payload: UserSignalBatch
}

export type Os1Event =
  | SessionEvent
  | AudioFrameEvent
  | VadSegmentEvent
  | AsrResultEvent
  | LlmDecisionEvent
  | TtsChunkEvent
  | HudStateEvent
  | CapabilityEvent
  | MemoryEvent
  | WindowChangedEvent
  | InputActivityEvent
  | FileChangedEvent
  | ClipboardChangedEvent
  | UserSignalBatchEvent
  | AuditEvent
