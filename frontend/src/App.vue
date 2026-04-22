<script lang="ts" setup>
import { computed } from "vue"
import { useTheme } from "@@/composables/useTheme"
import { useAppStore } from "@/pinia/stores/app"
import { elementLocales } from "@/locales"

const { initTheme } = useTheme()
const appStore = useAppStore()

initTheme()

const locale = computed(() => {
  return elementLocales[appStore.language] || elementLocales.en
})
</script>

<template>
  <el-config-provider :locale="locale">
    <router-view />
  </el-config-provider>
</template>
