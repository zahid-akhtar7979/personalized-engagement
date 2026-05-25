import { useState, useEffect, useCallback } from 'react'
import {
  getReplayStatus, startReplay, pauseReplay, resumeReplay, setReplaySpeed,
} from '../api/client'

export default function ReplayControls() {
  const [status, setStatus] = useState({ running: false, paused: false, speed: 1, processed: 0, total: 0 })
  const [speedInput, setSpeedInput] = useState(1)

  const refresh = useCallback(async () => {
    try {
      const { data } = await getReplayStatus()
      setStatus(data)
      setSpeedInput(data.speed || 1)
    } catch (_) {}
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 1000)
    return () => clearInterval(t)
  }, [refresh])

  const progress = status.total > 0 ? (status.processed / status.total) * 100 : 0

  const action = async (fn, label) => {
    try {
      await fn()
      await refresh()
    } catch (e) {
      alert(`${label} failed: ${e.message}`)
    }
  }

  return (
    <div className="max-w-xl">
      <h1 className="text-2xl font-bold mb-2">Dataset Replay Controls</h1>
      <p className="text-netflix-muted text-sm mb-6">
        Stream Retailrocket events into Kafka to simulate live customer traffic
      </p>

      <div className="bg-netflix-card rounded-xl p-6 border border-white/5 space-y-6">
        <div className="flex gap-3 flex-wrap">
          <button
            onClick={() => action(startReplay, 'Start')}
            className="px-5 py-2.5 bg-netflix-accent rounded-lg font-medium hover:bg-red-600"
          >
            ▶ Start Replay
          </button>
          <button
            onClick={() => action(pauseReplay, 'Pause')}
            className="px-5 py-2.5 bg-yellow-600/80 rounded-lg font-medium hover:bg-yellow-600"
          >
            ⏸ Pause
          </button>
          <button
            onClick={() => action(resumeReplay, 'Resume')}
            className="px-5 py-2.5 bg-green-600/80 rounded-lg font-medium hover:bg-green-600"
          >
            ⏵ Resume
          </button>
        </div>

        <div>
          <label className="text-sm text-netflix-muted block mb-2">Replay Speed: {speedInput}x</label>
          <input
            type="range"
            min="0.5"
            max="10"
            step="0.5"
            value={speedInput}
            onChange={(e) => setSpeedInput(Number(e.target.value))}
            onMouseUp={() => action(() => setReplaySpeed(speedInput), 'Speed')}
            onTouchEnd={() => action(() => setReplaySpeed(speedInput), 'Speed')}
            className="w-full accent-netflix-accent"
          />
        </div>

        <div>
          <div className="flex justify-between text-sm mb-1">
            <span className="text-netflix-muted">Progress</span>
            <span>
              {status.processed} / {status.total} events
            </span>
          </div>
          <div className="h-2 bg-black/40 rounded-full overflow-hidden">
            <div
              className="h-full bg-netflix-accent transition-all duration-300"
              style={{ width: `${progress}%` }}
            />
          </div>
        </div>

        <div className="grid grid-cols-3 gap-3 text-center text-sm">
          <div className="p-3 rounded-lg bg-black/30">
            <p className="text-netflix-muted">Status</p>
            <p className="font-semibold mt-1">
              {status.running ? (status.paused ? 'Paused' : 'Running') : 'Idle'}
            </p>
          </div>
          <div className="p-3 rounded-lg bg-black/30">
            <p className="text-netflix-muted">Speed</p>
            <p className="font-semibold mt-1">{status.speed}x</p>
          </div>
          <div className="p-3 rounded-lg bg-black/30">
            <p className="text-netflix-muted">Progress</p>
            <p className="font-semibold mt-1">{progress.toFixed(0)}%</p>
          </div>
        </div>
      </div>

      <div className="mt-6 p-4 rounded-lg bg-black/30 text-sm text-netflix-muted">
        <p className="font-medium text-white mb-2">Demo tip</p>
        <ol className="list-decimal list-inside space-y-1">
          <li>Start replay here</li>
          <li>Open Dashboard and select a user</li>
          <li>Watch recommendations update in real time</li>
          <li>Check Analytics for live ROI metrics</li>
        </ol>
      </div>
    </div>
  )
}
