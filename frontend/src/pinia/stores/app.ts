import { DeviceEnum, SIDEBAR_CLOSED, SIDEBAR_OPENED } from "@@/constants/app-key"
import { getSidebarStatus, setSidebarStatus, getLanguage, setLanguage as setLocalStorageLanguage } from "@@/utils/local-storage"
import { pinia } from "@/pinia"
import { i18n } from "@/locales"

interface Sidebar {
  opened: boolean
  withoutAnimation: boolean
}

/** Set sidebar status in local storage */
function handleSidebarStatus(opened: boolean) {
  opened ? setSidebarStatus(SIDEBAR_OPENED) : setSidebarStatus(SIDEBAR_CLOSED)
}

export const useAppStore = defineStore("app", () => {
  // Sidebar state
  const sidebar: Sidebar = reactive({
    opened: getSidebarStatus() !== SIDEBAR_CLOSED,
    withoutAnimation: false
  })

  // Device type
  const device = ref<DeviceEnum>(DeviceEnum.Desktop)

  // Language
  const language = ref<string>(getLanguage())

  // Watch sidebar opened state
  watch(
    () => sidebar.opened,
    (opened) => {
      handleSidebarStatus(opened)
    }
  )

  // Toggle sidebar
  const toggleSidebar = (withoutAnimation: boolean) => {
    sidebar.opened = !sidebar.opened
    sidebar.withoutAnimation = withoutAnimation
  }

  // Close sidebar
  const closeSidebar = (withoutAnimation: boolean) => {
    sidebar.opened = false
    sidebar.withoutAnimation = withoutAnimation
  }

  // Toggle device type
  const toggleDevice = (value: DeviceEnum) => {
    device.value = value
  }

  // Set language
  const setLanguage = (value: string) => {
    language.value = value
    setLocalStorageLanguage(value)
    ;(i18n.global.locale.value as any) = value
  }

  return { device, sidebar, language, toggleSidebar, closeSidebar, toggleDevice, setLanguage }
})

/**
 * @description In SPA apps, can be used before the pinia instance is activated
 * @description In SSR apps, can be used outside of setup
 */
export function useAppStoreOutside() {
  return useAppStore(pinia)
}
