import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Modal, Progress, Space, Tag, Typography } from 'antd'
import { CloudDownloadOutlined, ReloadOutlined } from '@ant-design/icons'
import { api, type UpdateInfo, type UpdateProgress } from '../api'
import { message } from '../toast'

const { Paragraph, Text } = Typography

export default function UpdatePanel({ auto = true, version }: { auto?: boolean; version?: string }) {
  const [info, setInfo] = useState<UpdateInfo | null>(null)
  const [open, setOpen] = useState(false)
  const [checking, setChecking] = useState(false)
  const [installing, setInstalling] = useState(false)
  const [prog, setProg] = useState<UpdateProgress | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const pollGeneration = useRef(0)

  const stopPolling = useCallback(() => {
    pollGeneration.current += 1
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
  }, [])

  const check = useCallback(async (force: boolean) => {
    setChecking(true)
    try {
      const res = await api.updateCheck(force)
      setInfo(res)
      if (res.ok && res.has_new && !res.skipped) setOpen(true)
      else if (force) {
        if (!res.ok) message.warning(res.error || '检查更新失败')
        else message.info(res.has_new ? '已跳过 ' + res.latest : '已是最新版本 ' + res.current)
      }
    } catch {
      if (force) message.error('检查更新失败，请检查网络后重试')
    } finally {
      setChecking(false)
    }
  }, [])

  useEffect(() => {
    if (!auto) return
    const t = setTimeout(() => void check(false), 2500)
    return () => clearTimeout(t)
  }, [auto, check])

  useEffect(() => stopPolling, [stopPolling])

  const poll = useCallback(() => {
    stopPolling()
    const generation = pollGeneration.current
    const tick = async () => {
      try {
        const next = (await api.updateApply('status')) as unknown as UpdateProgress
        if (generation !== pollGeneration.current) return
        setProg(next)
        if (next.busy) timer.current = setTimeout(() => void tick(), 700)
      } catch {
        if (generation !== pollGeneration.current) return
        setProg((p) => p && { ...p, busy: false, error: '读取下载进度失败，请重新检查更新' })
      }
    }
    void tick()
  }, [stopPolling])

  const download = async () => {
    setProg({ ok: true, busy: true, done: 0, total: info?.size ?? 0, ready: false, error: '' })
    try {
      await api.updateApply('download')
      poll()
    } catch (e) {
      setProg({ ok: false, busy: false, done: 0, total: 0, ready: false, error: String(e) })
    }
  }

  const install = async () => {
    setInstalling(true)
    try {
      await api.updateApply('install')
    } catch (e) {
      setInstalling(false)
      message.error('安装未开始：' + String(e))
    }
  }

  const percent = prog?.total ? Math.min(100, Math.round(prog.done / prog.total * 100)) : prog?.ready ? 100 : 0

  return <>
    <Button size="small" icon={<ReloadOutlined />} loading={checking}
      onClick={() => prog?.busy || prog?.ready ? setOpen(true) : void check(true)}>
      {prog?.busy ? '正在下载更新' : prog?.ready ? '更新已就绪' : '检查更新'}
    </Button>
    <Modal open={open && !!info} title={<Space><CloudDownloadOutlined />软件更新<Tag>{info?.latest}</Tag></Space>}
      onCancel={() => setOpen(false)} footer={null} width={560} destroyOnHidden>
      <Paragraph type="secondary">当前 {info?.current || version}。下载完成后，由你决定何时安装并重启。</Paragraph>
      {installing ? <Paragraph>正在重启完成更新…</Paragraph> : <>
        {prog ? <Space orientation="vertical" style={{ width: '100%' }}>
          <Progress percent={percent} status={prog.error ? 'exception' : prog.ready ? 'success' : 'active'} />
          <Text type="secondary">{fmt(prog.done)} / {prog.total ? fmt(prog.total) : '未知大小'}</Text>
          {prog.error && <Text type="danger">{prog.error}</Text>}
          {prog.ready && <Text>下载与校验完成，可以安装。</Text>}
        </Space> : <div className="upd-notes">{info?.notes?.trim() || '该版本没有更新说明。'}</div>}
        <div className="upd-actions">
          {prog?.ready ? <Button type="primary" onClick={() => void install()}>安装并重启</Button>
            : <Button type="primary" disabled={prog?.busy} onClick={() => void download()}>{prog?.error ? '重新下载' : '下载更新'}</Button>}
          {prog?.busy ? <Button onClick={() => {
            void api.updateApply('cancel').then(() => { stopPolling(); setProg(null) }).catch(() => message.error('取消下载失败，仍在获取进度'))
          }}>取消下载</Button> : <Button onClick={() => {
            void api.updateApply('cancel').then(() => api.updateApply('status', info?.latest || '')).then(() => {
              stopPolling()
              setProg(null)
              setOpen(false)
              message.info('已跳过 ' + info?.latest + '，仍可手动检查更新')
            }).catch(() => message.error('记录跳过失败'))
          }}>跳过此版本</Button>}
          <div className="spacer" />
          <Button onClick={() => setOpen(false)}>稍后</Button>
        </div>
      </>}
    </Modal>
  </>
}

function fmt(n: number): string {
  if (n < 1024) return n + ' B'
  return n < 1024 * 1024 ? (n / 1024).toFixed(1) + ' KB' : (n / 1024 / 1024).toFixed(1) + ' MB'
}
