import { useState, useEffect, useCallback } from 'react'
import RecommendationRow from '../components/RecommendationRow'
import { getDashboard, getSampleUsers } from '../api/client'
import { useWebSocket } from '../hooks/useWebSocket'

const emptyRecs = {
  recommendedForYou: [],
  trendingNow: [],
  becauseYouViewed: [],
  cartRecommendations: [],
}

export default function Dashboard() {
  const [users, setUsers] = useState([])
  const [userId, setUserId] = useState(null)
  const [recs, setRecs] = useState(emptyRecs)
  const [updatedAt, setUpdatedAt] = useState(null)
  const [pulse, setPulse] = useState(false)

  useEffect(() => {
    getSampleUsers()
      .then(({ data }) => {
        const list = Array.isArray(data) ? data : []
        setUsers(list)
        if (list.length > 0) {
          setUserId(list[0].userId)
        }
      })
      .catch(() => setUsers([]))
  }, [])

  const loadDashboard = useCallback(async () => {
    if (!userId) return
    try {
      const { data } = await getDashboard(userId)
      if (data?.recommendations) {
        setRecs(data.recommendations)
        setUpdatedAt(data.updatedAt)
      }
    } catch (_) {}
  }, [userId])

  useEffect(() => {
    loadDashboard()
  }, [loadDashboard])

  const onWsMessage = useCallback((msg) => {
    if (msg.type === 'dashboard_update' && msg.data?.recommendations) {
      setRecs(msg.data.recommendations)
      setUpdatedAt(msg.data.updatedAt)
      setPulse(true)
      setTimeout(() => setPulse(false), 600)
    }
  }, [])

  const { connected } = useWebSocket(userId ?? 0, onWsMessage)

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-4 mb-6">
        <div>
          <h1 className="text-2xl font-bold">For You</h1>
          <p className="text-netflix-muted text-sm mt-1">
            Real-time personalized recommendations (Retailrocket users)
            {updatedAt && (
              <span className="ml-2">
                · Updated {new Date(updatedAt).toLocaleTimeString()}
              </span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <span
            className={`text-xs px-2 py-1 rounded-full ${
              connected ? 'bg-green-500/20 text-green-400' : 'bg-yellow-500/20 text-yellow-400'
            }`}
          >
            {connected ? '● Live' : '○ Reconnecting'}
          </span>
          <select
            value={userId ?? ''}
            onChange={(e) => setUserId(Number(e.target.value))}
            disabled={users.length === 0}
            className="bg-netflix-card border border-white/10 rounded px-3 py-2 text-sm max-w-[220px]"
          >
            {users.length === 0 ? (
              <option value="">Loading users…</option>
            ) : (
              users.map((u) => (
                <option key={u.userId} value={u.userId}>
                  {u.username} ({u.eventCount} events)
                </option>
              ))
            )}
          </select>
        </div>
      </div>

      <div className={`transition-all duration-300 ${pulse ? 'ring-2 ring-netflix-accent/50 rounded-xl p-2' : ''}`}>
        <RecommendationRow title="Recommended For You" items={recs.recommendedForYou} />
        <RecommendationRow title="Trending Now" items={recs.trendingNow} />
        <RecommendationRow title="Because You Viewed" items={recs.becauseYouViewed} />
        <RecommendationRow title="Cart Recommendations" items={recs.cartRecommendations} />
      </div>
    </div>
  )
}
