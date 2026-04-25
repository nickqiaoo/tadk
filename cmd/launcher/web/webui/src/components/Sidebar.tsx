import type { Session } from '../api'

interface Props {
  apps: string[]
  currentApp: string
  onSelectApp: (app: string) => void
  sessions: Session[]
  currentSession: Session | null
  onSelectSession: (s: Session) => void
  onCreateSession: () => void
  onDeleteSession: (sid: string) => void
}

export default function Sidebar({ apps, currentApp, onSelectApp, sessions, currentSession, onSelectSession, onCreateSession, onDeleteSession }: Props) {
  return (
    <aside className="w-64 flex-shrink-0 bg-gray-800 border-r border-gray-700 flex flex-col">
      <div className="p-4 border-b border-gray-700">
        <h1 className="text-xl font-bold text-indigo-400 tracking-wider">TADK</h1>
      </div>
      <div className="p-4 border-b border-gray-700">
        <label className="block text-xs font-semibold text-gray-400 uppercase mb-2">App</label>
        <select
          className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
          value={currentApp}
          onChange={e => onSelectApp(e.target.value)}
        >
          <option value="">Select app...</option>
          {apps.map(a => <option key={a} value={a}>{a}</option>)}
        </select>
      </div>
      {currentApp && (
        <div className="flex-1 flex flex-col min-h-0">
          <div className="p-4 flex items-center justify-between border-b border-gray-700">
            <span className="text-xs font-semibold text-gray-400 uppercase">Sessions</span>
            <button
              onClick={onCreateSession}
              className="text-xs bg-indigo-600 hover:bg-indigo-500 text-white px-2 py-1 rounded transition"
            >
              + New
            </button>
          </div>
          <ul className="flex-1 overflow-y-auto">
            {sessions?.map(s => (
              <li
                key={s.id}
                onClick={() => onSelectSession(s)}
                className={`flex items-center justify-between px-4 py-2 cursor-pointer text-sm border-b border-gray-700/50 transition ${
                  currentSession?.id === s.id ? 'bg-gray-700 text-white' : 'text-gray-300 hover:bg-gray-700/50'
                }`}
              >
                <span className="font-mono truncate">{s.id.slice(0, 8)}</span>
                <button
                  onClick={e => { e.stopPropagation(); onDeleteSession(s.id) }}
                  className="text-gray-500 hover:text-red-400 ml-2 text-lg leading-none"
                  title="Delete"
                >
                  ×
                </button>
              </li>
            ))}
            {sessions.length === 0 && (
              <li className="px-4 py-3 text-sm text-gray-500">No sessions</li>
            )}
          </ul>
        </div>
      )}
    </aside>
  )
}
