import axios from 'axios'

// Empty baseURL = same origin (nginx on :3000 proxies /api/* to backend services).
function serviceBase(envValue, devPort) {
  if (envValue) return envValue
  if (typeof window !== 'undefined') return ''
  return `http://localhost:${devPort}`
}

const replay = axios.create({ baseURL: serviceBase(import.meta.env.VITE_REPLAY_URL, 8081) })
const recs = axios.create({ baseURL: serviceBase(import.meta.env.VITE_RECS_URL, 8082) })
const engagement = axios.create({ baseURL: serviceBase(import.meta.env.VITE_ENGAGEMENT_URL, 8083) })
const analytics = axios.create({ baseURL: serviceBase(import.meta.env.VITE_ANALYTICS_URL, 8084) })
const ai = axios.create({ baseURL: serviceBase(import.meta.env.VITE_AI_URL, 8085) })

export const getReplayStatus = () => replay.get('/api/replay/status')
export const startReplay = () => replay.post('/api/replay/start')
export const nextReplayBatch = () => replay.post('/api/replay/next-batch')
export const pauseReplay = () => replay.post('/api/replay/pause')
export const resumeReplay = () => replay.post('/api/replay/resume')
export const resetReplay = () => replay.post('/api/replay/reset')
export const setReplaySpeed = (speed) => replay.put('/api/replay/speed', { speed })

export const getDashboard = (userId) => engagement.get(`/api/dashboard/${userId}`)
export const getActivityFeed = () => engagement.get('/api/activity')
export const getAnalytics = () => analytics.get('/api/analytics')
export const getAnalyticsHistory = () => analytics.get('/api/analytics/history')
export const getRecommendations = (userId) => recs.get(`/api/recommendations/${userId}`)
export const getSampleUsers = () => recs.get('/api/users/sample')
export const getActiveReplayUsers = () => recs.get('/api/users/active')
export const getAIAlerts = () => ai.get('/api/ai/alerts')
export const runSQLQuery = (question) => ai.post('/api/ai/sql', { question })
export const runAIQuery = (question) => ai.post('/api/ai/query', { question })

export function getWsUrl(userId) {
  const env = import.meta.env.VITE_WS_URL
  if (env) return `${env}/ws/dashboard/${userId}`
  if (typeof window !== 'undefined') {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${window.location.host}/ws/dashboard/${userId}`
  }
  return `ws://localhost:8083/ws/dashboard/${userId}`
}
