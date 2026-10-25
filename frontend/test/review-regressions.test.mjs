import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import ts from 'typescript'

const settle = () => new Promise(setImmediate)
const text = (node) => Array.isArray(node) ? node.map(text).join('')
  : node && typeof node === 'object' ? text(node.props?.children) : String(node ?? '')
const elements = (node) => Array.isArray(node) ? node.flatMap(elements)
  : node && typeof node === 'object' ? [node, ...elements(node.props?.children)] : []

// Shallow hook runner: exercise shipped handlers without a DOM, network, or native actions.
function mount(file, api, props = {}) {
  const slots = [], effects = [], timers = new Map()
  let cursor = 0, first = true, timerID = 0, eventEnabled
  const react = {
    useState(initial) {
      const i = cursor++
      if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial
      return [slots[i], (next) => { slots[i] = typeof next === 'function' ? next(slots[i]) : next }]
    },
    useRef(initial) {
      const i = cursor++
      if (!(i in slots)) slots[i] = { current: initial }
      return slots[i]
    },
    useEffect(fn) { if (first) effects.push(fn) },
    useCallback: (fn) => fn,
    useMemo: (fn) => fn(),
  }
  const ui = Object.fromEntries(['Badge', 'Button', 'Dropdown', 'Empty', 'InputNumber', 'Modal',
    'Popconfirm', 'Progress', 'Select', 'Space', 'Spin', 'Switch', 'Table', 'Tag', 'Tooltip'].map((name) => [name, name]))
  ui.Input = Object.assign(function Input() {}, Object.fromEntries(['Search', 'Password'].map(name => [name, 'Input.' + name])))
  ui.Typography = { Paragraph: 'Paragraph', Text: 'Text' }
  const schedule = (fn) => { timers.set(++timerID, fn); return timerID }
  const jsx = (type, properties) => ({ type, props: properties })
  const context = {
    exports: {}, AbortController, HTMLInputElement: class {},
    setTimeout: schedule, clearTimeout: (id) => timers.delete(id),
    document: { addEventListener() {}, removeEventListener() {} },
    window: { location: { hash: '', pathname: '/', search: '' }, history: { replaceState() {} },
      addEventListener() {}, removeEventListener() {}, setInterval: schedule,
      clearInterval: (id) => timers.delete(id), setTimeout: schedule, clearTimeout: (id) => timers.delete(id) },
    require(name) {
      if (name === 'react') return react
      if (name === 'react/jsx-runtime') return { jsx, jsxs: jsx, Fragment: 'Fragment' }
      if (name === 'antd') return ui
      if (name === '@ant-design/icons') return new Proxy({}, { get: (_, key) => key })
      if (name.endsWith('/api')) return { api, ApiError: Error }
      if (name.endsWith('/toast')) return { message: { info() {}, warning() {}, error() {}, success() {} } }
      if (name.endsWith('/errors')) return { briefError: (error) => ({ title: String(error) }) }
      if (name.endsWith('/hooks')) return { useEvents: (_, enabled) => { eventEnabled = enabled; return false } }
      if (name.startsWith('./pages/') || name === './components/UpdatePanel') return { default: name.split('/').at(-1) }
      throw new Error(`Unexpected import: ${name}`)
    },
  }
  const source = readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8')
  vm.runInNewContext(ts.transpileModule(source, { compilerOptions: {
    target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX,
  } }).outputText, context)
  const render = () => { cursor = 0; return context.exports.default(props) }
  const find = (predicate) => {
    const found = elements(render()).find(predicate)
    assert.ok(found, 'Expected component control was not rendered')
    return found
  }
  render()
  first = false
  for (const effect of effects) effect()
  return { render, find, timers, eventsEnabled: () => eventEnabled,
    button: (label) => find((node) => ['Button', 'button'].includes(node.type) && text(node).trim() === label),
    field: (label) => find((node) => node.props?.['aria-label'] === label),
    async tick() {
      const entry = timers.entries().next().value
      assert.ok(entry, 'Expected polling to remain scheduled')
      timers.delete(entry[0])
      entry[1]()
      await settle()
    },
  }
}

const translationProps = { active: true, localAccess: false, languages: [], targetLanguages: [], onTranslated() {} }
const translated = { text: 'old translation', provider: 'test', ms: 1, src: 'en', tgt: 'zh' }

test('swap finishes before subsequent edits and cannot cancel a newer translation', async () => {
  let releaseSwap, finishTranslation, newestSignal
  const lateSwap = new Promise((resolve) => { releaseSwap = resolve })
  let requests = 0
  const page = mount('pages/Translate.tsx', {
    swap: () => lateSwap,
    translate: async (_text, _src, _tgt, signal) => {
      if (++requests === 1) return translated
      newestSignal = signal
      return new Promise((resolve) => { finishTranslation = resolve })
    },
  }, translationProps)
  page.field('原文').props.onChange({ target: { value: 'original' } })
  page.button('翻译 Ctrl+↵').props.onClick()
  await settle()
  page.field('交换语向').props.onClick()
  page.field('原文').props.onChange({ target: { value: 'new user input' } })
  page.button('翻译 Ctrl+↵').props.onClick()
  releaseSwap({ from: 'zh', to: 'en' })
  await settle()
  assert.equal(newestSignal.aborted, false)
  assert.equal(page.field('原文').props.value, 'new user input')
  finishTranslation({ ...translated, text: 'new translation' })
  await settle()
  assert.equal(text(page.field('译文')), 'new translation')
})

test('model discovery preserves a manual model added while the response is pending', async () => {
  let finishDiscovery
  const provider = { id: 'test', label: 'Test', key_ref: 'test', enabled: true, models: [{ id: 'existing' }] }
  const page = mount('pages/Settings.tsx', {
    config: async () => ({ providers: [provider], api_keys: {}, hotkeys: {} }),
    presets: async () => ({ groups: [], presets: [], protocols: { values: [] }, thinking: { styles: [] } }),
    models: () => new Promise((resolve) => { finishDiscovery = resolve }),
  }, { onSaved() {} })
  await settle()
  page.button('识别模型').props.onClick()
  page.field('Test 添加模型').props.onSearch('manual-new')
  finishDiscovery({ models: [{ id: 'catalogue-model' }] })
  await settle()
  const rows = page.find((node) => node.type === 'Table').props.dataSource
  assert.deepEqual(Array.from(rows, (row) => row.id), ['existing', 'manual-new', 'catalogue-model'])
})

test('skipping a downloaded release clears readiness and permits another manual check', async () => {
  const calls = []
  const page = mount('components/UpdatePanel.tsx', {
    updateCheck: async (force) => { calls.push(['check', force]); return { ok: true, has_new: true, latest: '3.5.0' } },
    updateApply: async (action, skip) => {
      calls.push([action, skip])
      return action === 'status' && !skip ? { busy: false, ready: true, done: 10, total: 10, error: '' } : { ok: true }
    },
  }, { auto: false })
  page.button('检查更新').props.onClick()
  await settle()
  page.button('下载更新').props.onClick()
  await settle()
  page.button('更新已就绪')
  page.button('跳过此版本').props.onClick()
  await settle()
  assert.deepEqual(calls.slice(-2), [['cancel', undefined], ['status', '3.5.0']])
  assert.equal(page.find((node) => node.type === 'Modal').props.open, false)
  page.button('检查更新').props.onClick()
  await settle()
  assert.equal(calls.filter(([action]) => action === 'check').length, 2)
})

test('a failed download cancellation retains polling until the completed package is shown', async () => {
  let ready = false
  const page = mount('components/UpdatePanel.tsx', {
    updateCheck: async () => ({ ok: true, has_new: true, latest: '3.5.0' }),
    updateApply: async (action) => {
      if (action === 'cancel') throw new Error('temporary disconnect')
      return action === 'status' ? { busy: !ready, ready, done: 1, total: 10, error: '' } : { ok: true }
    },
  }, { auto: false })
  page.button('检查更新').props.onClick()
  await settle()
  page.button('下载更新').props.onClick()
  await settle()
  page.button('取消下载').props.onClick()
  await settle()
  ready = true
  await page.tick()
  page.button('安装并重启')
  assert.equal(page.timers.size, 0)
})

test('local status gates management and native controls while LAN translation works without config', async () => {
  for (const local of [false, true]) {
    let saves = 0, translations = 0
    const api = {
      status: async () => ({ local, desktop: false, update_check: false, src: 'auto', tgt: 'zh', providers: [], cache: 0 }),
      languages: async () => ({ all: [], target: [] }),
      config: async () => { throw new Error('LAN config must not be requested') },
      saveSettings: async () => { saves++; return { ok: true } },
      translate: async () => { translations++; return translated },
    }
    const app = mount('App.tsx', api)
    await settle()
    const nodes = elements(app.render())
    assert.deepEqual(nodes.filter((node) => node.type === 'button').map(text), local ? ['翻译', '常用语', '设置', '自检'] : ['翻译'])
    assert.equal(nodes.some((node) => node.type === 'UpdatePanel'), local)
    assert.equal(app.eventsEnabled(), local)
    const props = app.find((node) => node.type === 'Translate').props
    assert.equal(props.localAccess, local)
    const page = mount('pages/Translate.tsx', api, props)
    assert.equal(elements(page.render()).some((node) => node.type === 'Button' && text(node).trim() === '截图取词'), local)
    page.field('译文语言').props.onChange('en')
    page.field('原文').props.onChange({ target: { value: 'hello' } })
    page.button('翻译 Ctrl+↵').props.onClick()
    await settle()
    assert.equal(saves, Number(local))
    assert.equal(translations, 1)
    assert.equal(text(page.field('译文')), translated.text)
  }
})
