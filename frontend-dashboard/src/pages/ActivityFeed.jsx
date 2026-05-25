import { useState, useEffect, useCallback } from 'react'
import { getActivityFeed } from '../api/client'
import { useWebSocket } from '../hooks/useWebSocket'

export default function ActivityFeed() {
  const [feed, setFeed] = useState([])

  const load = useCallback(async () => {
    try {
      const { data } = await getActivityFeed()
      setFeed(data || [])
    } catch (_) {}
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 2000)
    return () => clearInterval(t)
  }, [load])

  useWebSocket(1, (msg) => {
    if (msg.type === 'activity' && msg.data?.message) {
      setFeed((prev) => [
        {
          id: Date.now().toString(),
          message: msg.data.message,
          userId: msg.data.userId,
          timestamp: new Date().toISOString(),
          type: 'event',
        },
        ...prev.slice(0, 99),
      ])
    }
  })

  const icon = (type) => {
    switch (type) {
      case 'recommendation': return '🎯'
      case 'dashboard': return '📊'
      case 'analytics': return '📈'
      default: return '👤'
    }
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-2">Live Activity Feed</h1>
      <p className="text-netflix-muted text-sm mb-6">Real-time customer engagement events</p>

      <div className="space-y-2 max-h-[70vh] overflow-y-auto">
        {feed.length === 0 && (
          <p className="text-netflix-muted">Start dataset replay to see live activity...</p>
        )}
        {feed.map((item) => (
          <div
            key={item.id}
            className="flex items-start gap-3 p-4 rounded-lg bg-netflix-card border border-white/5 animate-slide-up"
          >
            <span className="text-xl">{icon(item.type)}</span>
            <div className="flex-1 min-w-0">
              <p className="text-sm">{item.message}</p>
              <p className="text-xs text-netflix-muted mt-1">
                {new Date(item.timestamp).toLocaleString()}
                {item.userId ? ` · User ${item.userId}` : ''}
              </p>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
