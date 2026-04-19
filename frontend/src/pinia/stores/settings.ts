import type { Ref } from "vue"
import type { LayoutsConfig } from "@/layouts/config"
import { setLayoutsConfig } from "@@/utils/local-storage"
import { layoutsConfig } from "@/layouts/config"
import { pinia } from "@/pinia"

type SettingsStore = {
  // Use mapped type to iterate over LayoutsConfig object keys
  [Key in keyof LayoutsConfig]: Ref<LayoutsConfig[Key]>
}

type SettingsStoreKey = keyof SettingsStore

export const useSettingsStore = defineStore("settings", () => {
  // State object
  const state = {} as SettingsStore

  // Iterate over LayoutsConfig object key-value pairs
  for (const [key, value] of Object.entries(layoutsConfig)) {
    // Use type assertion to specify the key type, wrap value in ref to create a reactive variable
    const refValue = ref(value)
    // @ts-expect-error ignore
    state[key as SettingsStoreKey] = refValue
    // Watch each reactive variable
    watch(refValue, () => {
      // Cache
      const settings = getCacheData()
      setLayoutsConfig(settings)
    })
  }

  // Get data to cache: convert state object to settings object
  const getCacheData = () => {
    const settings = {} as LayoutsConfig
    for (const [key, value] of Object.entries(state)) {
      // @ts-expect-error ignore
      settings[key as SettingsStoreKey] = value.value
    }
    return settings
  }

  return state
})

/**
 * @description In SPA apps, can be used before the pinia instance is activated
 * @description In SSR apps, can be used outside of setup
 */
export function useSettingsStoreOutside() {
  return useSettingsStore(pinia)
}
