import { useState, useEffect, useCallback } from 'react'
import RecommendationRow from '../components/RecommendationRow'
import { getDashboard, getRecommendations, getActiveReplayUsers } from '../api/client'
import { useWebSocket } from '../hooks/useWebSocket'

const emptyRecs = {
  recommendedForYou: [],
  trendingNow: [],
  becauseYouViewed: [],
  cartRecommendations: [],
}

function normalizeRecs(data) {
  if (!data) return emptyRecs
  return {
    recommendedForYou: data.recommendedForYou ?? [],
    trendingNow: data.trendingNow ?? [],
    becauseYouViewed: data.becauseYouViewed ?? [],
    cartRecommendations: data.cartRecommendations ?? [],
  }
}

function hasRecs(recs) {
  return (
    recs.recommendedForYou.length > 0 ||
    recs.trendingNow.length > 0 ||
    recs.becauseYouViewed.length > 0 ||
    recs.cartRecommendations.length > 0
  )
}

export default function Dashboard() {
  const [users, setUsers] = useState([])
  const [userId, setUserId] = useState(null)
  const [recs, setRecs] = useState(emptyRecs)
  const [updatedAt, setUpdatedAt] = useState(null)
  const [pulse, setPulse] = useState(false)
  const [hint, setHint] = useState('')

  useEffect(() => {
    let cancelled = false

    async function loadUsers() {
      try {
        const { data } = await getActiveReplayUsers()
        if (!cancelled && Array.isArray(data) && data.length > 0) {
          setUsers(data)
          setUserId((prev) => prev ?? data[0].userId)
          setHint('')
          return
        }
      } catch (_) {}

      try {
        const { data } = await getSampleUsers()
        if (!cancelled && Array.isArray(data) && data.length > 0) {
          setUsers(data)
          setUserId((prev) => prev ?? data[0].userId)
          setHint('')
          return
        }
      } catch (_) {}

      if (!cancelled) {
        setUsers([])
        setHint('Could not load users. Ensure recommendation-service is running.')
      }
    }

    loadUsers()
    const t = setInterval(loadUsers, 5000)
    return () => {
      cancelled = true
      clearInterval(t)
    }
  }, [])

  const loadDashboard = useCallback(async () => {
    if (!userId) return
    try {
      // Primary: live recommendation engine (includes DB history + Kafka replay)
      const { data: liveRecs } = await getRecommendations(userId)
      const normalized = normalizeRecs(liveRecs)
      setRecs(normalized)

      if (hasRecs(normalized)) {
        setHint('')
        setUpdatedAt(new Date().toISOString())
      } else {
        setHint('No recommendations yet for this user. Run Replay (100 events) or pick a user from the live list.')
      }

      // Secondary: cached dashboard timestamp from orchestrator
      try {
        const { data: cached } = await getDashboard(userId)
        if (cached?.updatedAt) setUpdatedAt(cached.updatedAt)
        const cachedRecs = normalizeRecs(cached?.recommendations)
        if (hasRecs(cachedRecs) && !hasRecs(normalized)) {
          setRecs(cachedRecs)
          setHint('')
        }
      } catch (_) {}
    } catch (_) {
      setHint('Could not load recommendations. Check that recommendation-service is running.')
    }
  }, [userId])

  useEffect(() => {
    loadDashboard()
    const t = setInterval(loadDashboard, 3000)
    return () => clearInterval(t)
  }, [loadDashboard])

  const onWsMessage = useCallback((msg) => {
    if (msg.type === 'dashboard_update' && msg.data?.recommendations) {
      const normalized = normalizeRecs(msg.data.recommendations)
      if (hasRecs(normalized)) {
        setRecs(normalized)
        setUpdatedAt(msg.data.updatedAt)
        setHint('')
        setPulse(true)
        setTimeout(() => setPulse(false), 600)
      }
    }
  }, [])

  const { connected } = useWebSocket(userId ?? 0, onWsMessage)

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
          {hint && <p className="text-yellow-400/90 text-xs mt-2">{hint}</p>}
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
            className="bg-netflix-card border border-white/10 rounded px-3 py-2 text-sm max-w-[240px]"
          >
            {users.length === 0 ? (
              <option value="">{hint || 'Loading users…'}</option>
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
