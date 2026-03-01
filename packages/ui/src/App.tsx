import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CapabilityEvent, HudStateEvent, MemoryEvent, Os1Event, TtsChunkEvent, UserSignalBatchEvent } from '@os1/protocol'
import { SamanthaHud } from './components/SamanthaHud'

type HudState = 'idle' | 'listening' | 'thinking' | 'speaking'

type ApiError = {
  error: string
}

type AuditRecentEvent = {
  type: string
  time: string
  sessionId?: string
}

type AuditRecentResp = {
  count: number
  events: AuditRecentEvent[]
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
  const [debug, setDebug] = useState(() => {
    try {
      const raw = localStorage.getItem('os1_debug')
      if (raw === '1') return true
      if (raw === '0') return false
    } catch {
    }
    return import.meta.env.DEV
  })
  const [state, setState] = useState<HudState>('idle')
  const [muted, setMuted] = useState(false)
  const [hudVisible, setHudVisible] = useState(true)
  const [connected, setConnected] = useState(false)
  const [sessionId, setSessionId] = useState('demo')
  const [input, setInput] = useState('')
  const [lastChunk, setLastChunk] = useState('')
  const [capabilityCount, setCapabilityCount] = useState(0)
  const [capabilityChange, setCapabilityChange] = useState('')
  const [memoryCount, setMemoryCount] = useState(0)
  const [memoryChange, setMemoryChange] = useState('')
  const [operationsPaused, setOperationsPaused] = useState(false)
  const [signalHint, setSignalHint] = useState('')
  const [auditLines, setAuditLines] = useState<string[]>([])
  const [error, setError] = useState('')

  const title = useMemo(() => {
    if (!connected) return 'OFFLINE'
    if (!hudVisible) return 'HIDDEN'
    if (muted) return 'MUTED'
    return state.toUpperCase()
  }, [connected, hudVisible, muted, state])

  const toggleDebug = useCallback(() => {
    setDebug(prev => {
      const next = !prev
      try {
        localStorage.setItem('os1_debug', next ? '1' : '0')
      } catch {
      }
      return next
    })
  }, [])

  const refreshState = useCallback(async () => {
    try {
      const data = await get<{ mute: boolean; hudVisible: boolean; operationsPaused: boolean }>('/api/state')
      setMuted(data.mute)
      setHudVisible(data.hudVisible)
      setOperationsPaused(data.operationsPaused)
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'state error')
    }
  }, [])

  const refreshCapabilities = useCallback(async () => {
    try {
      const data = await get<{ count: number }>('/api/capabilities')
      setCapabilityCount(data.count)
    } catch {
    }
  }, [])

  const refreshMemory = useCallback(async () => {
    try {
      const data = await get<{ count: number }>('/api/memory')
      setMemoryCount(data.count)
    } catch {
    }
  }, [])

  const refreshAudit = useCallback(async () => {
    try {
      const data = await get<AuditRecentResp>('/api/audit/recent?limit=80')
      setAuditLines(
        data.events.map(e => {
          const t = e.time.length >= 19 ? e.time.slice(11, 19) : e.time
          return `${t} ${e.type}`
        })
      )
    } catch {
    }
  }, [])

  useEffect(() => {
    void refreshState()
    void refreshCapabilities()
    void refreshMemory()
    void refreshAudit()
  }, [refreshAudit, refreshCapabilities, refreshMemory, refreshState])

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
        setAuditLines(prev => {
          const t = event.time.length >= 19 ? event.time.slice(11, 19) : event.time
          return [...prev.slice(-59), `${t} ${event.type}`]
        })
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
          if (typeof payload.hudVisible === 'boolean') {
            setHudVisible(payload.hudVisible)
          }
          if (typeof payload.operationsPaused === 'boolean') {
            setOperationsPaused(payload.operationsPaused)
          }
        }
        if (event.type === 'capability') {
          const capabilityEvent = event as CapabilityEvent
          setCapabilityCount(capabilityEvent.payload.total)
          setCapabilityChange(
            `${capabilityEvent.payload.action.toUpperCase()} ${capabilityEvent.payload.capability.name}@${capabilityEvent.payload.capability.version}`
          )
        }
        if (event.type === 'memory') {
          const memoryEvent = event as MemoryEvent
          setMemoryCount(memoryEvent.payload.total)
          setMemoryChange(`${memoryEvent.payload.action.toUpperCase()} ${memoryEvent.payload.item.kind}`)
        }
        if (event.type === 'user_signal_batch') {
          const signalEvent = event as UserSignalBatchEvent
          const p = signalEvent.payload
          const parts = [
            p.window ? `WINDOW ${p.window.app}` : '',
            p.input ? `INPUT ${p.input.active ? 'ACTIVE' : 'IDLE'}` : '',
            p.fileChanges?.length ? `FILES ${p.fileChanges.length}` : '',
            p.clipboardChanged ? 'CLIPBOARD' : ''
          ].filter(Boolean)
          setSignalHint(parts.join(' · ') || 'SIGNAL')
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

  const sendSignal = useCallback(
    async (type: 'window' | 'input' | 'file' | 'clipboard') => {
      try {
        const base = { sessionId, type }
        const now = new Date().toISOString()
        if (type === 'window') {
          await post('/api/signals', {
            ...base,
            window: { app: 'Browser', windowTitle: 'OS1 Demo' }
          })
        }
        if (type === 'input') {
          await post('/api/signals', {
            ...base,
            input: { active: true, lastActivity: now }
          })
        }
        if (type === 'file') {
          await post('/api/signals', {
            ...base,
            file: { path: '/tmp/demo.txt', eventType: 'write', timestamp: now }
          })
        }
        if (type === 'clipboard') {
          await post('/api/signals', {
            ...base,
            clipboard: { timestamp: now }
          })
        }
        setError('')
      } catch (e) {
        setError(e instanceof Error ? e.message : 'signal error')
      }
    },
    [sessionId]
  )

  const addMemory = useCallback(async () => {
    try {
      await post('/api/memory/create', {
        sessionId,
        kind: 'preference',
        content: 'likes warm organic visuals',
        tags: ['visual', 'style'],
        confidence: 0.9,
        source: 'user'
      })
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'memory error')
    }
  }, [sessionId])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'F7') {
        e.preventDefault()
        void toggleHud()
      }
      if (e.key === 'F8') {
        e.preventDefault()
        toggleDebug()
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
  }, [interrupt, toggleDebug, toggleHud, toggleMute])

  return (
    <div className="h-screen w-screen bg-bg text-glow">
      <SamanthaHud
        state={state}
        muted={muted}
        title={title}
        visible={hudVisible}
        connected={connected}
        lastChunk={lastChunk}
        capabilityCount={capabilityCount}
        capabilityChange={capabilityChange}
        memoryCount={memoryCount}
        memoryChange={memoryChange}
        operationsPaused={operationsPaused}
        signalHint={signalHint}
        error={error}
        debug={debug}
      />
      {debug ? (
        <>
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
            <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void sendSignal('window')}>
              SIGNAL WINDOW
            </button>
            <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void sendSignal('input')}>
              SIGNAL INPUT
            </button>
            <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void sendSignal('file')}>
              SIGNAL FILE
            </button>
            <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void sendSignal('clipboard')}>
              SIGNAL CLIPBOARD
            </button>
            <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => void addMemory()}>
              ADD MEMORY
            </button>
            <button className="h-10 rounded-lg border border-glow/30 px-4 text-sm" onClick={() => toggleDebug()}>
              HIDE DEBUG (F8)
            </button>
          </div>
          {auditLines.length ? (
            <div className="absolute left-6 top-6 max-w-[52vw] rounded border border-glow/20 bg-black/30 px-3 py-2 text-[10px] leading-5 text-glow/70 backdrop-blur">
              {auditLines.slice(-10).map((line, i) => (
                <div key={i} className="truncate">
                  {line}
                </div>
              ))}
            </div>
          ) : null}
          {error ? <div className="absolute right-6 top-6 rounded border border-red-300/40 bg-red-950/40 px-3 py-2 text-xs text-red-200">{error}</div> : null}
        </>
      ) : null}
    </div>
  )
}
