import type { App } from "vue"
import * as ElementPlusIconsVue from "@element-plus/icons-vue"

export function installElementPlusIcons(app: App) {
  // Register all Element Plus Icons
  for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component)
  }
}
