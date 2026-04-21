const DEFAULT_THEME_NAME = "normal"

type DefaultThemeName = typeof DEFAULT_THEME_NAME

/** Registered theme names */
export type ThemeName = DefaultThemeName

/** Currently applied theme name */
const activeThemeName = ref<ThemeName>(DEFAULT_THEME_NAME)

/** Initialize */
function initTheme() {
  document.documentElement.classList.add(DEFAULT_THEME_NAME)
}

/** Theme Composable */
export function useTheme() {
  return { activeThemeName, initTheme }
}
