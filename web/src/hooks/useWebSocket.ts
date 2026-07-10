import { useEffect, useRef, useCallback, useState } from 'react'

type WSMessage = {
  type: string
  payload: unknown
}

type MessageHandler = (message: WSMessage) => void

export function useWebSocket(onMessage?: MessageHandler) {
  const wsRef = useRef<WebSocket | null>(null)
  const [isConnected, setIsConnected] = useState(false)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const reconnectAttempts = useRef(0)

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      return
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws`)

    ws.onopen = () => {
      setIsConnected(true)
      reconnectAttempts.current = 0
      console.log('[WS] Connected')
    }

    ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data) as WSMessage
        onMessage?.(message)
      } catch (err) {
        console.error('[WS] Failed to parse message:', err)
      }
    }

    ws.onclose = () => {
      setIsConnected(false)
      wsRef.current = null
      console.log('[WS] Disconnected')

      // Exponential backoff for reconnection
      const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), 30000)
      reconnectAttempts.current++
      reconnectTimeoutRef.current = setTimeout(connect, delay)
    }

    ws.onerror = (error) => {
      console.error('[WS] Error:', error)
    }

    wsRef.current = ws
  }, [onMessage])

  useEffect(() => {
    connect()

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current)
      }
      wsRef.current?.close()
    }
  }, [connect])

  return { isConnected }
}

// Hook for Auto PR page that auto-refreshes on updates
export function useAutoPRWebSocket(onUpdate: () => void) {
  const handleMessage = useCallback((message: WSMessage) => {
    if (
      message.type === 'recommendation_approved' ||
      message.type === 'recommendation_dismissed' ||
      message.type === 'recommendation_created' ||
      message.type === 'mapping_updated'
    ) {
      console.log('[WS] Received update:', message.type)
      onUpdate()
    }
  }, [onUpdate])

  return useWebSocket(handleMessage)
}
