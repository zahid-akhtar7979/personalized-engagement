import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import ActivityFeed from './pages/ActivityFeed'
import Analytics from './pages/Analytics'
import SQLAssistant from './pages/SQLAssistant'
import VoiceSQLAssistant from './pages/VoiceSQLAssistant'
import ReplayControls from './pages/ReplayControls'

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/activity" element={<ActivityFeed />} />
        <Route path="/analytics" element={<Analytics />} />
        <Route path="/sql" element={<SQLAssistant />} />
        <Route path="/voice-sql" element={<VoiceSQLAssistant />} />
        <Route path="/replay" element={<ReplayControls />} />
      </Routes>
    </Layout>
  )
}
