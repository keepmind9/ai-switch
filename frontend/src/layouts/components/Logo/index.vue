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
        <span class="layout-logo-icon">LG</span>
      </router-link>
      <router-link v-else key="expand" to="/">
        <span class="layout-logo-text">LLM Gateway</span>
      </router-link>
    </transition>
  </div>
</template>

<style lang="scss" scoped>
.layout-logo-container {
  position: relative;
  width: 100%;
  height: var(--v3-header-height);
  line-height: var(--v3-header-height);
  text-align: center;
  overflow: hidden;
  .layout-logo-icon {
    display: none;
  }
  .layout-logo-text {
    font-size: 18px;
    font-weight: 700;
    color: #ffffff;
    letter-spacing: 1px;
  }
}
.layout-mode-top {
  height: var(--v3-navigationbar-height);
  line-height: var(--v3-navigationbar-height);
}
.collapse {
  .layout-logo-icon {
    display: inline-block;
    font-size: 16px;
    font-weight: 700;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    width: 32px;
    height: 32px;
    line-height: 32px;
    border-radius: 6px;
    vertical-align: middle;
  }
  .layout-logo-text {
    display: none;
  }
}
</style>
