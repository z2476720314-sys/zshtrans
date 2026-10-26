import { useEffect, useMemo, useState } from 'react'
import {
  Button,
  Dropdown,
  Empty,
  Input,
  InputNumber,
  Popconfirm,
  Select,
  Switch,
  Table,
  Tooltip,
} from 'antd'
import { message } from '../toast'
import {
  ApiOutlined,
  DeleteOutlined,
  DownOutlined,
  ExperimentOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  UndoOutlined,
} from '@ant-design/icons'
import {
  api,
  ApiError,
  type Config,
  type Hotkeys,
  type Model,
  type PresetsPayload,
  type Provider,
} from '../api'

const TABS = ['模型提供方', '截图与 OCR', '快捷键', '其他翻译接口', '更新'] as const

const TAB_HINTS: Record<(typeof TABS)[number], string> = {
  模型提供方: '密钥只写入本地，留空保留已保存值；所有修改在保存后生效。',
  '截图与 OCR': '从已启用并支持图片输入的模型中选择；保存时校验密钥。',
  快捷键: '组合键格式：ctrl+alt+t。系统占用冲突会在保存时显示。',
  其他翻译接口: '不回显已保存的密钥；留空保持不变。',
  更新: '自动检查只会提示新版本，由你决定是否下载、安装和重启。',
}

/** 其他接口密钥 8 项（对应 translate.py OTHER_KEYS） */
const OTHER_KEYS: { key: string; label: string }[] = [
  { key: 'deepl', label: 'DeepL Free' },
  { key: 'baidu_appid', label: '百度翻译 AppID' },
  { key: 'baidu_key', label: '百度翻译 密钥' },
  { key: 'niutrans', label: '小牛翻译 apikey' },
  { key: 'agnes', label: 'Agnes 2.5 Flash API Key' },
  { key: 'ocrspace', label: 'OCR.space key' },
  { key: 'baidu_ocr_ak', label: '百度OCR API Key' },
  { key: 'baidu_ocr_sk', label: '百度OCR Secret' },
]

const HOTKEY_FIELDS: { key: keyof Hotkeys; label: string }[] = [
  { key: 'selection', label: '划词翻译' },
  { key: 'screenshot', label: '截图翻译' },
  { key: 'window', label: '主窗口显隐' },
]

type Props = { onSaved: () => void }

export default function Settings({ onSaved }: Props) {
  const [tab, setTab] = useState<(typeof TABS)[number]>('模型提供方')
  const [cfg, setCfg] = useState<Config | null>(null)
  const [meta, setMeta] = useState<PresetsPayload | null>(null)
  const [providers, setProviders] = useState<Provider[]>([])
  const [keys, setKeys] = useState<Record<string, string>>({})
  const [hotkeys, setHotkeys] = useState<Hotkeys | null>(null)
  const [saving, setSaving] = useState(false)
  const [busy, setBusy] = useState('')

  const load = async () => {
    try {
      const [c, p] = await Promise.all([api.config(), api.presets()])
      setCfg(c)
      setMeta(p)
      setProviders(c.providers ?? [])
      setHotkeys(c.hotkeys)
      const blank: Record<string, string> = {}
      for (const k of Object.keys(c.api_keys ?? {})) blank[k] = ''
      setKeys(blank)
    } catch (e) {
      message.error(e instanceof ApiError ? e.message : String(e))
    }
  }

  useEffect(() => {
    void load()
  }, [])

  /** 可用于「默认模型」下拉：启用 + 有模型 + 已配置密钥 */
  const usableModels = useMemo(() => {
    const rows: { value: string; label: string }[] = []
    for (const p of providers) {
      if (!p.enabled) continue
      const hasKey = Boolean(cfg?.api_keys?.[p.key_ref] || keys[p.key_ref]?.trim())
      if (!hasKey && !p.key_env) continue
      for (const m of p.models ?? []) {
        rows.push({ value: `${p.id}:${m.id}`, label: `${p.label} · ${m.label || m.id}` })
      }
    }
    return rows
  }, [providers, cfg, keys])

  /** 视觉模型（截图 OCR 用） */
  const visionModels = useMemo(() => {
    const rows: { value: string; label: string }[] = []
    for (const p of providers) {
      if (!p.enabled) continue
      const hasKey = Boolean(cfg?.api_keys?.[p.key_ref] || keys[p.key_ref]?.trim())
      if (!hasKey && !p.key_env) continue
      for (const m of p.models ?? []) {
        if (!m.image) continue
        rows.push({ value: `${p.id}:${m.id}`, label: `${p.label} · ${m.label || m.id}` })
      }
    }
    return rows
  }, [providers, cfg, keys])

  const patch = (id: string, next: Partial<Provider>) => {
    setProviders((list) => list.map((p) => (p.id === id ? { ...p, ...next } : p)))
  }

  const patchModel = (pid: string, mid: string, next: Partial<Model>) => {
    setProviders((list) =>
      list.map((p) =>
        p.id !== pid
          ? p
          : { ...p, models: (p.models ?? []).map((m) => (m.id === mid ? { ...m, ...next } : m)) },
      ),
    )
  }

  const addProvider = (preset: Provider) => {
    if (providers.some((p) => p.id === preset.id)) {
      message.warning(`提供方 ${preset.id} 已存在`)
      return
    }
    setProviders((list) => [...list, { ...preset, enabled: true }])
    message.success(`已添加 ${preset.label}`)
  }

  const removeProvider = (id: string) => {
    setProviders((list) => list.filter((p) => p.id !== id))
  }

  const resetProvider = async (id: string) => {
    if (!cfg) return
    const orig = cfg.providers.find((p) => p.id === id)
    if (!orig) return
    patch(id, orig)
    message.success('已重置回已保存值')
  }

  const restoreSeed = async () => {
    if (!meta) return
    setProviders(meta.seed.map((p) => ({ ...p })))
    message.success('已恢复默认提供方')
  }

  const fetchModels = async (p: Provider) => {
    setBusy(p.id)
    try {
      const res = await api.models(p.id, keys[p.key_ref] ?? '')
      if (!res.models?.length) {
        message.warning('接口没有返回任何模型')
        return
      }
      // 目录元数据只补未知项，不覆盖手工选择（与原 apply_model_metadata 一致）
      const catalogue = new Map(res.models.map((m) => [m.id, m]))
      setProviders((current) => current.map((entry) => {
        if (entry.id !== p.id) return entry
        const rows = [...(entry.models ?? [])]
        const existing = new Set(rows.map((m) => m.id))
        for (const [id, incoming] of catalogue) {
          if (!existing.has(id)) {
            rows.push({
              id,
              label: incoming.label || id,
              image: incoming.image === true,
              reasoning:
                incoming.reasoning_levels?.includes(incoming.reasoning)
                  ? incoming.reasoning
                  : incoming.reasoning_levels?.[0] ?? 'off',
              thinking_param: incoming.thinking_param,
              thinking_budget: incoming.thinking_budget,
              temperature: incoming.temperature ?? 0.3,
              context_window: incoming.context_window,
              max_output_tokens: incoming.max_output_tokens,
              reasoning_levels: incoming.reasoning_levels,
              thinking_types: incoming.thinking_types,
              metadata_source: incoming.metadata_source ?? 'provider',
            })
          } else {
            const idx = rows.findIndex((m) => m.id === id)
            const cur = rows[idx]
            rows[idx] = {
              ...cur,
              image: incoming.image === true ? true : cur.image,
              context_window: incoming.context_window || cur.context_window,
              max_output_tokens: incoming.max_output_tokens || cur.max_output_tokens,
              reasoning_levels: incoming.reasoning_levels?.length
                ? incoming.reasoning_levels
                : cur.reasoning_levels,
              thinking_types: incoming.thinking_types ?? cur.thinking_types,
              metadata_source: incoming.metadata_source ?? 'provider',
            }
          }
        }
        return { ...entry, models: rows }
      }))
      message.success(`已获取 ${catalogue.size} 个模型，已有手动配置已保留`)
    } catch (e) {
      message.error(e instanceof ApiError ? e.message : String(e))
    } finally {
      setBusy('')
    }
  }

  const testProvider = async (p: Provider) => {
    const model = (p.models ?? [])[0]
    if (!model) {
      message.warning('该提供方没有模型')
      return
    }
    setBusy('test-' + p.id)
    try {
      const res = await api.test(p.id, model.id, keys[p.key_ref] ?? '')
      if (res.ok) message.success(`连通正常（${p.label} · ${model.id}）`)
      else message.error(res.error || '连通失败')
    } catch (e) {
      message.error(e instanceof ApiError ? e.message : String(e))
    } finally {
      setBusy('')
    }
  }

  const save = async () => {
    if (!cfg || !hotkeys) return
    setSaving(true)
    try {
      const res = await api.saveSettings({
        src_lang: cfg.src_lang,
        tgt_lang: cfg.tgt_lang,
        default_provider: cfg.default_provider,
        ocr_model: cfg.ocr_model,
        hotkeys,
        providers,
        api_keys: keys,
        update_check: cfg.update_check,
      })
      message.success(res.message)
      await load()
      onSaved()
    } catch (e) {
      message.error(e instanceof ApiError ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  const groupMenu = useMemo(() => {
    if (!meta) return []
    const byGroup = new Map<string, Provider[]>()
    for (const preset of meta.presets) {
      const g = preset.group || '其他'
      if (!byGroup.has(g)) byGroup.set(g, [])
      byGroup.get(g)!.push(preset)
    }
    const order = meta.groups?.length ? meta.groups : [...byGroup.keys()]
    return order
      .filter((g) => byGroup.has(g))
      .map((g) => ({
        key: g,
        label: g,
        children: byGroup.get(g)!.map((p) => ({
          key: `${g}/${p.id}`,
          label: p.label,
          onClick: () => addProvider(p),
        })),
      }))
  }, [meta, providers])

  if (!cfg || !hotkeys) {
    return <div className="hint">加载配置中…</div>
  }

  return (
    <div>
      <div className="langbar">
        <span className="pill">{TAB_HINTS[tab]}</span>
        <div className="spacer" />
        <Button icon={<ReloadOutlined />} onClick={() => void load()}>
          重新载入
        </Button>
        <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={() => void save()}>
          保存并生效
        </Button>
      </div>

      <div className="settings-grid">
        <div className="navlist">
          {TABS.map((t) => (
            <button key={t} className={tab === t ? 'on' : ''} onClick={() => setTab(t)} type="button">
              {t}
            </button>
          ))}
        </div>

        <div>
          {tab === '模型提供方' && (
            <>
              <div className="row">
                <label>默认翻译模型</label>
                <Select
                  value={cfg.default_provider || ''}
                  onChange={(v) => setCfg({ ...cfg, default_provider: v })}
                  options={[{ value: '', label: '自动路由（按健康度轮转）' }, ...usableModels]}
                  showSearch
                  optionFilterProp="label"
                  placeholder="自动路由"
                />
              </div>

              <div className="langbar" style={{ marginTop: 14 }}>
                <Dropdown menu={{ items: groupMenu }} trigger={['click']}>
                  <Button icon={<PlusOutlined />}>
                    添加提供方 <DownOutlined />
                  </Button>
                </Dropdown>
                <Popconfirm
                  title="恢复默认提供方？"
                  description="会重置为内置的百炼 + Acctoken 两张卡"
                  okText="恢复"
                  cancelText="取消"
                  onConfirm={() => void restoreSeed()}
                >
                  <Button icon={<UndoOutlined />}>恢复默认提供方</Button>
                </Popconfirm>
                <div className="spacer" />
                <span className="pill">{providers.length} 张卡</span>
              </div>

              {providers.length === 0 && <Empty description="还没有提供方，点上方添加" style={{ marginTop: 32 }} />}

              {providers.map((p) => (
                <div className="card" key={p.id}>
                  <div className="card-head">
                    <b>{p.label || p.id}</b>
                    <span className="tag mono">{p.id}</span>
                    {p.builtin && <span className="tag lime">内置</span>}
                    {!p.enabled && <span className="tag off">已禁用</span>}
                    <div className="spacer" />
                    <Tooltip title="拉取账号模型列表">
                      <Button
                        size="small"
                        icon={<ApiOutlined />}
                        loading={busy === p.id}
                        onClick={() => void fetchModels(p)}
                      >
                        识别模型
                      </Button>
                    </Tooltip>
                    <Tooltip title="连通性测试">
                      <Button
                        size="small"
                        icon={<ExperimentOutlined />}
                        loading={busy === 'test-' + p.id}
                        onClick={() => void testProvider(p)}
                      >
                        测试
                      </Button>
                    </Tooltip>
                    <Tooltip title="重置回已保存值">
                      <Button size="small" icon={<UndoOutlined />} onClick={() => void resetProvider(p.id)} />
                    </Tooltip>
                    <Tooltip title="删除该提供方">
                      <Popconfirm title="删除该提供方？" okText="删除" cancelText="取消" onConfirm={() => removeProvider(p.id)}>
                        <Button size="small" danger icon={<DeleteOutlined />} />
                      </Popconfirm>
                    </Tooltip>
                  </div>

                  <div className="row">
                    <label>名称</label>
                    <Input value={p.label} onChange={(e) => patch(p.id, { label: e.target.value })} />
                  </div>
                  <div className="row">
                    <label>接口地址</label>
                    <Input
                      value={p.base_url}
                      onChange={(e) => patch(p.id, { base_url: e.target.value })}
                      placeholder="https://api.example.com/v1"
                    />
                  </div>
                  <div className="row">
                    <label>协议</label>
                    <Select
                      value={p.protocol}
                      onChange={(v) => patch(p.id, { protocol: v })}
                      options={(meta?.protocols.values ?? []).map((v) => ({
                        value: v,
                        label: meta?.protocols.labels?.[v] || v,
                      }))}
                    />
                  </div>
                  <div className="row">
                    <label>密钥</label>
                    <Input.Password
                      value={keys[p.key_ref] ?? ''}
                      onChange={(e) => setKeys({ ...keys, [p.key_ref]: e.target.value })}
                      placeholder={
                        cfg.api_keys?.[p.key_ref]
                          ? cfg.key_saved
                          : p.key_env
                            ? `读取环境变量 ${p.key_env}`
                            : cfg.key_missing
                      }
                    />
                  </div>
                  <div className="row">
                    <label>思考风格</label>
                    <Select
                      value={p.thinking_style}
                      onChange={(v) => patch(p.id, { thinking_style: v })}
                      options={(meta?.thinking.styles ?? []).map((v) => ({
                        value: v,
                        label: meta?.thinking.labels?.[v] || v,
                      }))}
                    />
                  </div>
                  <div className="row">
                    <label>超时 / 流式</label>
                    <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
                      <InputNumber
                        min={1}
                        max={300}
                        value={p.timeout}
                        onChange={(v) => patch(p.id, { timeout: Number(v) || 20 })}
                        suffix="秒"
                        style={{ width: 130 }}
                      />
                      <Switch
                        checked={p.stream}
                        onChange={(v) => patch(p.id, { stream: v })}
                        checkedChildren="流式"
                        unCheckedChildren="整包"
                      />
                      <Switch
                        checked={p.enabled}
                        onChange={(v) => patch(p.id, { enabled: v })}
                        checkedChildren="启用"
                        unCheckedChildren="禁用"
                      />
                    </div>
                  </div>

                  <Table<Model>
                    scroll={{ x: 780 }}
                    size="small"
                    rowKey="id"
                    pagination={false}
                    dataSource={p.models ?? []}
                    style={{ marginTop: 10 }}
                    locale={{ emptyText: '还没有模型，点「识别模型」拉取或手动添加' }}
                    columns={[
                      {
                        title: '模型 ID',
                        dataIndex: 'id',
                        render: (v: string) => <span className="mono">{v}</span>,
                      },
                      {
                        title: '显示名',
                        dataIndex: 'label',
                        width: 160,
                        render: (v: string, m) => (
                          <Input
                            size="small"
                            value={v || ''}
                            placeholder={m.id}
                            onChange={(e) => patchModel(p.id, m.id, { label: e.target.value })}
                          />
                        ),
                      },
                      {
                        title: '图片',
                        dataIndex: 'image',
                        width: 64,
                        render: (v: boolean, m) => (
                          <Switch size="small" checked={v} onChange={(x) => patchModel(p.id, m.id, { image: x })} />
                        ),
                      },
                      {
                        title: '推理档',
                        dataIndex: 'reasoning',
                        width: 128,
                        render: (v: string, m) => {
                          // 目录返回的档位优先，否则退回全量档位
                          const levels = m.reasoning_levels?.length
                            ? m.reasoning_levels
                            : meta?.reasoning.values ?? []
                          return (
                            <Select
                              size="small"
                              value={v}
                              style={{ width: 112 }}
                              onChange={(x) => patchModel(p.id, m.id, { reasoning: x })}
                              options={levels.map((r) => ({
                                value: r,
                                label: meta?.reasoning.labels?.[r] || r,
                              }))}
                            />
                          )
                        },
                      },
                      {
                        title: '输出上限',
                        dataIndex: 'output_limit',
                        width: 116,
                        render: (v: number | undefined, m) => <InputNumber size="small" min={1}
                          aria-label={m.id + ' 输出上限'} placeholder="自动" value={v || undefined}
                          max={m.max_output_tokens || undefined} style={{ width: 100 }}
                          onChange={(x) => patchModel(p.id, m.id, { output_limit: x || 0 })} />,
                      },
                      {
                        title: '温度',
                        dataIndex: 'temperature',
                        width: 96,
                        render: (v: number | null, m) => (
                          <InputNumber
                            size="small"
                            min={0}
                            max={2}
                            step={0.1}
                            style={{ width: 82 }}
                            value={v ?? undefined}
                            onChange={(x) => patchModel(p.id, m.id, { temperature: x === null ? null : Number(x) })}
                          />
                        ),
                      },
                      {
                        title: '',
                        width: 46,
                        render: (_v, m) => (
                          <Button
                            size="small"
                            type="text"
                            danger
                            icon={<DeleteOutlined />}
                            onClick={() =>
                              patch(p.id, { models: (p.models ?? []).filter((x) => x.id !== m.id) })
                            }
                          />
                        ),
                      },
                    ]}
                  />
                  <Input.Search className="manual-model" aria-label={p.label + ' 添加模型'}
                    placeholder="输入模型 ID，手动添加" enterButton="添加模型"
                    onSearch={(value, event) => {
                      const id = value.trim()
                      if (!id) return
                      if (p.models.some((m) => m.id === id)) { message.warning('该模型已存在'); return }
                      patch(p.id, { models: [...p.models, { id, label: id, image: false, reasoning: 'off', temperature: null }] })
                      if (event?.target instanceof HTMLInputElement) event.target.value = ''
                    }} />
                </div>
              ))}
            </>
          )}

          {tab === '截图与 OCR' && (
            <>
              <div className="row">
                <label>识别模型</label>
                <Select
                  value={cfg.ocr_model || ''}
                  onChange={(v) => setCfg({ ...cfg, ocr_model: v })}
                  options={[{ value: '', label: cfg.ocr_unset }, ...visionModels]}
                  showSearch
                  optionFilterProp="label"
                  placeholder={cfg.ocr_unset}
                />
              </div>
              <div className="hint">
                截图取词会并行尝试视觉模型 / OCR.space / 百度 OCR，按结果长度取中位数择优；没配视觉模型也能用另外两个引擎。
              </div>
              <div className="row" style={{ marginTop: 16 }}>
                <label>OCR.space key</label>
                <Input.Password
                  value={keys['ocrspace'] ?? ''}
                  onChange={(e) => setKeys({ ...keys, ocrspace: e.target.value })}
                  placeholder={cfg.api_keys?.['ocrspace'] ? cfg.key_saved : cfg.key_missing}
                />
              </div>
              <div className="row">
                <label>百度 OCR AK</label>
                <Input.Password
                  value={keys['baidu_ocr_ak'] ?? ''}
                  onChange={(e) => setKeys({ ...keys, baidu_ocr_ak: e.target.value })}
                  placeholder={cfg.api_keys?.['baidu_ocr_ak'] ? cfg.key_saved : cfg.key_missing}
                />
              </div>
              <div className="row">
                <label>百度 OCR SK</label>
                <Input.Password
                  value={keys['baidu_ocr_sk'] ?? ''}
                  onChange={(e) => setKeys({ ...keys, baidu_ocr_sk: e.target.value })}
                  placeholder={cfg.api_keys?.['baidu_ocr_sk'] ? cfg.key_saved : cfg.key_missing}
                />
              </div>
            </>
          )}

          {tab === '快捷键' && (
            <>
              {HOTKEY_FIELDS.map((f) => (
                <div className="row" key={f.key}>
                  <label>{f.label}</label>
                  <Input
                    className="mono"
                    value={hotkeys[f.key]}
                    onChange={(e) => setHotkeys({ ...hotkeys, [f.key]: e.target.value })}
                    placeholder="ctrl+alt+t"
                  />
                </div>
              ))}
              <div className="hint">
                组合键用 + 连接，修饰键写在前（ctrl / alt / shift / win）。主窗口显隐使用同一个快捷键。
              </div>
            </>
          )}

          {tab === '其他翻译接口' && (
            <>
              {OTHER_KEYS.map((k) => (
                <div className="row" key={k.key}>
                  <label>{k.label}</label>
                  <Input.Password
                    value={keys[k.key] ?? ''}
                    onChange={(e) => setKeys({ ...keys, [k.key]: e.target.value })}
                    placeholder={cfg.api_keys?.[k.key] ? cfg.key_saved : cfg.key_missing}
                  />
                </div>
              ))}
              <div className="hint">
                这些是 9 个内置直连提供方使用的密钥；不填时对应提供方会直接失败并自动降级到下一个。
              </div>
            </>
          )}
          {tab === '更新' && <div className="row">
            <label htmlFor="auto-update">启动时检查新版本</label>
            <Switch id="auto-update" checked={cfg.update_check}
              onChange={(checked) => setCfg({ ...cfg, update_check: checked })} />
          </div>}
        </div>
      </div>
    </div>
  )
}
