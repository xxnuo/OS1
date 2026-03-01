import type {
  ASRProvider,
  ASRResult,
  LLMDecision,
  LLMProvider,
  SegmentMeta,
  TTSChunk,
  TTSProvider
} from '@os1/protocol'

export class MockASRProvider implements ASRProvider {
  async transcribe(sessionId: string, segment: Uint8Array, meta: SegmentMeta): Promise<ASRResult> {
    return {
      text: `session:${sessionId} bytes:${segment.byteLength} dur:${meta.durationMs}`,
      confidence: 0.95,
      latencyMs: 30
    }
  }
}

export class MockLLMProvider implements LLMProvider {
  async respond(sessionId: string, userText: string): Promise<LLMDecision> {
    return {
      intent: 'chat',
      replyText: `Samantha(${sessionId}): ${userText}`,
      actions: []
    }
  }
}

export class MockTTSProvider implements TTSProvider {
  async *synthesize(sessionId: string, text: string): AsyncIterable<TTSChunk> {
    const words = `${sessionId} ${text}`.split(' ')
    for (const w of words) {
      await new Promise(resolve => setTimeout(resolve, 20))
      yield { chunk: w, isFinal: false }
    }
    yield { chunk: '', isFinal: true }
  }
}
