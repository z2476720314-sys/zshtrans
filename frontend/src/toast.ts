/**
 * antd 6 移除了 message 静态方法（必须走 App.useApp 的上下文实例）。
 * 这里做一个模块级桥：由 <ToastBridge/> 在 AntApp 内部绑定实例，
 * 其余组件直接 import { message } from '../toast' 即可，调用点无需改。
 */
import type { MessageInstance } from 'antd/es/message/interface'

let inst: MessageInstance | null = null

export function bindMessage(next: MessageInstance | null) {
  inst = next
}

type Content = Parameters<MessageInstance['success']>[0]

export const message = {
  open: (c: Parameters<MessageInstance['open']>[0]) => inst?.open(c),
  success: (c: Content) => inst?.success(c),
  error: (c: Content) => inst?.error(c),
  info: (c: Content) => inst?.info(c),
  warning: (c: Content) => inst?.warning(c),
  loading: (c: Content) => inst?.loading(c),
  destroy: (key?: React.Key) => inst?.destroy(key),
}

/** 供 <ToastBridge/> 使用；实例未绑定时调用是空操作，不会抛错 */
export function currentMessage(): MessageInstance | null {
  return inst
}
