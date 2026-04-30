<script setup lang="ts">
import { ref, onMounted } from "vue"
import { Monitor, Connection, Key, Right } from "@element-plus/icons-vue"
import { getAdminStatus, listPresets, type Preset } from "@/api/stats"

const status = ref<any>({})
const presets = ref<Preset[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    const [s, pr] = await Promise.all([getAdminStatus(), listPresets()])
    status.value = s.data
    presets.value = pr.data
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="app-container">
    <div class="page-header">
      <div>
        <h3>{{ $t('dashboard.overview') }}</h3>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">{{ $t('dashboard.overviewDesc') }}</p>
      </div>
    </div>

    <!-- Stats Grid -->
    <el-skeleton :loading="loading" animated :rows="3">
      <template #default>
        <el-row :gutter="20" class="mb-8">
          <el-col :xs="24" :sm="8">
            <el-card shadow="hover" class="stat-card border-none!">
              <div class="flex items-center gap-5">
                <div class="stat-icon-circle bg-indigo-50 dark:bg-indigo-900/30 text-indigo-600 dark:text-indigo-400">
                  <el-icon :size="24"><Monitor /></el-icon>
                </div>
                <div class="stat-info">
                  <div class="stat-label uppercase tracking-wider text-[11px] font-bold opacity-70">{{ $t('dashboard.status') }}</div>
                  <div class="stat-value text-lg! text-slate-900 dark:text-slate-100">{{ status.server?.host || '—' }}:{{ status.server?.port || '—' }}</div>
                </div>
              </div>
            </el-card>
          </el-col>
          <el-col :xs="24" :sm="8">
            <el-card shadow="hover" class="stat-card border-none!">
              <div class="flex items-center gap-5">
                <div class="stat-icon-circle bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400">
                  <el-icon :size="24"><Connection /></el-icon>
                </div>
                <div class="stat-info">
                  <div class="stat-label uppercase tracking-wider text-[11px] font-bold opacity-70">{{ $t('dashboard.providers') }}</div>
                  <div class="stat-value text-blue-600! dark:text-blue-400!">{{ status.provider_count || 0 }}</div>
                </div>
              </div>
            </el-card>
          </el-col>
          <el-col :xs="24" :sm="8">
            <el-card shadow="hover" class="stat-card border-none!">
              <div class="flex items-center gap-5">
                <div class="stat-icon-circle bg-emerald-50 dark:bg-emerald-900/30 text-emerald-600 dark:text-emerald-400">
                  <el-icon :size="24"><Key /></el-icon>
                </div>
                <div class="stat-info">
                  <div class="stat-label uppercase tracking-wider text-[11px] font-bold opacity-70">{{ $t('dashboard.activeKeys') }}</div>
                  <div class="stat-value text-emerald-700! dark:text-emerald-400!">{{ status.route_count || 0 }}</div>
                </div>
              </div>
            </el-card>
          </el-col>
        </el-row>

        <!-- Main Content -->
        <el-row :gutter="20">
          <el-col :span="24">
            <el-card shadow="never" class="section-card border-none!">
              <template #header>
                <div class="flex items-center justify-between">
                  <span class="card-header-label">{{ $t('dashboard.supportedPresets') }}</span>
                  <el-button link type="primary">{{ $t('dashboard.viewDocs') }} <el-icon class="ml-1"><Right /></el-icon></el-button>
                </div>
              </template>
              <div class="preset-grid">
                <div
                  v-for="p in presets"
                  :key="p.key"
                  class="preset-item"
                  :style="{ '--p-color': p.icon_color }"
                >
                  <div class="preset-icon" :style="{ backgroundColor: p.icon_color + '15' }">
                    <span class="text-xs font-bold" :style="{ color: p.icon_color }">{{ p.key.slice(0, 2).toUpperCase() }}</span>
                  </div>
                  <div class="preset-name">
                    {{ p.name }}
                    <el-tooltip v-if="p.is_partner" :content="$t('dashboard.partnerProvider')" placement="top">
                      <span class="partner-star">★</span>
                    </el-tooltip>
                  </div>
                </div>
              </div>
            </el-card>
          </el-col>
        </el-row>
      </template>
    </el-skeleton>
  </div>
</template>

<style lang="scss" scoped>
.stat-card {
  height: 100%;
  .stat-icon-circle {
    width: 60px;
    height: 60px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
}

.preset-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 16px;
}

.preset-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  border-radius: var(--v3-border-radius-small);
  background-color: #fbfaf8;
  border: 1px solid var(--el-border-color);
  transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
  cursor: default;

  &:hover {
    border-color: var(--p-color);
    background-color: #ffffff;
    transform: translateY(-3px);
    box-shadow: 0 4px 12px rgba(29, 28, 22, 0.06);
  }

  .preset-icon {
    width: 32px;
    height: 32px;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .preset-name {
    font-size: 14px;
    font-weight: 600;
    color: #334155;
    display: flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}

html.dark {
  .preset-item {
    background-color: #1e293b;
    border-color: #334155;
    .preset-name { color: #f1f5f9; }
    &:hover { background-color: #1e293b; }
  }
}

.partner-star {
  color: #f59e0b;
  font-size: 14px;
}
</style>
