type HudState = 'idle' | 'listening' | 'thinking' | 'speaking'

type Props = {
  state: HudState
  muted: boolean
  title: string
  visible: boolean
  connected: boolean
  lastChunk: string
}

const sizeByState: Record<HudState, string> = {
  idle: 'h-40 w-40',
  listening: 'h-52 w-52',
  thinking: 'h-60 w-60',
  speaking: 'h-64 w-64'
}

const glowByState: Record<HudState, string> = {
  idle: 'shadow-[0_0_70px_rgba(246,178,107,0.25)]',
  listening: 'shadow-[0_0_100px_rgba(255,226,179,0.5)]',
  thinking: 'shadow-[0_0_120px_rgba(255,196,120,0.55)]',
  speaking: 'shadow-[0_0_150px_rgba(255,175,88,0.75)]'
}

export function SamanthaHud({ state, muted, title, visible, connected, lastChunk }: Props) {
  return (
    <div className={`relative flex h-full w-full flex-col items-center justify-center gap-8 transition-opacity duration-500 ${visible ? 'opacity-100' : 'opacity-0'}`}>
      <div
        className={`rounded-full border border-ring/60 bg-gradient-to-br from-ring/35 via-ring/15 to-transparent transition-all duration-700 ${sizeByState[state]} ${glowByState[state]} ${muted || !connected ? '' : 'animate-pulseRing'}`}
      />
      <div className="text-3xl tracking-[0.28em]">{title}</div>
      <div className="max-w-[70vw] truncate text-xs tracking-[0.24em] text-glow/60">{lastChunk || 'READY'}</div>
      <div className="text-xs tracking-[0.32em] text-glow/40">F7 HUD · M MUTE · ESC INTERRUPT</div>
    </div>
  )
}
