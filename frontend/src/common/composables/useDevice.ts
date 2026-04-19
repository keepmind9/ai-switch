import { DeviceEnum } from "@@/constants/app-key"
import { useAppStore } from "@/pinia/stores/app"

const appStore = useAppStore()

const isMobile = computed(() => appStore.device === DeviceEnum.Mobile)

const isDesktop = computed(() => appStore.device === DeviceEnum.Desktop)

/** Device type Composable */
export function useDevice() {
  return { isMobile, isDesktop }
}
