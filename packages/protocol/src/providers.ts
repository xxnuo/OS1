export type SegmentMeta = {
  sampleRate: number
  durationMs: number
}

export type ASRResult = {
  text: string
  confidence: number
  latencyMs: number
}

export type LLMDecision = {
  intent: string
  replyText: string
  actions: string[]
}

export type TTSChunk = {
  chunk: string
  isFinal: boolean
}

export type ProviderHealth = {
  ok: boolean
  detail: string
}

export interface ASRProvider {
  transcribe(sessionId: string, segment: Uint8Array, meta: SegmentMeta): Promise<ASRResult>
}

export interface LLMProvider {
  respond(sessionId: string, userText: string): Promise<LLMDecision>
}

export interface TTSProvider {
  synthesize(sessionId: string, text: string): AsyncIterable<TTSChunk>
}
