import type { Message } from '../api'

function renderContent(content: Message['content']) {
  if (!content) return null
  if (typeof content === 'string') return <p className="whitespace-pre-wrap">{content}</p>
  if (!Array.isArray(content)) return <pre className="text-xs">{JSON.stringify(content, null, 2)}</pre>
  return content.map((c, i) => {
    if (!c) return null
    switch (c.type) {
      case 'text':
        return <p key={i} className="whitespace-pre-wrap">{c.text}</p>
      case 'thinking':
        return (
          <div key={i} className="text-gray-400 text-sm italic border-l-2 border-gray-600 pl-2 my-1">
            💭 {c.thinking}
          </div>
        )
      case 'toolCall':
        return (
          <div key={i} className="bg-gray-700 rounded px-3 py-2 text-xs font-mono my-1">
            <span className="text-yellow-400">🔧 {c.name}</span>
            <pre className="mt-1 text-gray-300 overflow-x-auto">{JSON.stringify(c.arguments, null, 2)}</pre>
          </div>
        )
      case 'image':
        return c.url ? <img key={i} src={c.url} alt="" className="max-w-full rounded my-1" /> : <div key={i}>[image]</div>
      default:
        return <pre key={i} className="text-xs">{JSON.stringify(c, null, 2)}</pre>
    }
  })
}

interface Props {
  message: Message
}

export default function MessageItem({ message }: Props) {
  const isUser = message.role === 'user'
  const isTool = message.role === 'toolResult'

  return (
    <div className={`flex gap-3 ${isUser ? 'flex-row-reverse' : ''}`}>
      <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm flex-shrink-0 ${
        isUser ? 'bg-indigo-600' : isTool ? 'bg-yellow-600' : 'bg-emerald-600'
      }`}>
        {isUser ? '👤' : isTool ? '🔧' : '🤖'}
      </div>
      <div className={`max-w-[80%] rounded-lg px-4 py-2 ${
        isUser ? 'bg-indigo-600 text-white' : 'bg-gray-800 text-gray-100'
      }`}>
        <div className="text-xs font-semibold text-gray-400 mb-1">
          {isUser ? 'You' : isTool ? 'Tool' : 'Assistant'}
        </div>
        <div className="text-sm leading-relaxed">
          {renderContent(message.content)}
        </div>
        {message.toolResults && message.toolResults.length > 0 && (
          <div className="mt-2 space-y-1">
            {message.toolResults.map((tr, i) => (
              <div key={i} className={`text-xs rounded px-2 py-1 ${tr.isError ? 'bg-red-900/50 text-red-300' : 'bg-green-900/50 text-green-300'}`}>
                <strong>{tr.toolName}</strong>: {typeof tr.details === 'string' ? tr.details : JSON.stringify(tr.details)}
              </div>
            ))}
          </div>
        )}
        {message.usage && (
          <div className="mt-1 text-[10px] text-gray-500">
            {message.usage.input ?? 0} in / {message.usage.output ?? 0} out tokens
          </div>
        )}
      </div>
    </div>
  )
}
