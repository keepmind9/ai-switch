<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getAdminStatus, type Preset, listPresets } from '../api/stats'
import { listProviders, type Provider } from '../api/providers'

const status = ref<any>({})
const providers = ref<Provider[]>([])
const presets = ref<Preset[]>([])
const loading = ref(true)

const presetMap = computed(() => {
  const m: Record<string, Preset> = {}
  for (const p of presets.value) m[p.key] = p
  return m
})

import { computed } from 'vue'

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
  <div class="page-header">
    <h2>Dashboard</h2>
  </div>

  <el-skeleton :loading="loading" animated :rows="5">
    <el-row :gutter="16" style="margin-bottom: 20px">
      <el-col :span="8">
        <div class="card">
          <div style="color: var(--text-secondary); font-size: 13px; margin-bottom: 8px">Server</div>
          <div style="font-size: 16px; font-weight: 500">{{ status.server?.host }}:{{ status.server?.port }}</div>
        </div>
      </el-col>
      <el-col :span="8">
        <div class="card">
          <div style="color: var(--text-secondary); font-size: 13px; margin-bottom: 8px">Providers</div>
          <div style="font-size: 24px; font-weight: 600; color: var(--accent)">{{ status.provider_count }}</div>
        </div>
      </el-col>
      <el-col :span="8">
        <div class="card">
          <div style="color: var(--text-secondary); font-size: 13px; margin-bottom: 8px">Routes</div>
          <div style="font-size: 24px; font-weight: 600; color: var(--accent)">{{ status.route_count }}</div>
        </div>
      </el-col>
    </el-row>

    <div class="card" v-if="providers.length">
      <h3 style="margin-bottom: 16px; font-size: 16px">Providers</h3>
      <el-table :data="providers" stripe style="width: 100%">
        <el-table-column prop="name" label="Name" width="150" />
        <el-table-column prop="key" label="Key" width="120" />
        <el-table-column prop="base_url" label="Base URL" />
        <el-table-column prop="format" label="Format" width="100" />
        <el-table-column prop="model" label="Model" width="150" />
        <el-table-column label="Sponsor" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.sponsor" type="success" size="small">Yes</el-tag>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </el-skeleton>
</template>
