import test from 'node:test'
import assert from 'node:assert/strict'
import { api } from '../src/api.ts'

test('desktop translation uses native messaging and forwards cancellation', async () => {
  const originalWindow = globalThis.window, originalFetch = globalThis.fetch
  const target = new EventTarget()
  let request, cancel
  globalThis.window = target
  target.zshDesktop = {
    request: async (id, method, path, body) => { request = { id, method, path, body } },
    cancel: async (id) => { cancel = id },
  }
  globalThis.fetch = () => { throw new Error('Desktop mode must never use a network fetch') }
  try {
    const controller = new AbortController()
    const pending = api.translate('Привет', 'ru', 'zh', controller.signal)
    pending.catch(() => {})
    assert.equal(request?.path, '/api/translate')
    assert.equal(request.method, 'POST')
    assert.equal(JSON.parse(request.body).text, 'Привет')
    controller.abort()
    await assert.rejects(pending, { name: 'AbortError' })
    assert.equal(cancel, request.id)
    target.dispatchEvent(new CustomEvent('zsh-response', { detail: { id: request.id, status: 200, body: '{}' } }))
  } finally {
    globalThis.window = originalWindow
    globalThis.fetch = originalFetch
  }
})

test('desktop native reply preserves normal API error handling', async () => {
  const originalWindow = globalThis.window
  const target = new EventTarget()
  globalThis.window = target
  target.zshDesktop = {
    request: async (id) => { queueMicrotask(() => target.dispatchEvent(new CustomEvent('zsh-response', { detail: { id, status: 400, body: '{"error":"invalid hotkey"}' } }))) },
    cancel: async () => {},
  }
  try { await assert.rejects(api.config(), /invalid hotkey/) }
  finally { globalThis.window = originalWindow }
})

test('desktop reload ignores a late reply from the previous document', async () => {
  const originalWindow = globalThis.window
  const before = new EventTarget(), after = new EventTarget()
  const controller = new AbortController()
  let oldID, newID, oldPending
  before.zshDesktop = { request: async (id) => { oldID = id }, cancel: async () => {} }
  after.zshDesktop = { request: async (id) => { newID = id }, cancel: async () => {} }
  try {
    globalThis.window = before
    const { api: oldApi } = await import('../src/api.ts?reload=before')
    oldPending = oldApi.translate('old text', 'en', 'zh', controller.signal)
    oldPending.catch(() => {})
    globalThis.window = after
    const { api: newApi } = await import('../src/api.ts?reload=after')
    const fresh = newApi.config()
    let settled = false
    void fresh.then(() => { settled = true }, () => { settled = true })
    after.dispatchEvent(new CustomEvent('zsh-response', { detail: { id: oldID, status: 200, body: '{"text":"old translation"}' } }))
    await new Promise((resolve) => setImmediate(resolve))
    assert.equal(settled, false, 'a stale translation must not resolve the new config request')
    after.dispatchEvent(new CustomEvent('zsh-response', { detail: { id: newID, status: 200, body: '{"version":"new document"}' } }))
    assert.equal((await fresh).version, 'new document')
  } finally {
    globalThis.window = before
    controller.abort()
    await oldPending?.catch(() => {})
    globalThis.window = originalWindow
  }
})

test('translation passes cancellation to the network request', async () => {
  const original = globalThis.fetch
  const controller = new AbortController()
  globalThis.fetch = async (_url, init) => {
    assert.equal(init.signal, controller.signal)
    return new Response(JSON.stringify({ text: '订单已发出', provider: 'test', ms: 1, src: 'ru', tgt: 'zh' }))
  }
  try {
    const result = await api.translate('Заказ отправлен', 'ru', 'zh', controller.signal)
    assert.equal(result.text, '订单已发出')
  } finally {
    globalThis.fetch = original
  }
})

test('selection preserves detected and target language in its response', async () => {
  const original = globalThis.fetch
  const controller = new AbortController()
  globalThis.fetch = async (_url, init) => {
    assert.equal(init.signal, controller.signal)
    return new Response(JSON.stringify({ source: 'Заказ отправлен', text: '订单已发出', src: 'ru', tgt: 'zh', provider: 'test', ms: 1 }))
  }
  try {
    const result = await api.selection(controller.signal)
    assert.equal(result.src, 'ru')
    assert.equal(result.tgt, 'zh')
  } finally {
    globalThis.fetch = original
  }
})
