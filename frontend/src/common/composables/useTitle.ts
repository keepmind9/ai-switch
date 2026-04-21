/** App title */
const VITE_APP_TITLE = import.meta.env.VITE_APP_TITLE ?? "AI Switch"

/** Dynamic title */
const dynamicTitle = ref<string>("")

/** Set title */
function setTitle(title?: string) {
  dynamicTitle.value = title ? `${VITE_APP_TITLE} | ${title}` : VITE_APP_TITLE
}

// Watch for title changes
watch(dynamicTitle, (value, oldValue) => {
  if (document && value !== oldValue) {
    document.title = value
  }
})

/** Title Composable */
export function useTitle() {
  return { setTitle }
}
