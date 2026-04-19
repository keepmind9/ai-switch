<script lang="ts" setup>
import { useSettingsStore } from "@/pinia/stores/settings"
import { Footer } from "../index"

const settingsStore = useSettingsStore()
</script>

<template>
  <section class="app-main">
    <div class="app-scrollbar">
      <router-view v-slot="{ Component, route }">
        <transition name="el-fade-in" mode="out-in">
          <component :is="Component" :key="route.path" class="app-container-grow" />
        </transition>
      </router-view>
      <Footer v-if="settingsStore.showFooter" />
    </div>
    <el-backtop target=".app-scrollbar" />
  </section>
</template>

<style lang="scss" scoped>
@import "@@/assets/styles/mixins.scss";

.app-main {
  width: 100%;
  display: flex;
}

.app-scrollbar {
  flex-grow: 1;
  overflow: auto;
  @extend %scrollbar;
  display: flex;
  flex-direction: column;
  .app-container-grow {
    flex-grow: 1;
  }
}
</style>
