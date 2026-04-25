export interface Message {
  role: 'user' | 'assistant' | 'toolResult'
  content?: string | Array<{ type: string; text?: string; thinking?: string; name?: string; arguments?: any; url?: string; data?: string; mimeType?: string }>
  toolResults?: Array<{ toolCallId: string; toolName: string; details: any; isError: boolean }>
  timestamp?: number
  usage?: { input?: number; output?: number; totalTokens?: number }
  stopReason?: string
  errorMessage?: string
}

export interface Entry {
  id: string
  time: number
  invocationId: string
  branch: string
  author: string
  message?: Message
  errorCode: string
  errorMessage: string
}

export interface Session {
  id: string
  appName: string
  userId: string
  lastUpdateTime: number
  entries: Entry[]
}

export interface RuntimeConfig {
  backendUrl: string
}

let base = ''

export async function initApi(): Promise<void> {
  const res = await fetch('./assets/config/runtime-config.json')
  const cfg: RuntimeConfig = res.ok ? await res.json() : { backendUrl: '' }
  base = cfg.backendUrl || ''
}

async function api<T>(method: string, path: string, body?: unknown): Promise<T> {
  const opts: RequestInit = { method, headers: {} }
  if (body) {
    (opts.headers as Record<string, string>)['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(`${base}${path}`, opts)
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`${res.status}: ${text}`)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const listApps = (): Promise<string[]> => api('GET', '/list-apps')

export const listSessions = (app: string, user: string): Promise<Session[]> =>
  api('GET', `/apps/${encodeURIComponent(app)}/users/${encodeURIComponent(user)}/sessions`)

export const getSession = (app: string, user: string, sid: string): Promise<Session> =>
  api('GET', `/apps/${encodeURIComponent(app)}/users/${encodeURIComponent(user)}/sessions/${encodeURIComponent(sid)}`)

export const createSession = (app: string, user: string, sid: string, entries: Entry[] = []): Promise<Session> =>
  api('POST', `/apps/${encodeURIComponent(app)}/users/${encodeURIComponent(user)}/sessions/${encodeURIComponent(sid)}`, { entries })

export const deleteSession = (app: string, user: string, sid: string): Promise<void> =>
  api('DELETE', `/apps/${encodeURIComponent(app)}/users/${encodeURIComponent(user)}/sessions/${encodeURIComponent(sid)}`)

export interface SSEEvent {
  type: string
  Delta?: string
  ContentIndex?: number
  ToolCallID?: string
  ToolName?: string
  Args?: any
  Result?: any
  IsError?: boolean
  Message?: Message
  m?: { author?: string; branch?: string; invocationId?: string }
}

export function runAgentSSE(
  app: string,
  user: string,
  sid: string,
  text: string,
  onEvent: (ev: SSEEvent) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const payload = {
      appName: app,
      userId: user,
      sessionId: sid,
      newMessage: { role: 'user', content: text },
      streaming: true,
    }
    fetch(`${base}/run_sse`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }).then(async (res) => {
      if (!res.ok) {
        reject(new Error(`${res.status}: ${await res.text()}`))
        return
      }
      const reader = res.body!.getReader()
      const decoder = new TextDecoder()
      let buf = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() || ''
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.slice(6)
            if (data === '[DONE]') continue
            try {
              const ev: SSEEvent = JSON.parse(data)
              onEvent(ev)
            } catch {}
          }
        }
      }
      resolve()
    }).catch(reject)
  })
}
