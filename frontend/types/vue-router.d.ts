import type * as ElementPlusIconsVue from "@element-plus/icons-vue"
import type { SvgName } from "~virtual/svg-component"
import "vue-router"

export {}

type ElementPlusIconsName = keyof typeof ElementPlusIconsVue

declare module "vue-router" {
  interface RouteMeta {
    /** Display name in sidebar and breadcrumb */
    title?: string
    /** Route icon from SVG (import to src/common/assets/icons first) */
    svgIcon?: SvgName
    /** Route icon from Element Plus (svgIcon takes priority if both set) */
    elIcon?: ElementPlusIconsName
    /** Hide from sidebar when true */
    hidden?: boolean
    /** Roles allowed to access this route */
    roles?: string[]
    /** Show in breadcrumb (default true) */
    breadcrumb?: boolean
    /** Pin in tags-view when true */
    affix?: boolean
    /** Always show parent route in sidebar regardless of child count */
    alwaysShow?: boolean
    /** Highlight this sidebar item instead (e.g. activeMenu: "/other/path") */
    activeMenu?: string
    /** Cache route page (route and component name must match) */
    keepAlive?: boolean
  }
}
