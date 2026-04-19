<script setup lang="ts">
import { ref, onMounted } from "vue"
import { getAdminStatus, listPresets, type Preset } from "@/api/stats"
import { listProviders, type Provider } from "@/api/providers"

const status = ref<any>({})
const providers = ref<Provider[]>([])
const presets = ref<Preset[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    const [s, p, pr] = await Promise.all([getAdminStatus(), listProviders(), listPresets()])
    status.value = s.data
    providers.value = p.data.data
    presets.value = pr.data.data
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="app-container">
    <el-row :gutter="16" style="margin-bottom: 20px">
      <el-col :span="8">
        <el-card shadow="never">
          <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Server</span></template>
          <div style="font-size: 16px; font-weight: 500">{{ status.server?.host }}:{{ status.server?.port }}</div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="never">
          <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Providers</span></template>
          <div style="font-size: 24px; font-weight: 600; color: var(--el-color-primary)">{{ status.provider_count }}</div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="never">
          <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Routes</span></template>
          <div style="font-size: 24px; font-weight: 600; color: var(--el-color-primary)">{{ status.route_count }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" style="margin-bottom: 20px">
      <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Supported Providers</span></template>
      <el-space wrap>
        <el-tag v-for="p in presets" :key="p.key" :color="p.icon_color" effect="dark" round>{{ p.name }}</el-tag>
      </el-space>
    </el-card>

    <el-card shadow="never" v-if="providers.length">
      <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Configured Providers</span></template>
      <el-table :data="providers" stripe>
        <el-table-column prop="name" label="Name" width="150" />
        <el-table-column prop="key" label="Key" width="120" />
        <el-table-column prop="base_url" label="Base URL" />
        <el-table-column prop="format" label="Format" width="100" />
        <el-table-column prop="model" label="Model" width="150" />
        <el-table-column label="Default" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.is_default" type="success" size="small">Default</el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>
