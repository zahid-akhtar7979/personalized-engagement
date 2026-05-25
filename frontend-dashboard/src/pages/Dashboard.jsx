import { useState, useEffect, useCallback } from 'react'
import RecommendationRow from '../components/RecommendationRow'
import { getDashboard } from '../api/client'
import { useWebSocket } from '../hooks/useWebSocket'

const USERS = [1, 2, 3, 4, 5]

const emptyRecs = {
  recommendedForYou: [],
  trendingNow: [],
  becauseYouViewed: [],
  cartRecommendations: [],
}

export default function Dashboard() {
  const [userId, setUserId] = useState(1)
  const [recs, setRecs] = useState(emptyRecs)
  const [updatedAt, setUpdatedAt] = useState(null)
  const [pulse, setPulse] = useState(false)

  const loadDashboard = useCallback(async () => {
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

  const { connected } = useWebSocket(userId, onWsMessage)

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-4 mb-6">
        <div>
          <h1 className="text-2xl font-bold">For You</h1>
          <p className="text-netflix-muted text-sm mt-1">
            Real-time personalized recommendations
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
            value={userId}
            onChange={(e) => setUserId(Number(e.target.value))}
            className="bg-netflix-card border border-white/10 rounded px-3 py-2 text-sm"
          >
            {USERS.map((id) => (
              <option key={id} value={id}>
                User {id}
              </option>
            ))}
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
