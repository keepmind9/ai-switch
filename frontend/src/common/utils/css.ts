/** Get the value of a CSS variable on the specified element (defaults to global) */
export function getCssVar(varName: string, element: HTMLElement = document.documentElement) {
  if (!varName?.startsWith("--")) {
    console.error("CSS variable name should start with '--'")
    return ""
  }
  // Returns an empty string when no value is found
  return getComputedStyle(element).getPropertyValue(varName)
}

/** Set the value of a CSS variable on the specified element (defaults to global) */
export function setCssVar(varName: string, value: string, element: HTMLElement = document.documentElement) {
  if (!varName?.startsWith("--")) {
    console.error("CSS variable name should start with '--'")
    return
  }
  element.style.setProperty(varName, value)
}
