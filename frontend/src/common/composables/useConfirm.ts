import { ref } from 'vue'

export function useConfirm() {
  const confirmState = ref<Record<string, boolean>>({})

  function toggle(key: string) {
    confirmState.value[key] = !confirmState.value[key]
  }

  function reset(key: string) {
    confirmState.value[key] = false
  }

  return { confirmState, toggle, reset }
}
