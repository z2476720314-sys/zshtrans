import { useEffect, useState } from 'react'
import { Badge, Descriptions, Switch, Table, Tag, message } from 'antd'
import { api, type SelfCheck, type UpdateInfo } from '../api'

export default function SelfCheckPanel() {
  const [data, setData] = useState<SelfCheck | null>(null)
  const [upd, setUpd] = useState<UpdateInfo | null>(null)
  const [autoCheck, setAutoCheck] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    void api
      .selfcheck()
      .then((d) => {
        setData(d)
        return api.config()
      })
      .then((c) => setAutoCheck(c.update_check !== false))
      .catch(() => setData(null))
    void api
      .updateCheck(false)
      .then(setUpd)
      .catch(() => setUpd(null))
  }, [])

  if (!data) return <div className="hint">自检中…</div>

  const rows = [
    { k: '应用', v: `${data.app} ${data.version}` },
    { k: '界面工具包', v: data.ui_toolkit },
    { k: '语言数量', v: String(data.language_count) },
    { k: '提供方预设', v: String(data.provider_preset_count) },
    { k: '直连提供方', v: String(data.direct_provider_count) },
    { k: '模型卡 / 可用', v: `${data.providers} / ${data.usable}` },
    { k: '常用语', v: String(data.phrases_count) },
    { k: '常用语指纹', v: data.phrases_sha256.slice(0, 16) + '…' },
    { k: '字形完整', v: data.font_glyphs_ok ? '是' : '否' },
    { k: '翻译缓存', v: `${data.cache} 条` },
  ]

  const saveAuto = async (on: boolean) => {
    setSaving(true)
    try {
      await api.saveSettings({ update_check: on })
      setAutoCheck(on)
      message.success(on ? '已开启启动时检查更新' : '已关闭启动时检查更新')
    } catch {
      message.error('保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <div className="langbar">
        <Badge status={data.ok ? 'success' : 'error'} text={data.ok ? '自检通过' : '自检异常'} />
        <div className="spacer" />
        <Tag color="lime">{data.version}</Tag>
      </div>

      <div className="card">
        <Table
          size="small"
          rowKey="k"
          pagination={false}
          dataSource={rows}
          columns={[
            { title: '项目', dataIndex: 'k', width: 180 },
            { title: '值', dataIndex: 'v', render: (v: string) => <span className="mono">{v}</span> },
          ]}
        />
      </div>

      <div className="card">
        <Descriptions
          column={1}
          size="small"
          title="运行路径"
          items={[
            { key: '1', label: '配置文件', children: <span className="mono">{data.config_path}</span> },
            { key: '2', label: '常用语', children: <span className="mono">{data.phrases_path}</span> },
            { key: '3', label: '日志', children: <span className="mono">{data.log_path}</span> },
          ]}
        />
      </div>

      <div className="card">
        <Descriptions
          column={1}
          size="small"
          title="更新"
          items={[
            {
              key: '1',
              label: '启动时自动检查',
              children: (
                <Switch
                  size="small"
                  checked={autoCheck}
                  loading={saving}
                  onChange={(v) => void saveAuto(v)}
                />
              ),
            },
            {
              key: '2',
              label: '最新版本',
              children: upd?.has_new ? (
                <span className="mono">
                  {upd.latest} <Tag color="lime">可更新</Tag>
                  {upd.skipped ? <Tag>已跳过</Tag> : null}
                </span>
              ) : (
                <span className="mono">
                  {upd && !upd.ok ? upd.error || '检查失败' : `${upd?.current ?? data.version}（已是最新）`}
                </span>
              ),
            },
            {
              key: '3',
              label: '发布渠道',
              children: <span className="mono">GitHub Releases · 可在顶栏「检查更新」手动触发</span>,
            },
          ]}
        />
      </div>
    </div>
  )
}
