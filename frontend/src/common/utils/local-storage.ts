import type { ThemeName } from "@@/composables/useTheme"
import type { SidebarClosed, SidebarOpened } from "@@/constants/app-key"
import type { LayoutsConfig } from "@/layouts/config"
import { CacheKey } from "@@/constants/cache-key"

// Layout config
export function getLayoutsConfig() {
  const json = localStorage.getItem(CacheKey.CONFIG_LAYOUT)
  return json ? (JSON.parse(json) as LayoutsConfig) : null
}

export function setLayoutsConfig(settings: LayoutsConfig) {
  localStorage.setItem(CacheKey.CONFIG_LAYOUT, JSON.stringify(settings))
}

export function removeLayoutsConfig() {
  localStorage.removeItem(CacheKey.CONFIG_LAYOUT)
}

// Sidebar status
export function getSidebarStatus() {
  return localStorage.getItem(CacheKey.SIDEBAR_STATUS)
}

export function setSidebarStatus(sidebarStatus: SidebarOpened | SidebarClosed) {
  localStorage.setItem(CacheKey.SIDEBAR_STATUS, sidebarStatus)
}

// Active theme
export function getActiveThemeName() {
  return localStorage.getItem(CacheKey.ACTIVE_THEME_NAME) as ThemeName | null
}

export function setActiveThemeName(themeName: ThemeName) {
  localStorage.setItem(CacheKey.ACTIVE_THEME_NAME, themeName)
}

// Language
export function getLanguage() {
  const cacheLang = localStorage.getItem(CacheKey.LANGUAGE)
  if (cacheLang) return cacheLang
  // Browser language
  const browserLang = navigator.language.toLowerCase()
  if (browserLang.includes("zh")) return "zh-cn"
  return "en"
}

export function setLanguage(lang: string) {
  localStorage.setItem(CacheKey.LANGUAGE, lang)
}
