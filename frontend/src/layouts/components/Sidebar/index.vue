<script lang="ts" setup>
import { useDevice } from "@@/composables/useDevice"
import { useLayoutMode } from "@@/composables/useLayoutMode"
import { useAppStore } from "@/pinia/stores/app"
import { useSettingsStore } from "@/pinia/stores/settings"
import { constantRoutes } from "@/router/index"
import { Logo } from "../index"
import Item from "./Item.vue"

const { isMobile } = useDevice()
const { isLeft, isTop } = useLayoutMode()
const route = useRoute()
const appStore = useAppStore()
const settingsStore = useSettingsStore()

const activeMenu = computed(() => route.meta.activeMenu || route.path)
const noHiddenRoutes = computed(() => constantRoutes.filter(item => !item.meta?.hidden))
const isCollapse = computed(() => !appStore.sidebar.opened)
const isLogo = computed(() => isLeft.value && settingsStore.showLogo)
const sidebarMenuItemHeight = computed(() => !isTop.value ? "var(--v3-sidebar-menu-item-height)" : "var(--v3-navigationbar-height)")
const tipLineWidth = computed(() => !isTop.value ? "2px" : "0px")
</script>

<template>
  <div :class="{ 'has-logo': isLogo }">
    <Logo v-if="isLogo" :collapse="isCollapse" />
    <el-scrollbar wrap-class="scrollbar-wrapper">
      <el-menu
        :default-active="activeMenu"
        :collapse="isCollapse && !isTop"
        :collapse-transition="false"
        :mode="isTop && !isMobile ? 'horizontal' : 'vertical'"
      >
        <Item
          v-for="noHiddenRoute in noHiddenRoutes"
          :key="noHiddenRoute.path"
          :item="noHiddenRoute"
          :base-path="noHiddenRoute.path"
        />
      </el-menu>
    </el-scrollbar>
  </div>
</template>

<style lang="scss" scoped>
.has-logo {
  .el-scrollbar {
    height: calc(100% - var(--v3-header-height));
  }
}

.el-scrollbar {
  height: 100%;
  background-color: var(--v3-sidebar-menu-bg-color);
  :deep(.scrollbar-wrapper) {
    overflow-x: hidden;
  }
  :deep(.el-scrollbar__bar) {
    &.is-horizontal {
      display: none;
    }
  }
}

.el-menu {
  user-select: none;
  border: none;
  width: 100%;
  background-color: transparent;
  padding: 12px;
}

.el-menu--horizontal {
  height: v-bind(sidebarMenuItemHeight);
  padding: 0 12px;
}

:deep(.el-menu-item),
:deep(.el-sub-menu__title),
:deep(.el-sub-menu .el-menu-item),
:deep(.el-menu--horizontal .el-menu-item) {
  height: v-bind(sidebarMenuItemHeight);
  line-height: v-bind(sidebarMenuItemHeight);
  border-radius: var(--v3-border-radius-small);
  margin-bottom: 4px;
  color: var(--v3-sidebar-menu-text-color);
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  
  .el-icon, .svg-icon {
    transition: transform 0.2s;
  }

  &.is-active {
    color: var(--v3-sidebar-menu-active-text-color);
    background-color: var(--v3-sidebar-menu-active-bg-color);
    font-weight: 600;
    
    .el-icon, .svg-icon {
      transform: scale(1.1);
    }
  }
  
  &:hover:not(.is-active) {
    background-color: var(--v3-sidebar-menu-hover-bg-color);
    color: #1e293b;
  }
}

html.dark :deep(.el-menu-item:hover:not(.is-active)) {
  color: #f1f5f9;
}

:deep(.el-sub-menu) {
  &.is-active {
    > .el-sub-menu__title {
      color: var(--v3-sidebar-menu-active-text-color);
      font-weight: 600;
    }
  }
}

.el-menu--collapse {
  padding: 12px 8px;
  :deep(.el-menu-item), :deep(.el-sub-menu__title) {
    justify-content: center;
    padding: 0 !important;
  }
}
</style>
