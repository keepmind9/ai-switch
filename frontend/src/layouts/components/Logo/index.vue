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
          <svg viewBox="0 0 32 32" width="30" height="30">
            <rect width="32" height="32" rx="10" fill="var(--el-color-primary)" />
            <text x="16" y="22" font-family="Inter,Arial,sans-serif" font-size="18" font-weight="800" fill="white" text-anchor="middle">G</text>
          </svg>
        </div>
      </router-link>
      <router-link v-else key="expand" to="/">
        <div class="layout-logo-text">
          <svg viewBox="0 0 32 32" width="28" height="28" class="logo-svg">
            <rect width="32" height="32" rx="9" fill="var(--el-color-primary)" />
            <text x="16" y="22" font-family="Inter,Arial,sans-serif" font-size="18" font-weight="800" fill="white" text-anchor="middle">G</text>
          </svg>
          <span class="logo-text">AI Switch</span>
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
  padding-left: 20px;
  overflow: hidden;
  
  .layout-logo-text {
    display: flex;
    align-items: center;
    gap: 12px;
    .logo-text {
      font-size: 17px;
      font-weight: 800;
      color: var(--v3-title-text-color);
      letter-spacing: -0.025em;
    }
  }
}

.layout-mode-top {
  height: var(--v3-navigationbar-height);
  line-height: var(--v3-navigationbar-height);
}

.collapse {
  padding-left: 0;
  justify-content: center;
  .layout-logo-text {
    display: none;
  }
}
</style>
