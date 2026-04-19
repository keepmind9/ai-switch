import type { Router } from "vue-router"
import { setRouteChange } from "@@/composables/useRouteListener"
import { useTitle } from "@@/composables/useTitle"
import NProgress from "nprogress"

NProgress.configure({ showSpinner: false })

const { setTitle } = useTitle()

export function registerNavigationGuard(router: Router) {
  router.afterEach((to) => {
    setRouteChange(to)
    setTitle(to.meta.title)
    NProgress.done()
  })
}
