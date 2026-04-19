<script lang="ts" setup>
import { useTheme } from "@@/composables/useTheme"
import en from "element-plus/es/locale/lang/en"

const { initTheme } = useTheme()
initTheme()
</script>

<template>
  <el-config-provider :locale="en">
    <router-view />
  </el-config-provider>
</template>
