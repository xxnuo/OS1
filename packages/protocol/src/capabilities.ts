export type Capability = {
  id: string
  name: string
  version: string
  inputSchema: Record<string, unknown>
  outputSchema: Record<string, unknown>
  sideEffects: string[]
  source: string
  signature: string
  addedAt: string
}
