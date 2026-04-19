import type { RouterHistory } from "vue-router"
import { createWebHashHistory, createWebHistory } from "vue-router"

/** Router config */
interface RouterConfig {
  /**
   * @name Router mode
   * @description hash mode and html5 mode
   */
  history: RouterHistory
  /**
   * @name Whether to enable dynamic routing
   * @description 1. When enabled, backend support is required to return a field (e.g., roles) for determining and loading dynamic routes
   * @description 2. If the project does not need to show different pages for different users, set dynamic: false
   */
  dynamic: boolean
  /**
   * @name Default roles
   * @description When dynamic routing is disabled:
   * @description 1. All routes should be written as constant routes (all logged-in users see the same pages)
   * @description 2. The system automatically assigns a default role with no effect to the current user
   */
  defaultRoles: Array<string>
  /**
   * @name Whether to enable third-level and above route caching
   * @description 1. When enabled, route downgrade is performed (converting third-level and above routes to second-level routes)
   * @description 2. Since all routes are converted to second-level, nested child routes at second-level and above will not work
   */
  thirdLevelRouteCache: boolean
}

const VITE_ROUTER_HISTORY = import.meta.env.VITE_ROUTER_HISTORY

const VITE_PUBLIC_PATH = import.meta.env.VITE_PUBLIC_PATH

export const routerConfig: RouterConfig = {
  history: VITE_ROUTER_HISTORY === "hash" ? createWebHashHistory(VITE_PUBLIC_PATH) : createWebHistory(VITE_PUBLIC_PATH),
  dynamic: true,
  defaultRoles: ["DEFAULT_ROLE"],
  thirdLevelRouteCache: false
}
