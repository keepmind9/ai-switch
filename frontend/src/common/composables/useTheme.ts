import { setCssVar } from "@@/utils/css"
import { getActiveThemeName, setActiveThemeName } from "@@/utils/local-storage"

const DEFAULT_THEME_NAME = "normal"

type DefaultThemeName = typeof DEFAULT_THEME_NAME

/** Registered theme names, where DefaultThemeName is required */
export type ThemeName = DefaultThemeName | "dark" | "dark-blue"

interface ThemeList {
  title: string
  name: ThemeName
}

/** Theme list */
const themeList: ThemeList[] = [
  {
    title: "Default",
    name: DEFAULT_THEME_NAME
  },
  {
    title: "Dark",
    name: "dark"
  },
  {
    title: "Dark Blue",
    name: "dark-blue"
  }
]

/** Currently applied theme name */
const activeThemeName = ref<ThemeName>(getActiveThemeName() || DEFAULT_THEME_NAME)

/** Set theme */
function setTheme({ clientX, clientY }: MouseEvent, value: ThemeName) {
  const maxRadius = Math.hypot(
    Math.max(clientX, window.innerWidth - clientX),
    Math.max(clientY, window.innerHeight - clientY)
  )
  setCssVar("--v3-theme-x", `${clientX}px`)
  setCssVar("--v3-theme-y", `${clientY}px`)
  setCssVar("--v3-theme-r", `${maxRadius}px`)
  const handler = () => {
    activeThemeName.value = value
  }
  document.startViewTransition ? document.startViewTransition(handler) : handler()
}

/** Add class to the html root element */
function addHtmlClass(value: ThemeName) {
  document.documentElement.classList.add(value)
}

/** Remove other theme classes from the html root element */
function removeHtmlClass(value: ThemeName) {
  const otherThemeNameList = themeList.map(item => item.name).filter(name => name !== value)
  document.documentElement.classList.remove(...otherThemeNameList)
}

/** Initialize */
function initTheme() {
  // Use watchEffect to collect side effects
  watchEffect(() => {
    const value = activeThemeName.value
    removeHtmlClass(value)
    addHtmlClass(value)
    setActiveThemeName(value)
  })
}

/** Theme Composable */
export function useTheme() {
  return { themeList, activeThemeName, initTheme, setTheme }
}
