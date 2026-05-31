import axios from 'axios'

const replay = axios.create({ baseURL: import.meta.env.VITE_REPLAY_URL || 'http://localhost:8081' })
const recs = axios.create({ baseURL: import.meta.env.VITE_RECS_URL || 'http://localhost:8082' })
const engagement = axios.create({ baseURL: import.meta.env.VITE_ENGAGEMENT_URL || 'http://localhost:8083' })
const analytics = axios.create({ baseURL: import.meta.env.VITE_ANALYTICS_URL || 'http://localhost:8084' })
const ai = axios.create({ baseURL: import.meta.env.VITE_AI_URL || 'http://localhost:8085' })

export const getReplayStatus = () => replay.get('/api/replay/status')
export const startReplay = () => replay.post('/api/replay/start')
export const pauseReplay = () => replay.post('/api/replay/pause')
export const resumeReplay = () => replay.post('/api/replay/resume')
export const setReplaySpeed = (speed) => replay.put('/api/replay/speed', { speed })

export const getDashboard = (userId) => engagement.get(`/api/dashboard/${userId}`)
export const getActivityFeed = () => engagement.get('/api/activity')
export const getAnalytics = () => analytics.get('/api/analytics')
export const getAnalyticsHistory = () => analytics.get('/api/analytics/history')
export const getRecommendations = (userId) => recs.get(`/api/recommendations/${userId}`)
export const getSampleUsers = () => recs.get('/api/users/sample')
export const getAIAlerts = () => ai.get('/api/ai/alerts')
export const runSQLQuery = (question) => ai.post('/api/ai/sql', { question })
export const runAIQuery = (question) => ai.post('/api/ai/query', { question })

export function getWsUrl(userId) {
  const base = import.meta.env.VITE_WS_URL || 'ws://localhost:8083'
  return `${base}/ws/dashboard/${userId}`
}
