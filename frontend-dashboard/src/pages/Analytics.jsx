import { useState, useEffect, useCallback } from 'react'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  BarChart, Bar, Legend,
} from 'recharts'
import { getAnalytics, getAnalyticsHistory, getAIAlerts } from '../api/client'
import { useWebSocket } from '../hooks/useWebSocket'

function MetricCard({ label, value, suffix = '%', color = 'text-white', hint }) {
  return (
    <div className="bg-netflix-card rounded-xl p-4 border border-white/5">
      <p className="text-xs text-netflix-muted uppercase tracking-wide">{label}</p>
      <p className={`text-2xl font-bold mt-1 ${color}`}>
        {typeof value === 'number' ? value.toFixed(1) : '—'}{suffix}
      </p>
      {hint && <p className="text-[10px] text-netflix-muted mt-1 leading-tight">{hint}</p>}
    </div>
  )
}

function num(row, ...keys) {
  for (const k of keys) {
    if (row[k] != null && !Number.isNaN(Number(row[k]))) return Number(row[k])
  }
  return 0
}

function normalizeHistory(rows) {
  return (rows || []).slice(-20).map((r, i) => ({
    name: `#${i + 1}`,
    retention: num(r, 'retentionRate', 'RetentionRate', 'retention_rate'),
    ctr: num(r, 'ctr', 'CTR'),
    conversion: num(r, 'conversionRate', 'ConversionRate', 'conversion_rate'),
    engagement: num(r, 'engagementScore', 'EngagementScore', 'engagement_score'),
    roi: num(r, 'roiPercentage', 'ROIPercentage', 'roi_percentage'),
  }))
}

export default function Analytics() {
  const [metrics, setMetrics] = useState({})
  const [history, setHistory] = useState([])
  const [alerts, setAlerts] = useState([])
  const [loadError, setLoadError] = useState('')

  const load = useCallback(async () => {
    try {
      const [m, h, a] = await Promise.all([
        getAnalytics(),
        getAnalyticsHistory(),
        getAIAlerts(),
      ])
      setMetrics(m.data || {})
      setHistory(normalizeHistory(h.data))
      setAlerts(a.data || [])
      setLoadError('')
    } catch (_) {
      setLoadError('Could not load analytics. Check retention-analytics and ai-insights services.')
    }
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [load])

  useWebSocket(1, (msg) => {
    if (msg.type === 'analytics_update' && msg.data) {
      setMetrics(msg.data)
    }
  })

  const barData = [
    { name: 'Retention', value: metrics.retentionRate || 0 },
    { name: 'CTR', value: metrics.ctr || 0 },
    { name: 'Conversion', value: metrics.conversionRate || 0 },
    { name: 'ROI', value: metrics.roiPercentage || 0 },
  ]

  const hasData = (metrics.views ?? 0) > 0 || (metrics.totalUsers ?? 0) > 0

  return (
    <div>
      <h1 className="text-2xl font-bold mb-2">Customer Retention Analytics</h1>
      <p className="text-netflix-muted text-sm mb-6">
        Live ROI and engagement metrics from PostgreSQL event data
        {metrics.updatedAt && (
          <span className="ml-2">· Updated {new Date(metrics.updatedAt).toLocaleTimeString()}</span>
        )}
      </p>

      {loadError && <p className="text-yellow-400/90 text-sm mb-4">{loadError}</p>}
      {!hasData && !loadError && (
        <p className="text-netflix-muted text-sm mb-4">
          No event data yet. Import the dataset or run Replay to populate metrics.
        </p>
      )}

      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3 mb-8">
        <MetricCard
          label="Retention Rate"
          value={metrics.retentionRate}
          hint="Users with 2+ events ÷ all users"
        />
        <MetricCard
          label="Recommendation CTR"
          value={metrics.ctr}
          hint="(Add-to-cart + purchases) ÷ product views"
        />
        <MetricCard
          label="Conversion Rate"
          value={metrics.conversionRate}
          hint="Purchases ÷ product views"
        />
        <MetricCard
          label="Engagement Index"
          value={metrics.engagementScore}
          suffix="/100"
          color="text-netflix-accent"
          hint="40% retention + 30% CTR + 30% conversion (scaled)"
        />
        <MetricCard
          label="Modeled ROI"
          value={metrics.roiPercentage}
          color={metrics.roiPercentage >= 0 ? 'text-green-400' : 'text-red-400'}
          hint="8% of GMV attributed to PEP minus platform cost"
        />
        <MetricCard
          label="Active Users"
          value={metrics.activeUsers}
          suffix=""
          hint="Distinct users in event history"
        />
      </div>

      <div className="grid lg:grid-cols-2 gap-6 mb-8">
        <div className="bg-netflix-card rounded-xl p-4 border border-white/5">
          <h3 className="font-semibold mb-4">Metrics Overview</h3>
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={barData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#333" />
              <XAxis dataKey="name" stroke="#888" fontSize={12} />
              <YAxis stroke="#888" fontSize={12} />
              <Tooltip contentStyle={{ background: '#1f1f1f', border: 'none' }} />
              <Bar dataKey="value" fill="#e50914" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="bg-netflix-card rounded-xl p-4 border border-white/5">
          <h3 className="font-semibold mb-4">Engagement Trend</h3>
          {history.length === 0 ? (
            <p className="text-netflix-muted text-sm py-16 text-center">History builds as metrics are recorded over time.</p>
          ) : (
            <ResponsiveContainer width="100%" height={220}>
              <LineChart data={history}>
                <CartesianGrid strokeDasharray="3 3" stroke="#333" />
                <XAxis dataKey="name" stroke="#888" fontSize={10} />
                <YAxis stroke="#888" fontSize={12} />
                <Tooltip contentStyle={{ background: '#1f1f1f', border: 'none' }} />
                <Legend />
                <Line type="monotone" dataKey="engagement" stroke="#e50914" dot={false} />
                <Line type="monotone" dataKey="retention" stroke="#46d369" dot={false} />
              </LineChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

      <div className="bg-netflix-card rounded-xl p-4 border border-white/5">
        <h3 className="font-semibold mb-3 flex items-center gap-2">
          <span>🤖</span> AI Retention Insights
        </h3>
        {alerts.length === 0 ? (
          <p className="text-netflix-muted text-sm">No alerts yet. Run replay or wait for enough event data in PostgreSQL.</p>
        ) : (
          <div className="space-y-2">
            {alerts.slice(0, 8).map((a, i) => (
              <div key={`${a.alertType}-${a.category}-${i}`} className="p-3 rounded-lg bg-black/30 border-l-4 border-netflix-accent">
                <div className="flex justify-between text-sm">
                  <span className="font-medium text-netflix-accent">{a.alertType}</span>
                  <span className="text-netflix-muted">{a.category}</span>
                </div>
                <p className="text-sm mt-1">{a.suggestion}</p>
                {a.dropPercentage > 0 && (
                  <p className="text-xs text-netflix-muted mt-1">Impact: {a.dropPercentage.toFixed(1)}%</p>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="mt-4 grid grid-cols-2 md:grid-cols-4 gap-3 text-sm text-netflix-muted">
        <div>Total Users: <span className="text-white">{metrics.totalUsers?.toLocaleString() ?? 0}</span></div>
        <div>Returning: <span className="text-white">{metrics.returningUsers?.toLocaleString() ?? 0}</span></div>
        <div>Views: <span className="text-white">{metrics.views?.toLocaleString() ?? 0}</span></div>
        <div>
          Attributed revenue:{' '}
          <span className="text-green-400">${(metrics.revenueGain ?? 0).toLocaleString(undefined, { maximumFractionDigits: 0 })}</span>
          <span className="block text-[10px]">Platform cost: ${(metrics.systemCost ?? 0).toLocaleString(undefined, { maximumFractionDigits: 0 })}</span>
        </div>
      </div>
    </div>
  )
}
