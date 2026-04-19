import { LayoutModeEnum } from "@@/constants/app-key"
import { getLayoutsConfig } from "@@/utils/local-storage"

export interface LayoutsConfig {
  layoutMode: LayoutModeEnum
  showSettings: boolean
  showLogo: boolean
  fixedHeader: boolean
  showFooter: boolean
  showThemeSwitch: boolean
  showScreenfull: boolean
}

const DEFAULT_CONFIG: LayoutsConfig = {
  layoutMode: LayoutModeEnum.Left,
  showSettings: false,
  fixedHeader: true,
  showFooter: false,
  showLogo: true,
  showThemeSwitch: true,
  showScreenfull: true
}

export const layoutsConfig: LayoutsConfig = { ...DEFAULT_CONFIG, ...getLayoutsConfig() }
