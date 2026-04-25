import { useEffect, useState } from 'react'
import { initApi, listApps, listSessions, createSession, deleteSession, runAgentSSE, type Message, type Session } from './api'
import Sidebar from './components/Sidebar'
import Chat from './components/Chat'

const DEFAULT_USER = 'user'

function App() {
  const [apps, setApps] = useState<string[]>([])
  const [currentApp, setCurrentApp] = useState('')
  const [sessions, setSessions] = useState<Session[]>([])
  const [currentSession, setCurrentSession] = useState<Session | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    initApi()
      .then(() => listApps())
      .then(data => setApps(Array.isArray(data) ? data : []))
      .catch(() => setApps([]))
  }, [])

  useEffect(() => {
    if (!currentApp) return
    refreshSessions()
  }, [currentApp])

  const refreshSessions = () => {
    listSessions(currentApp, DEFAULT_USER)
      .then(data => setSessions(Array.isArray(data) ? data : []))
      .catch(() => setSessions([]))
  }

  const handleCreateSession = async () => {
    const id = crypto.randomUUID()
    const s = await createSession(currentApp, DEFAULT_USER, id)
    refreshSessions()
    setCurrentSession(s)
    setMessages(s.entries?.filter(e => e.message).map(e => e.message!) || [])
  }

  const handleDeleteSession = async (sid: string) => {
    await deleteSession(currentApp, DEFAULT_USER, sid)
    refreshSessions()
    if (currentSession?.id === sid) {
      setCurrentSession(null)
      setMessages([])
    }
  }

  const handleSelectSession = (s: Session) => {
    setCurrentSession(s)
    setMessages(s.entries?.filter(e => e.message).map(e => e.message!) || [])
  }

  const handleSend = async (text: string) => {
    if (!currentApp || !currentSession) return
    const userMsg: Message = { role: 'user', content: text, timestamp: Date.now() }
    setMessages(prev => [...prev, userMsg])
    setLoading(true)

    let assistantMsg: Message = { role: 'assistant', content: [{ type: 'text', text: '' }], timestamp: Date.now() }
    setMessages(prev => [...prev, assistantMsg])

    try {
      await runAgentSSE(currentApp, DEFAULT_USER, currentSession.id, text, (ev) => {
        if (ev.Delta !== undefined) {
          const content = Array.isArray(assistantMsg.content) ? assistantMsg.content : [{ type: 'text', text: '' }]
          content[0] = { ...content[0], text: (content[0].text || '') + (ev.Delta || '') }
          assistantMsg = { ...assistantMsg, content }
          setMessages(prev => {
            const copy = [...prev]
            copy[copy.length - 1] = assistantMsg
            return copy
          })
        } else if (ev.Message !== undefined && ev.Message !== null && ev.Message.role === 'assistant' && ev.Message.content !== null) {
          assistantMsg = ev.Message
          setMessages(prev => {
            const copy = [...prev]
            copy[copy.length - 1] = assistantMsg
            return copy
          })
        } else if (ev.ToolCallID !== undefined && ev.Args !== undefined) {
          setMessages(prev => [...prev, {
            role: 'toolResult',
            content: [{ type: 'text', text: `🔧 Running ${ev.ToolName}...` }],
            timestamp: Date.now(),
          }])
        } else if (ev.ToolCallID !== undefined && ev.Result !== undefined) {
          setMessages(prev => [...prev, {
            role: 'toolResult',
            content: [{ type: 'text', text: String(ev.Result ?? '') }],
            toolResults: [{ toolCallId: ev.ToolCallID || '', toolName: ev.ToolName || '', details: ev.Result, isError: !!ev.IsError }],
            timestamp: Date.now(),
          }])
        }
      })
    } catch (e: any) {
      setMessages(prev => [...prev, {
        role: 'assistant',
        content: [{ type: 'text', text: `Error: ${e.message}` }],
        timestamp: Date.now(),
      }])
    } finally {
      setLoading(false)
      refreshSessions()
    }
  }

  return (
    <div className="flex h-screen w-screen bg-gray-900 text-gray-100 overflow-hidden">
      <Sidebar
        apps={apps}
        currentApp={currentApp}
        onSelectApp={setCurrentApp}
        sessions={sessions}
        currentSession={currentSession}
        onSelectSession={handleSelectSession}
        onCreateSession={handleCreateSession}
        onDeleteSession={handleDeleteSession}
      />
      <Chat
        app={currentApp}
        session={currentSession}
        messages={messages}
        onSend={handleSend}
        loading={loading}
      />
    </div>
  )
}

export default App
