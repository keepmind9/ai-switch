<script lang="ts" setup>
import Screenfull from "@@/components/Screenfull/index.vue"
import { useDevice } from "@@/composables/useDevice"
import { useLayoutMode } from "@@/composables/useLayoutMode"
import { useAppStore } from "@/pinia/stores/app"
import { useSettingsStore } from "@/pinia/stores/settings"
import { Breadcrumb, Hamburger, Sidebar } from "../index"
import { Operation } from "@element-plus/icons-vue"

const { isMobile } = useDevice()
const { isTop } = useLayoutMode()
const appStore = useAppStore()
const settingsStore = useSettingsStore()
const { showScreenfull } = storeToRefs(settingsStore)

function toggleSidebar() {
  appStore.toggleSidebar(false)
}

function handleLanguageChange(lang: string) {
  appStore.setLanguage(lang)
  ElMessage.success(lang === "zh-cn" ? "语言切换成功" : "Language switched successfully")
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
      <div class="right-menu-container">
        <el-dropdown trigger="click" @command="handleLanguageChange">
          <div class="right-menu-item">
            <el-tooltip :content="$t('navbar.language')" placement="bottom">
              <el-icon :size="18"><Operation /></el-icon>
            </el-tooltip>
          </div>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="en" :disabled="appStore.language === 'en'">English</el-dropdown-item>
              <el-dropdown-item command="zh-cn" :disabled="appStore.language === 'zh-cn'">简体中文</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <Screenfull v-if="showScreenfull" class="right-menu-item" />
      </div>
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
  padding: 0 8px;

  .hamburger {
    display: flex;
    align-items: center;
    height: 100%;
    padding: 0 12px;
    cursor: pointer;
    transition: background-color 0.3s;
    &:hover {
      background-color: var(--v3-sidebar-menu-hover-bg-color);
    }
  }
  .breadcrumb {
    flex: 1;
    margin-left: 8px;
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
  }
  .right-menu {
    height: 100%;
    display: flex;
    align-items: center;
    
    &-container {
      display: flex;
      align-items: center;
      gap: 4px;
      padding-right: 8px;
    }

    &-item {
      padding: 0 10px;
      height: 40px;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #64748b;
      border-radius: var(--v3-border-radius-small);
      transition: all 0.3s;
      cursor: pointer;
      
      &:hover {
        background-color: var(--v3-sidebar-menu-hover-bg-color);
        color: var(--el-color-primary);
      }
    }
  }
}
</style>
