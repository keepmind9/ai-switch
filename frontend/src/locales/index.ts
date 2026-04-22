import { createI18n } from "vue-i18n"
import { getLanguage } from "@@/utils/local-storage"
import enLocale from "./en.json"
import zhCnLocale from "./zh-cn.json"
import enElement from "element-plus/es/locale/lang/en"
import zhCnElement from "element-plus/es/locale/lang/zh-cn"

const messages = {
  en: {
    ...enLocale,
    ...enElement
  },
  "zh-cn": {
    ...zhCnLocale,
    ...zhCnElement
  }
}

export const i18n = createI18n({
  legacy: false,
  locale: getLanguage(),
  fallbackLocale: "en",
  messages
})

export const elementLocales: Record<string, any> = {
  en: enElement,
  "zh-cn": zhCnElement
}
