/* eslint-disable perfectionist/sort-imports */

// Core
import { pinia } from "@/pinia"
import { router } from "@/router"
import { i18n } from "@/locales"
import { installPlugins } from "@/plugins"
import App from "@/App.vue"
// CSS
import "normalize.css"
import "nprogress/nprogress.css"
import "@@/assets/styles/index.scss"
import "virtual:uno.css"

// Create app instance
const app = createApp(App)

// Install plugins (global components, custom directives, etc.)
installPlugins(app)

// Install i18n, pinia and router
app.use(i18n).use(pinia).use(router)

// Mount app after router is ready
router.isReady().then(() => {
  app.mount("#app")
})
