export type UserSignalType = 'window' | 'input' | 'file' | 'clipboard'

export type WindowSignal = {
  app: string
  windowTitle?: string
}

export type InputSignal = {
  active: boolean
  lastActivity: string
}

export type FileSignal = {
  path: string
  eventType: string
  timestamp: string
}

export type ClipboardSignal = {
  timestamp: string
}

export type UserSignalBatch = {
  signalCount: number
  from: string
  to: string
  window?: WindowSignal
  input?: InputSignal
  fileChanges?: FileSignal[]
  clipboardChanged: boolean
}
