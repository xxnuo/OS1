export type MemoryItem = {
  id: string
  kind: string
  content: string
  tags?: string[]
  confidence?: number
  source: string
  createdAt: string
  updatedAt: string
  locked: boolean
  deleted: boolean
}

