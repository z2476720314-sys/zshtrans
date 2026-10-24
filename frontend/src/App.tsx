import { useCallback, useEffect, useMemo, useState } from 'react'
import { Badge, Button, Tooltip } from 'antd'
import { message } from './toast'
import {
  CheckCircleOutlined,
  CloseOutlined,
  CopyOutlined,
  SettingOutlined,
  TranslationOutlined,
  UnorderedListOutlined,
  HeartOutlined,
} from '@ant-design/icons'
import { api, type LangOption, type ProviderStatus, type Status, type TranslateResult } from './api'
import { useEvents } from './hooks'
import Translate from './pages/Translate'
import Phrases from './pages/Phrases'
import Settings from './pages/Settings'
import SelfCheckPanel from './pages/SelfCheck'
import UpdatePanel from './components/UpdatePanel'

const TABS = [
  { key: 'translate', label: '翻译', icon: <TranslationOutlined /> },
  { key: 'phrases', label: '常用语', icon: <UnorderedListOutlined /> },
  { key: 'settings', label: '设置', icon: <SettingOutlined /> },
  { key: 'selfcheck', label: '自检', icon: <CheckCircleOutlined /> },
] as const

type TabKey = (typeof TABS)[number]['key']

type Popup = {
  source: string
  text: string
  provider: string
  ms: number
  engine?: string
}

export default function App() {
  const [tab, setTab] = useState<TabKey>('translate')
  const [status, setStatus] = useState<Status | null>(null)
  const [languages, setLanguages] = useState<LangOption[]>([])
  const [targetLanguages, setTargetLanguages] = useState<LangOption[]>([])
  const [used, setUsed] = useState<{ provider: string; ms: number } | null>(null)
  const [popup, setPopup] = useState<Popup | null>(null)
  const [insert, setInsert] = useState<{ text: string; n: number } | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [st, lg] = await Promise.all([api.status(), api.languages()])
      setStatus(st)
      setLanguages(lg.all ?? [])
      setTargetLanguages(lg.target ?? [])
    } catch {
      /* 后端尚未就绪时静默重试 */
    }
  }, [])

  useEffect(() => {
    void refresh()
    const timer = window.setInterval(() => void refresh(), 15000)
    return () => window.clearInterval(timer)
  }, [refresh])

  // 托盘「设置」等入口通过 URL hash 指定落地页（#settings / #phrases / #selfcheck）
  useEffect(() => {
    const apply = () => {
      const key = window.location.hash.replace(/^#/, '') as TabKey
      if (TABS.some((t) => t.key === key)) {
        setTab(key)
        window.history.replaceState(null, '', window.location.pathname + window.location.search)
      }
    }
    apply()
    window.addEventListener('hashchange', apply)
    return () => window.removeEventListener('hashchange', apply)
  }, [])

  const online = useEvents((ev) => {
    if (ev.kind === 'ready') {
      // 通道就绪心跳，不打扰用户
    } else if (ev.kind === 'popup' && ev.data) {
      setPopup(ev.data as Popup)
      void refresh()
    } else if (ev.kind === 'log') {
      // 后端热键链路失败（取词失败/截图失败/OCR 失败）透传成前端提示，
      // 对应 pyc 里热键异常时弹出的错误框
      message.error(typeof ev.data === 'string' ? ev.data : '热键执行失败')
    } else if (ev.kind === 'toggle') {
      message.info('主窗口显隐热键已触发')
    } else if (ev.kind === 'close') {
      message.info('隐藏到托盘热键已触发')
    }
  })

  // 划词弹窗：点击外部 / 按 Esc 关闭（对应 translate.py PopupClickMonitor）
  useEffect(() => {
    if (!popup) return
    const onDown = (e: MouseEvent) => {
      const el = e.target as HTMLElement | null
      if (el?.closest('.popup-wrap')) return
      setPopup(null)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPopup(null)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [popup])

  const onTranslated = useCallback((res: TranslateResult | null) => {
    setUsed(res ? { provider: res.provider, ms: res.ms } : null)
    if (res) void refresh()
  }, [refresh])

  const chain: ProviderStatus[] = status?.providers ?? []

  const pills = useMemo(() => {
    if (!status) return null
    const healthy = chain.filter((p) => p.healthy).length
    return (
      <>
        <span className={`pill ${healthy === chain.length && chain.length > 0 ? 'ok' : 'err'}`}>
          {healthy}/{chain.length} 提供方健康
        </span>
        <span className="pill">缓存 {status.cache}</span>
        {status.lan && status.lan !== '127.0.0.1' && (
          <span className="pill mono">局域网 {status.lan}</span>
        )}
      </>
    )
  }, [status, chain])

  const insertPhrase = (content: string) => {
    setInsert({ text: content, n: Date.now() })
    setTab('translate')
  }

  return (
    <>
      <div className="aurora">
        <i />
        <i />
        <i />
      </div>

      <div className="shell">
        <header className="topbar">
          <div className="brand">
            <b>{status?.app ?? 'ZSH TRANS'}</b>
            <span>跨境翻译台 · {status?.version ?? 'v3.0'}</span>
          </div>

          <div className="seg" style={{ marginLeft: 8 }}>
            {TABS.map((t) => (
              <button
                key={t.key}
                className={tab === t.key ? 'on' : ''}
                onClick={() => setTab(t.key)}
                type="button"
              >
                {t.label}
              </button>
            ))}
          </div>

          <div className="spacer" />
          <UpdatePanel version={status?.version} />
          {pills}
          <Tooltip title={online ? '热键事件通道已连接' : '热键事件通道重连中'}>
            <span className={`pill ${online ? 'ok' : 'err'}`}>
              <HeartOutlined /> {online ? '实时' : '离线'}
            </span>
          </Tooltip>
        </header>

        <main className="body">
          {tab === 'translate' && (
            <Translate
              languages={languages}
              targetLanguages={targetLanguages}
              onTranslated={onTranslated}
              insert={insert}
            />
          )}
          {tab === 'phrases' && <Phrases onInsert={insertPhrase} />}
          {tab === 'settings' && <Settings onSaved={() => void refresh()} />}
          {tab === 'selfcheck' && <SelfCheckPanel />}
        </main>

        <footer className="chain">
          <span>链路</span>
          {chain.length === 0 && <span className="node cool">无可用提供方</span>}
          {chain.map((p) => {
            const cls = [
              'node',
              p.healthy ? '' : 'cool',
              used?.provider === p.name ? 'used' : '',
            ]
              .filter(Boolean)
              .join(' ')
            return (
              <span className={cls} key={p.name}>
                <i />
                {p.label || p.name}
                {used?.provider === p.name && <span className="mono"> · {used.ms}ms</span>}
              </span>
            )
          })}
        </footer>
      </div>

      {popup && (
        <div className="popup-wrap">
          <div className="popup-head">
            <Badge status="processing" />
            <span>划词翻译</span>
            <span className="pill" style={{ marginLeft: 2 }}>
              {popup.provider} · {popup.ms}ms
            </span>
            <div className="spacer" />
            <Tooltip title="复制译文">
              <Button
                size="small"
                type="text"
                icon={<CopyOutlined />}
                onClick={() => void navigator.clipboard.writeText(popup.text).then(
                  () => message.success('已复制'),
                  () => message.error('复制失败'),
                )}
              />
            </Tooltip>
            <Button size="small" type="text" icon={<CloseOutlined />} onClick={() => setPopup(null)} />
          </div>
          <div className="popup-body">
            <div className="src">{popup.source}</div>
            <div className="dst">{popup.text}</div>
          </div>
        </div>
      )}
    </>
  )
}
