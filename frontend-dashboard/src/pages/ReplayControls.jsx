import { useState, useEffect, useCallback } from 'react'
import {
  getReplayStatus,
  startReplay,
  nextReplayBatch,
  pauseReplay,
  resumeReplay,
  resetReplay,
  setReplaySpeed,
} from '../api/client'

export default function ReplayControls() {
  const [status, setStatus] = useState({
    running: false,
    paused: false,
    speed: 1,
    processed: 0,
    total: 0,
    batchSize: 100,
    hasMore: true,
  })
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
  const batchSize = status.batchSize || 100
  const atEnd = status.processed >= status.total && status.total > 0

  const action = async (fn, label) => {
    try {
      await fn()
      await refresh()
    } catch (e) {
      const msg = e.response?.data?.error || e.message
      alert(`${label} failed: ${msg}`)
    }
  }

  return (
    <div className="max-w-xl">
      <h1 className="text-2xl font-bold mb-2">Dataset Replay Controls</h1>
      <p className="text-netflix-muted text-sm mb-6">
        Stream Retailrocket events into Kafka in batches of {batchSize} (not all {status.total?.toLocaleString()} at once)
      </p>

      <div className="bg-netflix-card rounded-xl p-6 border border-white/5 space-y-6">
        <div className="flex gap-3 flex-wrap">
          <button
            onClick={() => action(startReplay, 'Start')}
            disabled={status.running}
            className="px-5 py-2.5 bg-netflix-accent rounded-lg font-medium hover:bg-red-600 disabled:opacity-50"
          >
            ▶ Start first {batchSize}
          </button>
          <button
            onClick={() => action(nextReplayBatch, 'Next batch')}
            disabled={status.running || atEnd}
            className="px-5 py-2.5 bg-blue-600/90 rounded-lg font-medium hover:bg-blue-600 disabled:opacity-50"
          >
            ⏭ Next {batchSize} events
          </button>
          <button
            onClick={() => action(pauseReplay, 'Pause')}
            disabled={!status.running}
            className="px-5 py-2.5 bg-yellow-600/80 rounded-lg font-medium hover:bg-yellow-600 disabled:opacity-50"
          >
            ⏸ Pause
          </button>
          <button
            onClick={() => action(resumeReplay, 'Resume')}
            disabled={!status.running && !status.paused}
            className="px-5 py-2.5 bg-green-600/80 rounded-lg font-medium hover:bg-green-600 disabled:opacity-50"
          >
            ⏵ Resume
          </button>
          <button
            onClick={() => action(resetReplay, 'Reset')}
            disabled={status.running}
            className="px-5 py-2.5 bg-white/10 rounded-lg font-medium hover:bg-white/20 disabled:opacity-50"
          >
            ↺ Reset
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
              {status.processed?.toLocaleString()} / {status.total?.toLocaleString()} events
            </span>
          </div>
          <div className="h-2 bg-black/40 rounded-full overflow-hidden">
            <div
              className="h-full bg-netflix-accent transition-all duration-300"
              style={{ width: `${progress}%` }}
            />
          </div>
          <p className="text-xs text-netflix-muted mt-2">
            {atEnd
              ? 'All events replayed. Click Reset to start over.'
              : status.hasMore
                ? `Next batch will replay events ${(status.processed + 1).toLocaleString()}–${Math.min(status.processed + batchSize, status.total).toLocaleString()}`
                : 'Ready'}
          </p>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-center text-sm">
          <div className="p-3 rounded-lg bg-black/30">
            <p className="text-netflix-muted">Status</p>
            <p className="font-semibold mt-1">
              {status.running ? (status.paused ? 'Paused' : 'Running') : 'Idle'}
            </p>
          </div>
          <div className="p-3 rounded-lg bg-black/30">
            <p className="text-netflix-muted">Batch size</p>
            <p className="font-semibold mt-1">{batchSize}</p>
          </div>
          <div className="p-3 rounded-lg bg-black/30">
            <p className="text-netflix-muted">Speed</p>
            <p className="font-semibold mt-1">{status.speed}x</p>
          </div>
          <div className="p-3 rounded-lg bg-black/30">
            <p className="text-netflix-muted">Progress</p>
            <p className="font-semibold mt-1">{progress.toFixed(2)}%</p>
          </div>
        </div>
      </div>

      <div className="mt-6 p-4 rounded-lg bg-black/30 text-sm text-netflix-muted">
        <p className="font-medium text-white mb-2">Demo tip</p>
        <ol className="list-decimal list-inside space-y-1">
          <li>Click <strong>Start (100 events)</strong> for the first batch</li>
          <li>Click <strong>Next 100 events</strong> to stream more without flooding Kafka</li>
          <li>Open Dashboard and pick a user to see live updates</li>
        </ol>
      </div>
    </div>
  )
}
