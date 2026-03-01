import { useEffect, useMemo, useRef } from 'react'

type HudState = 'idle' | 'listening' | 'thinking' | 'speaking'

type Props = {
  state: HudState
  muted: boolean
  title: string
  visible: boolean
  connected: boolean
  lastChunk: string
  capabilityCount: number
  capabilityChange: string
  memoryCount: number
  memoryChange: string
  operationsPaused: boolean
  signalHint: string
  error: string
  debug: boolean
}

type Particle = {
  x: number
  y: number
  vx: number
  vy: number
  seed: number
  size: number
  glow: number
}

export function SamanthaHud({
  state,
  muted,
  title,
  visible,
  connected,
  lastChunk,
  capabilityCount,
  capabilityChange,
  memoryCount,
  memoryChange,
  operationsPaused,
  signalHint,
  error,
  debug
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const particlesRef = useRef<Particle[]>([])
  const rafRef = useRef<number | null>(null)
  const lastTsRef = useRef<number>(0)
  const energyRef = useRef<number>(0.2)
  const impulseRef = useRef<{ speak: number; think: number; event: number; error: number }>({
    speak: 0,
    think: 0,
    event: 0,
    error: 0
  })
  const lastRef = useRef<{ state: HudState; capability: string; memory: string; signal: string; chunk: string; connected: boolean; error: string }>({
    state: 'idle',
    capability: '',
    memory: '',
    signal: '',
    chunk: '',
    connected: true,
    error: ''
  })

  const targetEnergy = useMemo(() => {
    if (!connected) return 0.12
    if (muted) return 0.18
    if (state === 'idle') return 0.2
    if (state === 'listening') return 0.42
    if (state === 'thinking') return 0.68
    return 0.92
  }, [connected, muted, state])

  useEffect(() => {
    const last = lastRef.current
    if (last.state !== state) {
      impulseRef.current.event = Math.min(2.5, impulseRef.current.event + 0.6)
      if (state === 'thinking') impulseRef.current.think = Math.min(2.2, impulseRef.current.think + 0.9)
    }
    if (capabilityChange && capabilityChange !== last.capability) {
      impulseRef.current.event = Math.min(2.5, impulseRef.current.event + 0.9)
    }
    if (memoryChange && memoryChange !== last.memory) {
      impulseRef.current.event = Math.min(2.5, impulseRef.current.event + 0.85)
    }
    if (signalHint && signalHint !== last.signal) {
      impulseRef.current.event = Math.min(2.5, impulseRef.current.event + 0.55)
    }
    if (lastChunk && lastChunk !== last.chunk) {
      impulseRef.current.speak = Math.min(2.6, impulseRef.current.speak + 0.35)
    }
    if ((!connected && last.connected) || (!!error && error !== last.error)) {
      impulseRef.current.error = Math.min(2.8, impulseRef.current.error + 1.25)
    }
    lastRef.current = {
      state,
      capability: capabilityChange,
      memory: memoryChange,
      signal: signalHint,
      chunk: lastChunk,
      connected,
      error
    }
  }, [capabilityChange, connected, error, lastChunk, memoryChange, signalHint, state])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d', { alpha: false })
    if (!ctx) return

    const ensureParticles = (count: number) => {
      const arr = particlesRef.current
      if (arr.length >= count) return
      for (let i = arr.length; i < count; i++) {
        const seed = Math.random()
        const a = Math.random() * Math.PI * 2
        const r = Math.sqrt(Math.random()) * 1.08
        arr.push({
          x: Math.cos(a) * r,
          y: Math.sin(a) * r,
          vx: (Math.random() - 0.5) * 0.05,
          vy: (Math.random() - 0.5) * 0.05,
          seed,
          size: 0.6 + seed * 1.9,
          glow: 0.25 + seed * 0.75
        })
      }
    }

    const resize = () => {
      const dpr = Math.max(1, Math.min(2, window.devicePixelRatio || 1))
      const w = Math.max(1, Math.floor(canvas.clientWidth * dpr))
      const h = Math.max(1, Math.floor(canvas.clientHeight * dpr))
      if (canvas.width === w && canvas.height === h) return
      canvas.width = w
      canvas.height = h
    }

    const draw = (ts: number) => {
      rafRef.current = requestAnimationFrame(draw)
      const lastTs = lastTsRef.current || ts
      lastTsRef.current = ts
      const dtRaw = Math.min(0.033, Math.max(0.001, (ts - lastTs) / 1000))
      const dt = operationsPaused ? dtRaw * 0.15 : dtRaw
      const w = canvas.width
      const h = canvas.height
      const cx = w * 0.5
      const cy = h * 0.5
      const minSide = Math.min(w, h)
      const scale = minSide * 0.44

      energyRef.current += (targetEnergy - energyRef.current) * Math.min(1, dt * 3.5)
      const energy = energyRef.current

      impulseRef.current.speak = Math.max(0, impulseRef.current.speak - dt * 1.25)
      impulseRef.current.think = Math.max(0, impulseRef.current.think - dt * 0.95)
      impulseRef.current.event = Math.max(0, impulseRef.current.event - dt * 1.05)
      impulseRef.current.error = Math.max(0, impulseRef.current.error - dt * 0.55)

      const speak = impulseRef.current.speak
      const think = impulseRef.current.think
      const eventPulse = impulseRef.current.event
      const errorPulse = impulseRef.current.error

      const baseR = 0.45 + energy * 0.42 + speak * 0.05
      const bg = ctx.createLinearGradient(0, 0, w, h)
      bg.addColorStop(0, '#05090e')
      bg.addColorStop(0.45, '#080d13')
      bg.addColorStop(1, '#04070b')
      ctx.fillStyle = bg
      ctx.fillRect(0, 0, w, h)

      const vignette = ctx.createRadialGradient(cx, cy, minSide * 0.12, cx, cy, minSide * 0.72)
      vignette.addColorStop(0, 'rgba(0,0,0,0)')
      vignette.addColorStop(1, 'rgba(0,0,0,0.55)')
      ctx.fillStyle = vignette
      ctx.fillRect(0, 0, w, h)

      const particleCount = Math.floor(720 + energy * 520)
      ensureParticles(particleCount)

      const swirl = 0.35 + energy * 0.95 + think * 0.35
      const pull = 0.22 + energy * 0.25
      const burst = eventPulse * 0.35 + speak * 0.18

      ctx.save()
      ctx.globalCompositeOperation = 'lighter'
      const particles = particlesRef.current
      const tw = ts * 0.001

      for (let i = 0; i < particles.length; i++) {
        const p = particles[i]
        const r2 = p.x * p.x + p.y * p.y
        const r = Math.sqrt(r2) || 0.0001
        const nx = p.x / r
        const ny = p.y / r
        const ax = -p.y * swirl - p.x * pull + Math.sin(tw * 0.7 + p.seed * 9.3) * 0.08 + nx * burst
        const ay = p.x * swirl - p.y * pull + Math.cos(tw * 0.9 + p.seed * 7.1) * 0.08 + ny * burst
        p.vx = (p.vx + ax * dt) * 0.986
        p.vy = (p.vy + ay * dt) * 0.986
        p.x += p.vx * dt
        p.y += p.vy * dt
        const rr = Math.sqrt(p.x * p.x + p.y * p.y)
        if (rr > 1.55) {
          const a = Math.random() * Math.PI * 2
          const nr = Math.sqrt(Math.random()) * 0.65
          p.x = Math.cos(a) * nr
          p.y = Math.sin(a) * nr
          p.vx = (Math.random() - 0.5) * 0.05
          p.vy = (Math.random() - 0.5) * 0.05
        }

        const px = cx + p.x * scale
        const py = cy + p.y * scale
        const fade = Math.max(0, 1 - rr / 1.55)
        const a = fade * (0.14 + energy * 0.55) * p.glow
        if (a <= 0.002) continue

        const warm = 0.62 + p.seed * 0.38
        const coolShift = !connected ? 0.55 : 0
        const red = Math.floor(160 + warm * 90 - coolShift * 80)
        const green = Math.floor(115 + warm * 80 - coolShift * 40)
        const blue = Math.floor(70 + warm * 55 + coolShift * 90)

        ctx.fillStyle = `rgba(${red},${green},${blue},${a})`
        ctx.beginPath()
        ctx.arc(px, py, p.size * (0.9 + energy * 0.85), 0, Math.PI * 2)
        ctx.fill()
      }

      const orbR = scale * baseR
      const wobble1 = Math.sin(tw * 1.15) * 0.025 + Math.sin(tw * 2.35) * 0.018
      const wobble2 = Math.sin(tw * 0.85 + 1.7) * 0.02 + Math.sin(tw * 1.9 + 0.4) * 0.014
      const orbOuter = orbR * (1 + wobble1 + wobble2)

      const coreWarm = connected ? 1 : 0.4
      const errTint = Math.min(1, errorPulse * 0.8)
      const coreR = Math.floor(246 - errTint * 120)
      const coreG = Math.floor(178 - errTint * 140)
      const coreB = Math.floor(107 - errTint * 60)

      const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, orbOuter * 1.15)
      g.addColorStop(0, `rgba(${coreR},${coreG},${coreB},${0.38 + energy * 0.28})`)
      g.addColorStop(0.35, `rgba(${coreR},${coreG},${coreB},${0.22 + energy * 0.18})`)
      g.addColorStop(1, `rgba(${coreR},${coreG},${coreB},0)`)
      ctx.fillStyle = g
      ctx.beginPath()
      ctx.arc(cx, cy, orbOuter * 1.15, 0, Math.PI * 2)
      ctx.fill()

      const steps = 96
      const ringAlpha = (0.14 + energy * 0.26) * coreWarm + eventPulse * 0.07 + speak * 0.05
      ctx.strokeStyle = `rgba(${coreR},${coreG},${coreB},${ringAlpha})`
      ctx.lineWidth = Math.max(1, minSide * (0.0012 + energy * 0.0012 + speak * 0.0005))
      ctx.beginPath()
      for (let i = 0; i <= steps; i++) {
        const a = (i / steps) * Math.PI * 2
        const wob = 1 + Math.sin(a * 3 + tw * 1.4) * (0.012 + energy * 0.008) + Math.sin(a * 7 + tw * 0.9) * (0.008 + think * 0.01)
        const rr = orbOuter * wob
        const x = cx + Math.cos(a) * rr
        const y = cy + Math.sin(a) * rr
        if (i === 0) ctx.moveTo(x, y)
        else ctx.lineTo(x, y)
      }
      ctx.closePath()
      ctx.stroke()

      if (state === 'thinking' || think > 0.01) {
        const arcN = 3
        for (let k = 0; k < arcN; k++) {
          const radius = orbOuter * (1.02 + k * 0.07 + Math.sin(tw * 0.7 + k) * 0.015)
          const a0 = tw * (0.55 + k * 0.12) + k * 1.7
          const span = 0.85 + energy * 0.55
          ctx.strokeStyle = `rgba(255,215,163,${0.08 + think * 0.18})`
          ctx.lineWidth = Math.max(1, minSide * (0.0008 + energy * 0.0007))
          ctx.beginPath()
          ctx.arc(cx, cy, radius, a0, a0 + span, false)
          ctx.stroke()
        }
      }

      if (state === 'speaking' || speak > 0.01) {
        const count = 3
        for (let k = 0; k < count; k++) {
          const t = (tw * 120 + k * 80) % 260
          const r = orbOuter * 0.95 + t
          const a = Math.max(0, 1 - t / 260) * (0.12 + speak * 0.18)
          if (a <= 0.002) continue
          ctx.strokeStyle = `rgba(255,215,163,${a})`
          ctx.lineWidth = Math.max(1, minSide * 0.0009)
          ctx.beginPath()
          ctx.arc(cx, cy, r, 0, Math.PI * 2)
          ctx.stroke()
        }
      }

      if (errorPulse > 0.01) {
        const r = orbOuter * (1.08 + Math.sin(tw * 6.2) * 0.02)
        ctx.strokeStyle = `rgba(255,90,70,${0.12 + errorPulse * 0.22})`
        ctx.lineWidth = Math.max(1, minSide * 0.0014)
        ctx.beginPath()
        ctx.arc(cx, cy, r, 0, Math.PI * 2)
        ctx.stroke()
      }

      ctx.restore()
    }

    resize()
    const onResize = () => resize()
    window.addEventListener('resize', onResize)
    rafRef.current = requestAnimationFrame(draw)
    return () => {
      window.removeEventListener('resize', onResize)
      if (rafRef.current != null) cancelAnimationFrame(rafRef.current)
    }
  }, [connected, muted, operationsPaused, state, targetEnergy])

  return (
    <div className={`relative h-full w-full transition-opacity duration-500 ${visible ? 'opacity-100' : 'opacity-0'}`}>
      <canvas ref={canvasRef} className="absolute inset-0 h-full w-full" />
      {debug ? (
        <div className="relative z-10 flex h-full w-full flex-col items-center justify-center gap-4">
          <div className="text-3xl tracking-[0.28em]">{title}</div>
          <div className="max-w-[70vw] truncate text-xs tracking-[0.24em] text-glow/60">{lastChunk || 'READY'}</div>
          <div className="max-w-[70vw] truncate text-[10px] tracking-[0.2em] text-glow/45">
            {capabilityChange || `CAPABILITIES ${capabilityCount}`}
          </div>
          <div className="max-w-[70vw] truncate text-[10px] tracking-[0.2em] text-glow/45">
            {memoryChange || `MEMORY ${memoryCount}`}
          </div>
          <div className="max-w-[70vw] truncate text-[10px] tracking-[0.2em] text-glow/45">
            {signalHint || (operationsPaused ? 'OPERATIONS PAUSED' : 'OPERATIONS RUNNING')}
          </div>
          <div className="max-w-[70vw] truncate text-[10px] tracking-[0.2em] text-glow/40">F7 HUD · F8 DEBUG · M MUTE · ESC INTERRUPT</div>
        </div>
      ) : null}
    </div>
  )
}
