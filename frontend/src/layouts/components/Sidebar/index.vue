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
%tip-line {
  &::before {
    content: "";
    position: absolute;
    top: 50%;
    left: 0;
    transform: translateY(-50%);
    width: v-bind(tipLineWidth);
    height: 60%;
    border-radius: 0 3px 3px 0;
    background-color: var(--v3-sidebar-menu-tip-line-bg-color);
  }
}

.has-logo {
  .el-scrollbar {
    height: calc(100% - var(--v3-header-height));
  }
}

.el-scrollbar {
  height: 100%;
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
  padding: 8px;
}

.el-menu--horizontal {
  height: v-bind(sidebarMenuItemHeight);
}

:deep(.el-menu-item),
:deep(.el-sub-menu__title),
:deep(.el-sub-menu .el-menu-item),
:deep(.el-menu--horizontal .el-menu-item) {
  height: v-bind(sidebarMenuItemHeight);
  line-height: v-bind(sidebarMenuItemHeight);
  border-radius: 8px;
  margin-bottom: 2px;
  transition: background-color 0.2s, color 0.2s;
  &.is-active {
    color: var(--el-color-primary);
    background-color: var(--el-color-primary-light-9);
    font-weight: 500;
  }
  &:hover {
    background-color: var(--el-fill-color-light);
  }
}

:deep(.el-sub-menu) {
  &.is-active {
    > .el-sub-menu__title {
      color: var(--el-color-primary);
    }
  }
}

:deep(.el-menu-item.is-active) {
  @extend %tip-line;
}

.el-menu--collapse {
  :deep(.el-sub-menu.is-active) {
    .el-sub-menu__title {
      @extend %tip-line;
      background-color: var(--el-color-primary-light-9);
    }
  }
}
</style>
