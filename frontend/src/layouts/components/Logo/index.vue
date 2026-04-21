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
    <router-link to="/">
      <div class="logo-wrapper">
        <div class="logo-icon">
          <svg viewBox="0 0 32 32" width="30" height="30" fill="none" xmlns="http://www.w3.org/2000/svg">
            <rect width="32" height="32" rx="10" fill="var(--el-color-primary)" />
            <path d="M12 10C12 8.89543 12.8954 8 14 8H18C19.1046 8 20 8.89543 20 10V14C20 15.1046 19.1046 16 18 16H14C12.8954 16 12 15.1046 12 14V10Z" fill="white" />
            <path opacity="0.5" d="M12 18C12 16.8954 12.8954 16 14 16H18C19.1046 16 20 16.8954 20 18V22C20 23.1046 19.1046 24 18 24H14C12.8954 24 12 23.1046 12 22V18Z" fill="white" />
            <rect opacity="0.3" x="22" y="8" width="2" height="16" rx="1" fill="white" />
            <rect opacity="0.3" x="8" y="8" width="2" height="16" rx="1" fill="white" />
          </svg>
        </div>
        <span v-if="!collapse" class="logo-text">AI Switch</span>
      </div>
    </router-link>
  </div>
</template>

<style lang="scss" scoped>
.layout-logo-container {
  position: relative;
  width: 100%;
  height: var(--v3-header-height);
  display: flex;
  align-items: center;
  padding: 0 16px;
  overflow: hidden;
  transition: all 0.3s ease;
  
  .logo-wrapper {
    display: flex;
    align-items: center;
    gap: 12px;
    
    .logo-icon {
      display: flex;
      align-items: center;
      justify-content: center;
      transition: transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
      
      &:hover {
        transform: rotate(10deg) scale(1.05);
      }
    }

    .logo-text {
      font-size: 18px;
      font-weight: 800;
      color: var(--v3-title-text-color);
      letter-spacing: -0.04em;
      white-space: nowrap;
      background: linear-gradient(135deg, var(--v3-title-text-color) 0%, #66635c 100%);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
    }
  }
}

.collapse {
  padding: 0;
  justify-content: center;
  .logo-text {
    opacity: 0;
    width: 0;
  }
}

.layout-mode-top {
  height: var(--v3-navigationbar-height);
}
</style>
