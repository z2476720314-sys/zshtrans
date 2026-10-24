import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Modal, Progress, Space, Tag, Typography, message } from 'antd'
import { CloudDownloadOutlined, ReloadOutlined } from '@ant-design/icons'
import { api, type UpdateInfo, type UpdateProgress } from '../api'

const { Paragraph, Text } = Typography

type Props = {
  /** 启动时自动检查（false 时只在用户点「检查更新」时检查） */
  auto?: boolean
  /** 应用版本（页面上要显示） */
  version?: string
}

/**
 * 自更新卡片：检查 GitHub Release → 用户自行决定是否更新（下载 / 安装 / 跳过此版本）。
 * 对应需求「增加自动检测更新功能供用户选择是否更新」。
 */
export default function UpdatePanel({ auto = true, version }: Props) {
  const [info, setInfo] = useState<UpdateInfo | null>(null)
  const [open, setOpen] = useState(false)
  const [checking, setChecking] = useState(false)
  const [installing, setInstalling] = useState(false)
  const [prog, setProg] = useState<UpdateProgress | null>(null)
  const timer = useRef<number | null>(null)

  const check = useCallback(
    async (force: boolean) => {
      setChecking(true)
      try {
        const res = await api.updateCheck(force)
        setInfo(res)
        if (res.ok && res.has_new && !res.skipped) setOpen(true)
        else if (force) {
          if (res.ok && res.has_new) message.info(`发现新版本 ${res.latest}（已设为跳过）`)
          else if (res.ok) message.success(`已是最新版本 ${res.current}`)
          else message.warning(res.error || '检查更新失败')
        }
      } catch {
        if (force) message.warning('检查更新失败（网络不可用或仓库未发布）')
      } finally {
        setChecking(false)
      }
    },
    [],
  )

  // 启动自动检查：延迟 2.5s，避免和首屏翻译抢带宽
  useEffect(() => {
    if (!auto) return
    const t = window.setTimeout(() => void check(false), 2500)
    return () => window.clearTimeout(t)
  }, [auto, check])

  // 下载进度轮询
  const startPolling = useCallback(() => {
    if (timer.current) return
    const tick = async () => {
      try {
        const p = (await api.updateApply('status')) as unknown as UpdateProgress
        setProg(p)
        if (p.ready) {
          if (timer.current) {
            window.clearInterval(timer.current)
            timer.current = null
          }
          // 下载完成 → 立刻换版重启
          await api.updateApply('install')
          setInstalling(true)
          message.success('新版本已就绪，程序即将重启…')
          return
        }
        if (p.error) {
          if (timer.current) {
            window.clearInterval(timer.current)
            timer.current = null
          }
          message.error(`下载失败：${p.error}`)
        }
      } catch {
        /* 轮询失败下一轮重试 */
      }
    }
    timer.current = window.setInterval(() => void tick(), 700)
  }, [])

  useEffect(
    () => () => {
      if (timer.current) window.clearInterval(timer.current)
    },
    [],
  )

  if (!info) return null

  const percent =
    prog && prog.total > 0 ? Math.min(100, Math.round((prog.done / prog.total) * 100)) : prog?.ready ? 100 : 0

  return (
    <>
      <Button
        size="small"
        icon={<ReloadOutlined />}
        loading={checking}
        onClick={() => void check(true)}
      >
        检查更新
      </Button>

      <Modal
        open={open}
        title={
          <Space>
            <CloudDownloadOutlined />
            发现新版本
            <Tag color="lime">{info.latest}</Tag>
            <Text type="secondary" style={{ fontSize: 12 }}>
              当前 {info.current}
            </Text>
          </Space>
        }
        onCancel={() => {
          setOpen(false)
          if (timer.current) {
            window.clearInterval(timer.current)
            timer.current = null
          }
        }}
        footer={null}
        width={560}
        destroyOnHidden
      >
        {installing ? (
          <Paragraph>
            <Text strong>正在重启完成更新…</Text>
          </Paragraph>
        ) : prog ? (
          <Space direction="vertical" style={{ width: '100%' }} size={10}>
            <Progress percent={percent} status={prog.error ? 'exception' : 'active'} />
            <Text type="secondary" className="mono" style={{ fontSize: 12 }}>
              {fmt(prog.done)} / {prog.total > 0 ? fmt(prog.total) : '未知大小'}
            </Text>
            {prog.error ? <Text type="danger">{prog.error}</Text> : null}
          </Space>
        ) : (
          <>
            <Paragraph type="secondary" style={{ marginBottom: 8, fontSize: 12 }}>
              更新内容（来自 GitHub Releases）
            </Paragraph>
            <div className="upd-notes">{info.notes?.trim() || '（该版本没有填写更新说明）'}</div>
            {info.page ? (
              <Paragraph style={{ marginTop: 10, marginBottom: 0, fontSize: 12 }}>
                <a href={info.page} target="_blank" rel="noreferrer">
                  在 GitHub 查看此版本
                </a>
                {info.size ? (
                  <Text type="secondary" className="mono" style={{ marginLeft: 10 }}>
                    {fmt(info.size)}
                  </Text>
                ) : null}
              </Paragraph>
            ) : null}
          </>
        )}

        <div className="upd-actions">
          <Button
            type="primary"
            disabled={installing}
            onClick={() => {
              setProg({ ok: true, busy: true, done: 0, total: info.size ?? 0, ready: false, error: '' })
              startPolling()
              void api.updateApply('download').catch(() => message.error('启动下载失败'))
            }}
          >
            立即更新
          </Button>
          <Button
            onClick={() => {
              void api
                .updateApply('status', info.latest ?? '')
                .then(() => {
                  message.info(`已跳过 ${info.latest}，可在「自检」页手动检查更新`)
                  setOpen(false)
                })
                .catch(() => message.error('记录跳过失败'))
            }}
          >
            跳过此版本
          </Button>
          <div className="spacer" />
          <Button type="text" onClick={() => setOpen(false)}>
            稍后
          </Button>
        </div>
      </Modal>
      {version ? <span className="sr-only">{version}</span> : null}
    </>
  )
}

function fmt(n: number): string {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i += 1
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}
