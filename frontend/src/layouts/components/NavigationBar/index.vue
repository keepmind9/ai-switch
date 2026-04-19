<script lang="ts" setup>
import Screenfull from "@@/components/Screenfull/index.vue"
import ThemeSwitch from "@@/components/ThemeSwitch/index.vue"
import { useDevice } from "@@/composables/useDevice"
import { useLayoutMode } from "@@/composables/useLayoutMode"
import { useAppStore } from "@/pinia/stores/app"
import { useSettingsStore } from "@/pinia/stores/settings"
import { Breadcrumb, Hamburger, Sidebar } from "../index"

const { isMobile } = useDevice()
const { isTop } = useLayoutMode()
const appStore = useAppStore()
const settingsStore = useSettingsStore()
const { showThemeSwitch, showScreenfull } = storeToRefs(settingsStore)

function toggleSidebar() {
  appStore.toggleSidebar(false)
}
</script>

<template>
  <div class="navigation-bar">
    <Hamburger
      v-if="!isTop || isMobile"
      :is-active="appStore.sidebar.opened"
      class="hamburger"
      @toggle-click="toggleSidebar"
    />
    <Breadcrumb v-if="!isTop || isMobile" class="breadcrumb" />
    <Sidebar v-if="isTop && !isMobile" class="sidebar" />
    <div class="right-menu">
      <Screenfull v-if="showScreenfull" class="right-menu-item" />
      <ThemeSwitch v-if="showThemeSwitch" class="right-menu-item" />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.navigation-bar {
  height: var(--v3-navigationbar-height);
  overflow: hidden;
  color: var(--v3-navigationbar-text-color);
  display: flex;
  justify-content: space-between;
  align-items: center;
  background-color: var(--v3-header-bg-color);
  border-bottom: var(--v3-header-border-bottom);
  .hamburger {
    display: flex;
    align-items: center;
    height: 100%;
    padding: 0 16px;
    cursor: pointer;
  }
  .breadcrumb {
    flex: 1;
    @media screen and (max-width: 576px) {
      display: none;
    }
  }
  .sidebar {
    flex: 1;
    min-width: 0px;
    :deep(.el-menu) {
      background-color: transparent;
    }
    :deep(.el-sub-menu) {
      &.is-active {
        .el-sub-menu__title {
          color: var(--el-color-primary);
        }
      }
    }
  }
  .right-menu {
    margin-right: 12px;
    height: 100%;
    display: flex;
    align-items: center;
    &-item {
      margin: 0 8px;
      cursor: pointer;
    }
  }
}
</style>
