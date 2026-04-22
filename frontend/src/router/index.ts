import type { RouteRecordRaw } from "vue-router"
import { createRouter } from "vue-router"
import { routerConfig } from "@/router/config"

const Layouts = () => import("@/layouts/index.vue")

export const constantRoutes: RouteRecordRaw[] = [
  {
    path: "/redirect",
    component: Layouts,
    meta: { hidden: true },
    children: [{ path: ":path(.*)", component: () => import("@/pages/redirect/index.vue") }]
  },
  {
    path: "/403",
    component: () => import("@/pages/error/403.vue"),
    meta: { hidden: true }
  },
  {
    path: "/404",
    component: () => import("@/pages/error/404.vue"),
    meta: { hidden: true },
    alias: "/:pathMatch(.*)*"
  },
  {
    path: "/",
    component: Layouts,
    redirect: "/dashboard",
    children: [
      {
        path: "dashboard",
        component: () => import("@/pages/dashboard/index.vue"),
        name: "Dashboard",
        meta: { title: "dashboard", svgIcon: "dashboard", affix: true }
      }
    ]
  },
  {
    path: "/providers",
    component: Layouts,
    children: [
      {
        path: "",
        component: () => import("@/pages/providers/index.vue"),
        name: "Providers",
        meta: { title: "providers", elIcon: "Connection" }
      }
    ]
  },
  {
    path: "/routes",
    component: Layouts,
    children: [
      {
        path: "",
        component: () => import("@/pages/routes/index.vue"),
        name: "Routes",
        meta: { title: "routes", elIcon: "Key" }
      }
    ]
  },
  {
    path: "/stats",
    component: Layouts,
    children: [
      {
        path: "",
        component: () => import("@/pages/stats/index.vue"),
        name: "Stats",
        meta: { title: "stats", elIcon: "DataAnalysis" }
      }
    ]
  }
]

export const router = createRouter({
  history: routerConfig.history,
  routes: constantRoutes
})
