import { useState, useRef, useEffect } from 'react'
import MessageItem from './MessageItem'
import type { Message, Session } from '../api'

interface Props {
  app: string
  session: Session | null
  messages: Message[]
  onSend: (text: string) => void
  loading: boolean
}

export default function Chat({ app, session, messages, onSend, loading }: Props) {
  const [input, setInput] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!input.trim() || loading) return
    onSend(input.trim())
    setInput('')
  }

  return (
    <div className="flex-1 flex flex-col min-w-0 bg-gray-900">
      <div className="h-14 flex items-center px-6 border-b border-gray-700 bg-gray-800">
        <span className="text-sm font-medium text-gray-300">
          {app ? `${app} / ${session?.id?.slice(0, 8) || '...'}` : 'Select an app'}
        </span>
      </div>
      <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4">
        {messages?.length === 0 && (
          <div className="flex items-center justify-center h-full text-gray-500 text-sm">
            Start a conversation
          </div>
        )}
        {messages?.map((m, i) => (
          <MessageItem key={i} message={m} />
        ))}
        <div ref={bottomRef} />
      </div>
      <form className="px-6 py-4 border-t border-gray-700 bg-gray-800" onSubmit={handleSubmit}>
        <div className="flex gap-2">
          <input
            value={input}
            onChange={e => setInput(e.target.value)}
            placeholder={session ? 'Type a message...' : 'Select a session first'}
            disabled={!session || loading}
            className="flex-1 bg-gray-700 border border-gray-600 rounded-lg px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 disabled:opacity-50"
          />
          <button
            type="submit"
            disabled={!session || loading || !input.trim()}
            className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 disabled:hover:bg-indigo-600 text-white px-4 py-2 rounded-lg text-sm font-medium transition"
          >
            {loading ? '...' : 'Send'}
          </button>
        </div>
      </form>
    </div>
  )
}
