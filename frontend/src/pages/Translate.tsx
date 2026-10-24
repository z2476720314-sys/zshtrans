import { useEffect, useMemo, useState } from 'react'
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
import { useStored } from '../hooks'
import { briefError, type BriefError } from '../errors'

type Props = {
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

export default function Translate({ languages, targetLanguages, onTranslated, insert }: Props) {
  const [src, setSrc] = useStored('tt.src', 'auto')
  const [tgt, setTgt] = useStored('tt.tgt', 'zh')
  const [text, setText] = useState('')
  const [out, setOut] = useState('')
  const [busy, setBusy] = useState(false)
  const [meta, setMeta] = useState<TranslateResult | null>(null)
  const [ocrBusy, setOcrBusy] = useState(false)
  const [err, setErr] = useState<BriefError | null>(null)

  const chars = useMemo(() => Array.from(text).length, [text])

  const run = async () => {
    const body = text.trim()
    if (!body) {
      message.warning('请先输入要翻译的文本')
      return
    }
    setBusy(true)
    setErr(null)
    try {
      const res = await api.translate(body, src, tgt)
      setOut(res.text)
      setMeta(res)
      onTranslated(res)
    } catch (e) {
      // 全链路失败时后端会把每个提供方的完整请求 URL 都塞进错误里，
      // 直接丢进 message 会把页面顶部撑爆，这里压成一行 + 面板内可展开详情
      const brief = briefError(e)
      setOut('')
      setMeta(null)
      setErr(brief)
      onTranslated(null)
      message.error(brief.title)
    } finally {
      setBusy(false)
    }
  }

  // Ctrl+Enter 翻译（对应原 12.4）
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
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
    setText((prev) => (prev ? `${prev}\n${insert.text}` : insert.text))
  }, [insert])

  // 交换语向（对应 translate.py:2874 _swap_lang / web 版 lastDetected）
  const swap = async () => {
    const detected = meta?.src && meta.src !== 'auto' ? meta.src : undefined
    try {
      const res = await api.swap(src, tgt, detected)
      setSrc(res.from)
      setTgt(res.to)
      if (out) {
        setText(out)
        setOut(text)
        setMeta(null)
      }
    } catch {
      const [nextSrc, nextTgt] = localSwap(src, tgt, detected)
      setSrc(nextSrc)
      setTgt(nextTgt)
    }
  }

  const copy = async () => {
    if (!out) return
    try {
      await navigator.clipboard.writeText(out)
      message.success('译文已复制')
    } catch {
      message.error('复制失败，请手动选择')
    }
  }

  const shot = async () => {
    setOcrBusy(true)
    try {
      const res = await api.capture()
      setText(res.source)
      setOut(res.text)
      message.success(`截图取词完成（${res.engine || res.provider}）`)
    } catch (e) {
      message.error(briefError(e).title)
    } finally {
      setOcrBusy(false)
    }
  }

  // 划词翻译：模拟 Ctrl+C 取外部程序里选中的文本（对应 Ctrl+Alt+T 热键的手动触发入口）
  const grab = async () => {
    setOcrBusy(true)
    try {
      const res = await api.selection()
      setText(res.source)
      setOut(res.text)
      setMeta({ text: res.text, provider: res.provider, ms: res.ms, src: '', tgt: '' })
      onTranslated({ text: res.text, provider: res.provider, ms: res.ms, src: '', tgt: '' })
      message.success(`已取到选中文本（${res.provider}）`)
    } catch (e) {
      message.error(briefError(e).title)
    } finally {
      setOcrBusy(false)
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
            value={src}
            onChange={setSrc}
            options={languages}
            popupMatchSelectWidth={false}
          />
          <div className="spacer" />
          <Tooltip title="交换语向">
            <button className="swap" onClick={() => void swap()} type="button">
              <SwapOutlined />
            </button>
          </Tooltip>
        </div>
        <div className="panel-body">
          <textarea
            className="editor"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="输入或粘贴客户消息、商品标题、物流售后文本… Ctrl+Enter 翻译"
            spellCheck={false}
          />
        </div>
        <div className="panel-head" style={{ borderBottom: 0, borderTop: '1px solid var(--line)' }}>
          <Button
            type="primary"
            icon={<ThunderboltOutlined />}
            loading={busy}
            onClick={() => void run()}
          >
            翻译 <kbd>Ctrl+↵</kbd>
          </Button>
          <Button
            icon={<CameraOutlined />}
            loading={ocrBusy}
            onClick={() => void shot()}
          >
            截图取词
          </Button>
          <Button
            icon={<SelectOutlined />}
            loading={ocrBusy}
            onClick={() => void grab()}
          >
            划词取词
          </Button>
          <Button icon={<ClearOutlined />} onClick={() => { setText(''); setOut(''); setMeta(null); onTranslated(null) }}>
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
            value={tgt}
            onChange={setTgt}
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
            <Button size="small" type="text" icon={<CopyOutlined />} disabled={!out} onClick={() => void copy()} />
          </Tooltip>
        </div>
        <div className="panel-body">
          {out ? (
            <div className="outtext">{out}</div>
          ) : busy ? (
            // 免费引擎链路要逐个超时重试（实测 10-12s），必须给可见反馈，
            // 否则用户以为按钮没生效
            <div className="outtext pending">
              <Spin size="small" />
              <span>正在翻译，逐个尝试提供方…</span>
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
