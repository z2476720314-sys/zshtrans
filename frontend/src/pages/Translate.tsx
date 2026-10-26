import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Select, Spin, Tooltip } from 'antd'
import { message } from '../toast'
import {
  CameraOutlined,
  CopyOutlined,
  SwapOutlined,
  ThunderboltOutlined,
  ClearOutlined,
  SelectOutlined,
  CloseCircleFilled,
} from '@ant-design/icons'
import { api, type LangOption, type TranslateResult } from '../api'
import { briefError, type BriefError } from '../errors'

type Props = {
  active: boolean
  localAccess: boolean
  initialDirection?: { src: string; tgt: string }
  languages: LangOption[]
  targetLanguages: LangOption[]
  onTranslated: (res: TranslateResult | null) => void
  /** 常用语「插入」信号：n 变化即插入一次 */
  insert?: { text: string; n: number } | null
}

// FLIP 与 Go 端 internal/lang.Flip 一致：除 auto/zh 外反向一律回中文（translate.py:4338）
const FLIP: Record<string, string> = { zh: 'ru' }
for (const id of ['ru', 'en', 'ja', 'ko', 'fr', 'de', 'es', 'pt', 'it', 'ar', 'tr', 'vi', 'th', 'id']) {
  FLIP[id] = 'zh'
}

/** 本地交换语向，语义同 internal/lang.Swap */
function localSwap(src: string, tgt: string, detected?: string): [string, string] {
  const base = src === 'auto' ? detected || 'ru' : src
  const newTgt = base === tgt ? FLIP[tgt] || 'ru' : base
  const newSrc = tgt === newTgt ? 'auto' : tgt
  return [newSrc, newTgt]
}

export default function Translate({ active, localAccess, initialDirection, languages, targetLanguages, onTranslated, insert }: Props) {
  const [src, setSrc] = useState('auto')
  const [tgt, setTgt] = useState('zh')
  const directionLoaded = useRef(false)
  const [text, setText] = useState('')
  const [out, setOut] = useState('')
  const [busy, setBusy] = useState(false)
  const [meta, setMeta] = useState<TranslateResult | null>(null)
  const [ocrBusy, setOcrBusy] = useState(false)
  const [err, setErr] = useState<BriefError | null>(null)
  const request = useRef<AbortController | null>(null)

  const cancel = () => {
    request.current?.abort()
    request.current = null
    setBusy(false)
    setOcrBusy(false)
  }

  useEffect(() => () => request.current?.abort(), [])

  useEffect(() => {
    if (!initialDirection || directionLoaded.current) return
    directionLoaded.current = true
    setSrc(initialDirection.src)
    setTgt(initialDirection.tgt)
  }, [initialDirection])

  const direction = (from: string, to: string) => {
    cancel()
    setSrc(from)
    setTgt(to)
    setMeta(null)
    setErr(null)
    if (localAccess) void api.saveSettings({ src_lang: from, tgt_lang: to }).catch(() => message.error('语向未保存，快捷翻译仍使用上次设置'))
  }

  const chars = useMemo(() => Array.from(text).length, [text])

  const run = async () => {
    if (request.current) return
    const body = text.trim()
    if (!body) {
      message.warning('请先输入要翻译的文本')
      return
    }
    setBusy(true)
    setErr(null)
    setOut('')
    setMeta(null)
    const owner = new AbortController()
    request.current = owner
    try {
      const res = await api.translate(body, src, tgt, owner.signal)
      if (request.current !== owner) return
      setOut(res.text)
      setMeta(res)
      onTranslated(res)
    } catch (e) {
      if (owner.signal.aborted || request.current !== owner) return
      // 全链路失败时后端会把每个提供方的完整请求 URL 都塞进错误里，
      // 直接丢进 message 会把页面顶部撑爆，这里压成一行 + 面板内可展开详情
      const brief = briefError(e)
      setOut('')
      setMeta(null)
      setErr(brief)
      onTranslated(null)
      message.error(brief.title)
    } finally {
      if (request.current === owner) {
        request.current = null
        setBusy(false)
      }
    }
  }

  // Ctrl+Enter 翻译（对应原 12.4）
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (active && (e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault()
        void run()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  // 常用语「插入」：信号变化时把内容接到输入框
  useEffect(() => {
    if (!insert) return
    cancel()
    setText((prev) => (prev ? `${prev}\n${insert.text}` : insert.text))
  }, [insert])

  // 交换语向（对应 translate.py:2874 _swap_lang / web 版 lastDetected）
  const swap = () => {
    const detected = meta?.src && meta.src !== 'auto' ? meta.src : undefined
    const [nextSrc, nextTgt] = localSwap(src, tgt, detected)
    direction(nextSrc, nextTgt)
    if (out) {
      setText(out)
      setOut(text)
    }
  }

  const copy = async () => {
    if (!out) return
    try {
      await api.copyText(out)
      message.success('译文已复制')
    } catch {
      message.error('复制失败，请手动选择')
    }
  }

  const shot = async () => {
    if (request.current) return
    const owner = new AbortController()
    request.current = owner
    setOcrBusy(true)
    try {
      const res = await api.capture(undefined, owner.signal)
      if (request.current !== owner || res.action === 'cancel' || res.action === 'complete') return
      setText(res.source)
      setOut(res.action === 'ocr' ? '' : res.text)
      setMeta(res.action === 'ocr' ? null : res)
      setErr(null)
      onTranslated(res.action === 'ocr' ? null : res)
      message.success(res.action === 'ocr' ? '识别文字已填入原文' : `截图翻译完成（${res.engine || res.provider}）`)
    } catch (e) {
      if (owner.signal.aborted || request.current !== owner) return
      message.error(briefError(e).title)
    } finally {
      if (request.current === owner) {
        request.current = null
        setOcrBusy(false)
      }
    }
  }

  // 划词翻译：模拟 Ctrl+C 取外部程序里选中的文本（对应 Ctrl+Alt+T 热键的手动触发入口）
  const grab = async () => {
    if (request.current) return
    const owner = new AbortController()
    request.current = owner
    setOcrBusy(true)
    try {
      const res = await api.selection(owner.signal)
      if (request.current !== owner) return
      setText(res.source)
      setOut(res.text)
      setMeta(res)
      setErr(null)
      onTranslated(res)
      message.success(`已取到选中文本（${res.provider}）`)
    } catch (e) {
      if (owner.signal.aborted || request.current !== owner) return
      message.error(briefError(e).title)
    } finally {
      if (request.current === owner) {
        request.current = null
        setOcrBusy(false)
      }
    }
  }

  return (
    <div className="workgrid">
      <section className="panel">
        <div className="panel-head">
          <Select
            size="small"
            variant="borderless"
            className="langsel"
            aria-label="原文语言"
            value={src}
            onChange={(value) => { direction(value, tgt); setOut('') }}
            options={languages}
            popupMatchSelectWidth={false}
          />
          <div className="spacer" />
          <Tooltip title="交换语向">
            <button className="swap" aria-label="交换语向" onClick={() => void swap()} type="button">
              <SwapOutlined />
            </button>
          </Tooltip>
        </div>
        <div className="panel-body">
          <textarea
            className="editor"
            aria-label="原文"
            value={text}
            onChange={(e) => { cancel(); setText(e.target.value) }}
            placeholder="输入或粘贴客户消息、商品标题、物流售后文本… Ctrl+Enter 翻译"
            spellCheck={false}
          />
        </div>
        <div className="panel-head" style={{ borderBottom: 0, borderTop: '1px solid var(--line)' }}>
          <Button
            type="primary"
            icon={<ThunderboltOutlined />}
            loading={busy}
            disabled={ocrBusy}
            onClick={() => void run()}
          >
            翻译 <kbd>Ctrl+↵</kbd>
          </Button>
          {localAccess && <Button
            icon={<CameraOutlined />}
            loading={ocrBusy}
            onClick={() => void shot()}
          >
            截图取词
          </Button>}
          {localAccess && <Button
            icon={<SelectOutlined />}
            loading={ocrBusy}
            onClick={() => void grab()}
          >
            划词取词
          </Button>}
          <Button icon={<ClearOutlined />} onClick={() => { cancel(); setText(''); setOut(''); setMeta(null); setErr(null); onTranslated(null) }}>
            清空
          </Button>
          <div className="spacer" />
          <span className="mono count">{chars} 字符</span>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <Select
            size="small"
            variant="borderless"
            className="langsel"
            aria-label="译文语言"
            value={tgt}
            onChange={(value) => { direction(src, value); setOut('') }}
            options={targetLanguages}
            popupMatchSelectWidth={false}
          />
          <div className="spacer" />
          {meta && (
            <span className="pill">
              {meta.provider} · {meta.ms}ms
            </span>
          )}
          <Tooltip title="复制译文">
            <Button aria-label="复制译文" size="small" type="text" icon={<CopyOutlined />} disabled={!out} onClick={() => void copy()} />
          </Tooltip>
        </div>
        <div className="panel-body">
          {out ? (
            <div className="outtext" aria-label="译文" aria-live="polite">{out}</div>
          ) : busy ? (
            // 免费引擎链路要逐个超时重试（实测 10-12s），必须给可见反馈，
            // 否则用户以为按钮没生效
            <div className="outtext pending">
              <Spin size="small" />
              <span>正在翻译…</span>
              <Button size="small" onClick={cancel}>取消</Button>
            </div>
          ) : err ? (
            <div className="errbox">
              <div className="errhead">
                <CloseCircleFilled style={{ color: 'var(--coral)' }} />
                <span>翻译失败</span>
                <div className="spacer" />
                <Button size="small" type="text" onClick={() => setErr(null)}>
                  关闭
                </Button>
              </div>
              <ol className="errlist">
                {err.items.length > 1 ? (
                  err.items.map((t, i) => <li key={i}>{t}</li>)
                ) : (
                  <li>{err.detail}</li>
                )}
              </ol>
            </div>
          ) : (
            <div className="outtext empty">译文会显示在这里</div>
          )}
        </div>
      </section>
    </div>
  )
}
