import { useState, useEffect, useCallback } from 'react'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  BarChart, Bar, Legend,
} from 'recharts'
import { getAnalytics, getAnalyticsHistory, getAIAlerts } from '../api/client'
import { useWebSocket } from '../hooks/useWebSocket'

function MetricCard({ label, value, suffix = '%', color = 'text-white' }) {
  return (
    <div className="bg-netflix-card rounded-xl p-4 border border-white/5">
      <p className="text-xs text-netflix-muted uppercase tracking-wide">{label}</p>
      <p className={`text-2xl font-bold mt-1 ${color}`}>
        {typeof value === 'number' ? value.toFixed(1) : '—'}{suffix}
      </p>
    </div>
  )
}

export default function Analytics() {
  const [metrics, setMetrics] = useState({})
  const [history, setHistory] = useState([])
  const [alerts, setAlerts] = useState([])

  const load = useCallback(async () => {
    try {
      const [m, h, a] = await Promise.all([
        getAnalytics(),
        getAnalyticsHistory(),
        getAIAlerts(),
      ])
      setMetrics(m.data || {})
      setHistory(
        (h.data || []).slice(0, 20).reverse().map((r, i) => ({
          name: `#${i + 1}`,
          retention: r.retention_rate,
          ctr: r.ctr,
          conversion: r.conversion_rate,
          engagement: r.engagement_score,
          roi: r.roi_percentage,
        }))
      )
      setAlerts(a.data || [])
    } catch (_) {}
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 3000)
    return () => clearInterval(t)
  }, [load])

  useWebSocket(1, (msg) => {
    if (msg.type === 'analytics_update') {
      setMetrics(msg.data)
    }
  })

  const barData = [
    { name: 'Retention', value: metrics.retentionRate || 0 },
    { name: 'CTR', value: metrics.ctr || 0 },
    { name: 'Conversion', value: metrics.conversionRate || 0 },
    { name: 'ROI', value: metrics.roiPercentage || 0 },
  ]

  return (
    <div>
      <h1 className="text-2xl font-bold mb-2">Customer Retention Analytics</h1>
      <p className="text-netflix-muted text-sm mb-6">Live ROI and engagement metrics</p>

      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3 mb-8">
        <MetricCard label="Retention Rate" value={metrics.retentionRate} />
        <MetricCard label="Recommendation CTR" value={metrics.ctr} />
        <MetricCard label="Conversion Rate" value={metrics.conversionRate} />
        <MetricCard label="Engagement Score" value={metrics.engagementScore} suffix="" color="text-netflix-accent" />
        <MetricCard label="ROI" value={metrics.roiPercentage} color={metrics.roiPercentage >= 0 ? 'text-green-400' : 'text-red-400'} />
        <MetricCard label="Active Users" value={metrics.activeUsers} suffix="" />
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
        </div>
      </div>

      <div className="bg-netflix-card rounded-xl p-4 border border-white/5">
        <h3 className="font-semibold mb-3 flex items-center gap-2">
          <span>🤖</span> AI Retention Insights
        </h3>
        {alerts.length === 0 ? (
          <p className="text-netflix-muted text-sm">No alerts yet. Run replay to generate insights.</p>
        ) : (
          <div className="space-y-2">
            {alerts.slice(0, 8).map((a, i) => (
              <div key={i} className="p-3 rounded-lg bg-black/30 border-l-4 border-netflix-accent">
                <div className="flex justify-between text-sm">
                  <span className="font-medium text-netflix-accent">{a.alertType}</span>
                  <span className="text-netflix-muted">{a.category}</span>
                </div>
                <p className="text-sm mt-1">{a.suggestion}</p>
                {a.dropPercentage > 0 && (
                  <p className="text-xs text-netflix-muted mt-1">Drop: {a.dropPercentage.toFixed(1)}%</p>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="mt-4 grid grid-cols-2 md:grid-cols-4 gap-3 text-sm text-netflix-muted">
        <div>Total Users: <span className="text-white">{metrics.totalUsers ?? 0}</span></div>
        <div>Returning: <span className="text-white">{metrics.returningUsers ?? 0}</span></div>
        <div>Views: <span className="text-white">{metrics.views ?? 0}</span></div>
        <div>Revenue Gain: <span className="text-green-400">${(metrics.revenueGain ?? 0).toFixed(0)}</span></div>
      </div>
    </div>
  )
}
