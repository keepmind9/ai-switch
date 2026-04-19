<script setup lang="ts">
import { RouterView } from 'vue-router'
import { Monitor, Connection, Key, DataAnalysis } from '@element-plus/icons-vue'
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-logo">LLM Gateway</div>
    <ul class="sidebar-nav">
      <li>
        <router-link to="/">
          <el-icon><Monitor /></el-icon>
          Dashboard
        </router-link>
      </li>
      <li>
        <router-link to="/providers">
          <el-icon><Connection /></el-icon>
          Providers
        </router-link>
      </li>
      <li>
        <router-link to="/routes">
          <el-icon><Key /></el-icon>
          Routes
        </router-link>
      </li>
      <li>
        <router-link to="/stats">
          <el-icon><DataAnalysis /></el-icon>
          Usage Stats
        </router-link>
      </li>
    </ul>
  </aside>
  <main class="main-content">
    <RouterView />
  </main>
</template>
