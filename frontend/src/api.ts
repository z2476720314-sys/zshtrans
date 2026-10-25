/** 与 Go 后端通信的极薄封装：所有接口集中在此，组件不直接 fetch */

export type LangOption = { value: string; label: string }

export type ProviderStatus = { name: string; label: string; healthy: boolean }

export type Status = {
  app: string
  version: string
  providers: ProviderStatus[]
  src: string
  tgt: string
  default: string
  cache: number
  lan: string
  desktop: boolean
  update_check: boolean
  local: boolean
}

export type TranslateResult = {
  text: string
  provider: string
  ms: number
  src: string
  tgt: string
}

export type CaptureResult = TranslateResult & {
  source: string
  engine?: string
  action?: 'translate' | 'ocr' | 'complete' | 'cancel'
}

export type Model = {
  id: string
  label?: string
  image: boolean
  reasoning: string
  thinking_param?: string
  thinking_budget?: number
  temperature: number | null
  output_limit?: number
  /** 以下为模型目录返回的元数据，缺省表示未知 */
  context_window?: number
  max_output_tokens?: number
  reasoning_levels?: string[]
  thinking_types?: string[]
  metadata_source?: string
}

export type Provider = {
  id: string
  preset_id?: string
  label: string
  group?: string
  base_url: string
  protocol: string
  enabled: boolean
  builtin?: boolean
  key_ref: string
  key_env: string
  thinking_style: string
  timeout: number
  stream: boolean
  models: Model[]
  docs_url?: string
  notice?: string
}

export type Hotkeys = {
  selection: string
  screenshot: string
  window: string
  close: string
}

export type WebCfg = { enabled: boolean; host: string; port: number }

export type Config = {
  app: string
  version: string
  src_lang: string
  tgt_lang: string
  default_provider: string
  ocr_model: string
  hotkeys: Hotkeys
  web: WebCfg
  providers: Provider[]
  api_keys: Record<string, string>
  key_saved: string
  key_missing: string
  ocr_unset: string
  /** 启动时自动检查更新 */
  update_check: boolean
  /** 覆盖默认 GitHub 仓库 owner/name */
  update_repo: string
  /** 已选择「跳过此版本」的版本号 */
  update_skipped: string
}

export type Preset = Provider & { description?: string }

export type PresetsPayload = {
  groups: string[]
  presets: Preset[]
  seed: Provider[]
  protocols: { values: string[]; labels: Record<string, string> }
  reasoning: { values: string[]; labels: Record<string, string> }
  thinking: { params: string[]; styles: string[]; labels: Record<string, string> }
}

export type Phrase = { id: string; desc: string; content: string }

export type SelfCheck = {
  app: string
  version: string
  language_count: number
  provider_preset_count: number
  direct_provider_count: number
  ui_toolkit: string
  phrases_count: number
  phrases_sha256: string
  font_glyphs_ok: boolean | null
  providers: number
  usable: number
  cache: number
  config_path: string
  phrases_path: string
  log_path: string
  ok: boolean
}

export type UpdateInfo = {
  ok: boolean
  error?: string
  current: string
  latest?: string
  tag?: string
  name?: string
  notes?: string
  page?: string
  size?: number
  has_new?: boolean
  skipped?: boolean
}

export type UpdateProgress = {
  ok: boolean
  busy: boolean
  done: number
  total: number
  ready: boolean
  error: string
}

export class ApiError extends Error {}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  const raw = await res.text()
  let data: unknown = null
  if (raw) {
    try {
      data = JSON.parse(raw)
    } catch {
      data = { error: raw }
    }
  }
  if (!res.ok) {
    const msg =
      data && typeof data === 'object' && 'error' in data
        ? String((data as { error: unknown }).error)
        : `HTTP ${res.status}`
    throw new ApiError(msg)
  }
  return data as T
}

export const api = {
  status: () => call<Status>('/api/status'),
  translate: (text: string, src: string, tgt: string, signal?: AbortSignal) =>
    call<TranslateResult>('/api/translate', {
      method: 'POST',
      body: JSON.stringify({ text, src, tgt }),
      signal,
    }),
  config: () => call<Config>('/api/config'),
  saveSettings: (body: Record<string, unknown>) =>
    call<{ ok: boolean; message: string; usable: number }>('/api/settings', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  languages: () =>
    call<{ all: LangOption[]; target: LangOption[]; flip: Record<string, string> }>(
      '/api/languages',
    ),
  presets: () => call<PresetsPayload>('/api/presets'),
  models: (providerID: string, key: string) =>
    call<{ models: Model[] }>('/api/models', {
      method: 'POST',
      body: JSON.stringify({ provider_id: providerID, key }),
    }),
  test: (providerID: string, modelID: string, key: string) =>
    call<{ ok: boolean; ms?: number; error?: string }>('/api/test', {
      method: 'POST',
      body: JSON.stringify({ provider_id: providerID, model_id: modelID, key }),
    }),
  phrases: () => call<{ items: Phrase[] }>('/api/phrases'),
  phrase: (body: Record<string, unknown>) =>
    call<{ ok?: boolean; items: Phrase[] }>('/api/phrases', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  ocr: (image: string) =>
    call<{ text: string; engine: string }>('/api/ocr', {
      method: 'POST',
      body: JSON.stringify({ image }),
    }),
  capture: (region?: { x: number; y: number; w: number; h: number }, signal?: AbortSignal) =>
    call<CaptureResult>(
      '/api/capture',
      {
        method: 'POST',
        body: JSON.stringify(region ?? {}),
        signal,
      },
    ),
  selection: (signal?: AbortSignal) =>
    call<CaptureResult>('/api/selection', {
      method: 'POST',
      body: '{}',
      signal,
    }),
  selfcheck: () => call<SelfCheck>('/api/selfcheck'),
  updateCheck: (force = false) =>
    call<UpdateInfo>(`/api/update/check${force ? '?force=1' : ''}`),
  updateApply: (action: 'download' | 'install' | 'status' | 'cancel', skip = '') =>
    call<{ ok: boolean; state?: string; skipped?: string }>('/api/update/apply', {
      method: 'POST',
      body: JSON.stringify({ action, skip }),
    }),
  swap: (from: string, to: string, detected?: string) =>
    call<{ from: string; to: string }>(
      `/api/swap?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}` +
        (detected ? `&detected=${encodeURIComponent(detected)}` : ''),
    ),
}
