<script lang="ts" setup>
import { useLayoutMode } from "@@/composables/useLayoutMode"

interface Props {
  collapse?: boolean
}

const { collapse = true } = defineProps<Props>()
const { isTop } = useLayoutMode()
</script>

<template>
  <div class="layout-logo-container" :class="{ 'collapse': collapse, 'layout-mode-top': isTop }">
    <transition name="layout-logo-fade">
      <router-link v-if="collapse" key="collapse" to="/">
        <div class="layout-logo-icon">
          <svg viewBox="0 0 32 32" width="28" height="28">
            <rect width="32" height="32" rx="8" fill="var(--el-color-primary)" />
            <text x="16" y="22" font-family="Inter,Arial,sans-serif" font-size="16" font-weight="700" fill="white" text-anchor="middle">G</text>
          </svg>
        </div>
      </router-link>
      <router-link v-else key="expand" to="/">
        <div class="layout-logo-text">
          <svg viewBox="0 0 32 32" width="24" height="24" class="logo-svg">
            <rect width="32" height="32" rx="8" fill="var(--el-color-primary)" />
            <text x="16" y="22" font-family="Inter,Arial,sans-serif" font-size="16" font-weight="700" fill="white" text-anchor="middle">G</text>
          </svg>
          <span class="logo-text">LLM Gateway</span>
        </div>
      </router-link>
    </transition>
  </div>
</template>

<style lang="scss" scoped>
.layout-logo-container {
  position: relative;
  width: 100%;
  height: var(--v3-header-height);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  .layout-logo-icon {
    display: none;
  }
  .layout-logo-text {
    display: flex;
    align-items: center;
    gap: 10px;
    .logo-text {
      font-size: 16px;
      font-weight: 700;
      color: var(--el-text-color-primary);
      letter-spacing: 0.5px;
    }
  }
}
.layout-mode-top {
  height: var(--v3-navigationbar-height);
  line-height: var(--v3-navigationbar-height);
}
.collapse {
  .layout-logo-icon {
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .layout-logo-text {
    display: none;
  }
}
</style>
