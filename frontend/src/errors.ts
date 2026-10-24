/**
 * 错误信息压缩。
 *
 * 后端在多提供方全失败时返回的是 `all providers failed:\n<逐条原因>`，
 * 每条原因里都带完整请求 URL（含百分号编码的原文），整段直接丢给 antd
 * message 会把页面顶部撑成一堵墙。这里把它压成「一行摘要 + 可展开详情」。
 */

import { ApiError } from './api'

export type BriefError = {
  /** 一行摘要，用于 toast */
  title: string
  /** 完整原文，用于面板内展开查看 */
  detail: string
  /** 逐条失败原因（后端按行给出时才有） */
  items: string[]
}

/** 把过长的 URL 查询串折叠掉，只保留路径 */
function foldURL(s: string): string {
  return s.replace(/(https?:\/\/[^\s?]+)\?[^\s]*/g, '$1?…')
}

/**
 * 折叠内联 HTML（Cloudflare 拦截页、网关错误页会把整段 <!DOCTYPE html> 塞进错误里）。
 * 只保留第一个标签名做线索，例如 `<html…>`。
 */
function foldHTML(s: string): string {
  return s.replace(/<(\/?)([a-zA-Z][a-zA-Z0-9-]*)[^>]*>/g, (_, slash, tag) => `<${slash}${tag}>`)
}

/** 去掉多余的分号堆叠与空白，让摘要像一句话 */
function tidy(s: string): string {
  return s
    .replace(/\s*;\s*/g, '; ')
    .replace(/\s*:\s*/g, ': ')
    .replace(/\s+/g, ' ')
    .trim()
}

/** 压成单行，并把连续空白折成一个空格 */
function oneLine(s: string): string {
  return s.replace(/\s+/g, ' ').trim()
}

/** 截断到 n 个字符（按码点，避免切坏 emoji / 中文） */
function clip(s: string, n: number): string {
  const r = Array.from(s)
  return r.length <= n ? s : r.slice(0, n).join('') + '…'
}

export function briefError(e: unknown): BriefError {
  const raw = e instanceof ApiError ? e.message : String(e)
  const detail = raw.trim() || '未知错误'
  const lines = detail
    .split('\n')
    .map((l) => tidy(oneLine(foldHTML(foldURL(l)))))
    .filter(Boolean)

  // 形如 `all providers failed:` 的首行是分组标题，真正的原因在后面的行里
  const head = lines[0] || '未知错误'
  const isGroup = /failed:?$|failed:$/i.test(head) || lines.length > 1
  const items = isGroup ? lines.slice(1) : lines

  let title: string
  if (isGroup && items.length > 0) {
    // 摘要只给「有几项失败 + 第一条的短因」，完整内容在右栏展开看
    const first = clip(items[0], 44)
    title = `翻译失败（${items.length} 项）：${first}`
  } else {
    title = clip(head, 90)
  }
  return { title, detail, items }
}
