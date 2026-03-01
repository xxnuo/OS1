import { useCallback, useEffect, useMemo, useState } from 'react'
import type { HudStateEvent, Os1Event, TtsChunkEvent } from '@os1/protocol'
import { SamanthaHud } from './components/SamanthaHud'

type HudState = 'idle' | 'listening' | 'thinking' | 'speaking'

type ApiError = {
  error: string
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
  const data = await res.json()
  if (!res.ok) {
    throw new Error((data as ApiError).error ?? 'request failed')
  }
  return data as T
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  const data = await res.json()
  if (!res.ok) {
    throw new Error((data as ApiError).error ?? 'request failed')
  }
  return data as T
}

export function App() {
  const [state, setState] = useState<HudState>('idle')
  const [muted, setMuted] = useState(false)
  const [hudVisible, setHudVisible] = useState(true)
  const [connected, setConnected] = useState(false)
  const [sessionId, setSessionId] = useState('demo')
  const [input, setInput] = useState('')
  const [lastChunk, setLastChunk] = useState('')
  const [error, setError] = useState('')

  const title = useMemo(() => {
    if (!connected) return 'OFFLINE'
    if (!hudVisible) return 'HIDDEN'
    if (muted) return 'MUTED'
    return state.toUpperCase()
  }, [connected, hudVisible, muted, state])

  const refreshState = useCallback(async () => {
    try {
      const data = await get<{ mute: boolean; hudVisible: boolean }>('/api/state')
      setMuted(data.mute)
      setHudVisible(data.hudVisible)
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'state error')
    }
  }, [])

  useEffect(() => {
    void refreshState()
  }, [refreshState])

  useEffect(() => {
    const es = new EventSource('/api/events')
    es.onopen = () => {
      setConnected(true)
      setError('')
    }
    es.onerror = () => {
      setConnected(false)
    }
    es.onmessage = msg => {
      try {
        const event = JSON.parse(msg.data) as Os1Event
        if (event.type === 'hud_state') {
          const hud = event as HudStateEvent
          setState(hud.payload.state)
        }
        if (event.type === 'tts_chunk') {
          const chunk = event as TtsChunkEvent
          if (chunk.payload.chunk) {
            setLastChunk(chunk.payload.chunk)
          }
        }
        if (event.type === 'audit') {
          const payload = event.payload as Record<string, unknown>
          if (typeof payload.mute === 'boolean') {
            setMuted(payload.mute)
          }
          if (typeof payload.hud_visible === 'boolean') {
            setHudVisible(payload.hud_visible)
          }
        }
      } catch {
      }
    }
    return () => es.close()
  }, [])

  const startSession = useCallback(async () => {
    try {
      await post('/api/session/start', { sessionId })
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'start error')
    }
  }, [sessionId])

  const sendAudio = useCallback(async () => {
    const text = input.trim()
    if (!text) {
      return
    }
    try {
      await post('/api/audio', { sessionId, text, durationMs: 800 })
      setInput('')
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'audio error')
    }
  }, [input, sessionId])

  const toggleMute = useCallback(async () => {
    try {
      await post('/api/mute', { enabled: !muted })
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'mute error')
    }
  }, [muted])

  const toggleHud = useCallback(async () => {
    try {
      await post('/api/hud/toggle', {})
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'toggle error')
    }
  }, [])

  const interrupt = useCallback(async () => {
    try {
      await post('/api/interrupt', { sessionId })
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'interrupt error')
    }
  }, [sessionId])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'F7') {
        e.preventDefault()
        void toggleHud()
      }
      if (e.key.toLowerCase() === 'm') {
        void toggleMute()
      }
      if (e.key === 'Escape') {
        void interrupt()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [interrupt, toggleHud, toggleMute])

  return (
    <div className="h-screen w-screen bg-bg text-glow">
      <SamanthaHud
        state={state}
        muted={muted}
        title={title}
        visible={hudVisible}
        connected={connected}
        lastChunk={lastChunk}
      />
      <div className="absolute bottom-6 left-6 right-6 flex flex-wrap items-center gap-3 rounded-xl border border-glow/20 bg-black/35 p-3 backdrop-blur">
        <input
          className="h-10 min-w-[220px] flex-1 rounded-lg border border-glow/30 bg-black/45 px-3 text-sm text-glow outline-none"
          value={sessionId}
          onChange={e => setSessionId(e.target.value)}
          placeholder="session id"
        />
        <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void startSession()}>
          START
        </button>
        <input
          className="h-10 min-w-[280px] flex-[2] rounded-lg border border-glow/30 bg-black/45 px-3 text-sm text-glow outline-none"
          value={input}
          onChange={e => setInput(e.target.value)}
          placeholder="输入一句话模拟语音"
          onKeyDown={e => {
            if (e.key === 'Enter') {
              void sendAudio()
            }
          }}
        />
        <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void sendAudio()}>
          SEND
        </button>
        <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void interrupt()}>
          INTERRUPT
        </button>
        <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void toggleMute()}>
          {muted ? 'UNMUTE' : 'MUTE'}
        </button>
        <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void toggleHud()}>
          TOGGLE HUD
        </button>
      </div>
      {error ? <div className="absolute right-6 top-6 rounded border border-red-300/40 bg-red-950/40 px-3 py-2 text-xs text-red-200">{error}</div> : null}
    </div>
  )
}
