import { useCallback, useEffect, useRef, useState } from 'react'
import { isDesktop } from './api'

/** SSE 事件类型，与 Go 端 server.Event 对应 */
export type ServerEvent = {
  kind: 'ready' | 'popup' | 'toggle' | 'log'
  data?: unknown
}

/** 订阅 /api/events；返回连接状态 */
export function useEvents(onEvent: (ev: ServerEvent) => void, enabled = true) {
  const [online, setOnline] = useState(false)
  const cbRef = useRef(onEvent)
  cbRef.current = onEvent

  useEffect(() => {
    if (!enabled) return
    if (isDesktop()) {
      const receive = (event: Event) => cbRef.current((event as CustomEvent<ServerEvent>).detail)
      window.addEventListener('zsh-event', receive)
      setOnline(true)
      return () => { window.removeEventListener('zsh-event', receive); setOnline(false) }
    }
    let es: EventSource | null = null
    let closed = false
    let retry = 0

    const connect = () => {
      if (closed) return
      es = new EventSource('/api/events')
      es.onopen = () => {
        retry = 0
        setOnline(true)
      }
      es.onmessage = (e) => {
        try {
          cbRef.current(JSON.parse(e.data) as ServerEvent)
        } catch {
          /* 忽略心跳等非 JSON 数据 */
        }
      }
      es.onerror = () => {
        setOnline(false)
        es?.close()
        if (closed) return
        // 退避重连，最长 10s
        retry = Math.min(retry + 1, 5)
        window.setTimeout(connect, 500 * retry)
      }
    }

    connect()
    return () => {
      closed = true
      es?.close()
      setOnline(false)
    }
  }, [enabled])

  return online
}

/** 本地持久化（对应原脚本 localStorage tt.src / tt.tgt） */
export function useStored<T>(key: string, initial: T) {
  const [value, setValue] = useState<T>(() => {
    try {
      const raw = window.localStorage.getItem(key)
      if (raw != null) return JSON.parse(raw) as T
    } catch {
      /* 损坏就忽略 */
    }
    return initial
  })

  const store = useCallback(
    (next: T) => {
      setValue(next)
      try {
        window.localStorage.setItem(key, JSON.stringify(next))
      } catch {
        /* 隐私模式忽略 */
      }
    },
    [key],
  )

  return [value, store] as const
}
