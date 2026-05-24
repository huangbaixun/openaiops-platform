import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'

const stored = localStorage.getItem('locale') as 'zh-CN' | 'en-US' | null
const browser = navigator.language.startsWith('zh') ? 'zh-CN' : 'en-US'
const locale = stored ?? browser

export default createI18n({
  legacy: false,
  locale,
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
})

export function setLocale(l: 'zh-CN' | 'en-US') {
  localStorage.setItem('locale', l)
  location.reload()
}
