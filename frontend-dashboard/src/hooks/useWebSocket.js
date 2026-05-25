import { useEffect, useRef, useState, useCallback } from 'react'
import { getWsUrl } from '../api/client'

export function useWebSocket(userId, onMessage) {
  const [connected, setConnected] = useState(false)
  const wsRef = useRef(null)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage

  const connect = useCallback(() => {
    if (!userId) return
    const ws = new WebSocket(getWsUrl(userId))
    wsRef.current = ws

    ws.onopen = () => setConnected(true)
    ws.onclose = () => {
      setConnected(false)
      setTimeout(connect, 3000)
    }
    ws.onerror = () => setConnected(false)
    ws.onmessage = (evt) => {
      try {
        const data = JSON.parse(evt.data)
        onMessageRef.current?.(data)
      } catch (_) {}
    }
  }, [userId])

  useEffect(() => {
    connect()
    return () => wsRef.current?.close()
  }, [connect])

  return { connected }
}
