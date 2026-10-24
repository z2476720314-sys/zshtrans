import React from 'react'
import { createRoot } from 'react-dom/client'
import { ConfigProvider, App as AntApp, theme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import App from './App'
import { bindMessage } from './toast'
import './styles.css'

/** 把 AntApp 上下文里的 message 实例桥接给模块级 toast，供任意组件直接调用 */
function ToastBridge() {
  const { message } = AntApp.useApp()
  React.useEffect(() => {
    bindMessage(message)
    return () => bindMessage(null)
  }, [message])
  return null
}

const root = createRoot(document.getElementById('root')!)

root.render(
  <React.StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          colorPrimary: '#B8FF3C',
          colorInfo: '#B8FF3C',
          colorSuccess: '#B8FF3C',
          colorError: '#FF5C7A',
          colorWarning: '#FFC857',
          colorBgBase: '#0B0C10',
          colorBgContainer: '#1C1F28',
          colorBgElevated: '#232733',
          colorBorder: '#2C313D',
          colorText: '#F2F3EE',
          colorTextSecondary: '#8B90A0',
          borderRadius: 10,
          fontFamily:
            "'Inter','Segoe UI','Microsoft YaHei',system-ui,-apple-system,sans-serif",
        },
      }}
    >
      <AntApp>
        <ToastBridge />
        <App />
      </AntApp>
    </ConfigProvider>
  </React.StrictMode>,
)
