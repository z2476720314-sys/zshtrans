import { useEffect, useMemo, useState } from 'react'
import { Button, Empty, Input, Modal, Popconfirm, Tooltip } from 'antd'
import { message } from '../toast'
import {
  DeleteOutlined,
  CopyOutlined,
  EditOutlined,
  PlusOutlined,
  SendOutlined,
} from '@ant-design/icons'
import { api, ApiError, type Phrase } from '../api'

type Props = {
  /** 点「插入」时把常用语塞进翻译输入框 */
  onInsert: (content: string) => void
}

export default function Phrases({ onInsert }: Props) {
  const [items, setItems] = useState<Phrase[]>([])
  const [loading, setLoading] = useState(true)
  const [keyword, setKeyword] = useState('')
  const [editing, setEditing] = useState<Phrase | null>(null)
  const [creating, setCreating] = useState(false)
  const [desc, setDesc] = useState('')
  const [content, setContent] = useState('')
  const [saving, setSaving] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const res = await api.phrases()
      setItems(res.items ?? [])
    } catch (e) {
      message.error(e instanceof ApiError ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const shown = useMemo(() => {
    const k = keyword.trim().toLowerCase()
    if (!k) return items
    return items.filter(
      (it) => it.desc.toLowerCase().includes(k) || it.content.toLowerCase().includes(k),
    )
  }, [items, keyword])

  const openCreate = () => {
    setEditing(null)
    setDesc('')
    setContent('')
    setCreating(true)
  }

  const openEdit = (it: Phrase) => {
    setEditing(it)
    setDesc(it.desc)
    setContent(it.content)
    setCreating(true)
  }

  const save = async () => {
    if (!content.trim()) {
      message.warning('内容不能为空')
      return
    }
    setSaving(true)
    try {
      const res = editing
        ? await api.phrase({ action: 'update', id: editing.id, desc, content })
        : await api.phrase({ action: 'add', desc, content })
      setItems(res.items ?? [])
      setCreating(false)
      message.success(editing ? '常用语已更新' : '常用语已添加')
    } catch (e) {
      message.error(e instanceof ApiError ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  const remove = async (it: Phrase) => {
    try {
      const res = await api.phrase({ action: 'delete', id: it.id })
      setItems(res.items ?? [])
      message.success('已删除')
    } catch (e) {
      message.error(e instanceof ApiError ? e.message : String(e))
    }
  }

  return (
    <div>
      <div className="langbar">
        <Input
          placeholder="搜索常用语"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          allowClear
          style={{ width: 240 }}
        />
        <div className="spacer" />
        <span className="pill">{items.length} 条</span>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          添加常用语
        </Button>
      </div>

      {loading ? (
        <div className="hint">加载中…</div>
      ) : shown.length === 0 ? (
        <Empty
          description={items.length === 0 ? '还没有常用语，点右上角添加' : '没有匹配的常用语'}
          style={{ marginTop: 48 }}
        />
      ) : (
        shown.map((it) => (
          <div className="phrase" key={it.id}>
            <div style={{ flex: 1, minWidth: 0 }}>
              {it.desc && <div className="desc">{it.desc}</div>}
              <div className="content">{it.content}</div>
            </div>
            <div className="ops">
              <Tooltip title="复制常用语">
                <button className="icon-btn" aria-label="复制常用语" type="button"
                  onClick={() => void api.copyText(it.content).then(
                    () => message.success('已复制'), () => message.error('复制失败，请手动选择文字'),
                  )}><CopyOutlined /></button>
              </Tooltip>
              <Tooltip title="插入到翻译框">
                <button className="icon-btn" aria-label="插入到翻译框" onClick={() => onInsert(it.content)} type="button">
                  <SendOutlined />
                </button>
              </Tooltip>
              <Tooltip title="编辑">
                <button className="icon-btn" aria-label="编辑常用语" onClick={() => openEdit(it)} type="button">
                  <EditOutlined />
                </button>
              </Tooltip>
              <Popconfirm title="删除这条常用语？" okText="删除" cancelText="取消" onConfirm={() => void remove(it)}>
                <button className="icon-btn danger" aria-label="删除常用语" type="button">
                  <DeleteOutlined />
                </button>
              </Popconfirm>
            </div>
          </div>
        ))
      )}

      <Modal
        title={editing ? '编辑常用语' : '添加常用语'}
        open={creating}
        onCancel={() => setCreating(false)}
        onOk={() => void save()}
        confirmLoading={saving}
        okText="保存"
        cancelText="取消"
        destroyOnHidden
      >
        <div className="row" style={{ marginTop: 12 }}>
          <label>备注</label>
          <Input
            value={desc}
            onChange={(e) => setDesc(e.target.value)}
            placeholder="例如：物流延迟说明"
            maxLength={40}
          />
        </div>
        <div className="row" style={{ alignItems: 'start' }}>
          <label style={{ paddingTop: 6 }}>内容</label>
          <Input.TextArea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            rows={6}
            placeholder="粘贴常用回复原文，翻译时会带上语境"
          />
        </div>
      </Modal>
    </div>
  )
}
