<script setup lang="ts">
import { ref, onMounted } from "vue"
import { Monitor, Connection, Key } from "@element-plus/icons-vue"
import { getAdminStatus, listPresets, type Preset } from "@/api/stats"

const status = ref<any>({})
const presets = ref<Preset[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    const [s, pr] = await Promise.all([getAdminStatus(), listPresets()])
    status.value = s.data
    presets.value = pr.data.data
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="app-container">
    <!-- Stats Cards -->
    <el-row :gutter="16" class="stat-row">
      <el-col :span="8">
        <el-card shadow="never" class="stat-card">
          <div class="stat-icon">
            <el-icon :size="24"><Monitor /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Server</div>
            <div class="stat-value">{{ status.server?.host }}:{{ status.server?.port }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="never" class="stat-card">
          <div class="stat-icon stat-icon--primary">
            <el-icon :size="24"><Connection /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Providers</div>
            <div class="stat-value stat-value--primary">{{ status.provider_count || 0 }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="never" class="stat-card">
          <div class="stat-icon stat-icon--success">
            <el-icon :size="24"><Key /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Routes</div>
            <div class="stat-value stat-value--success">{{ status.route_count || 0 }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- Supported Providers -->
    <el-card shadow="never" class="section-card">
      <template #header>
        <div class="card-header-label">Supported Providers</div>
      </template>
      <el-space wrap :size="8">
        <el-tag
          v-for="p in presets"
          :key="p.key"
          :color="p.icon_color"
          effect="dark"
          round
          class="provider-tag"
        >
          <span v-if="p.is_partner" class="partner-star">&#9733;</span>
          {{ p.name }}
        </el-tag>
      </el-space>
    </el-card>
  </div>
</template>

<style lang="scss" scoped>
.stat-row {
  margin-bottom: 16px;
}

.stat-card {
  :deep(.el-card__body) {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 20px;
  }

  .stat-icon {
    width: 48px;
    height: 48px;
    border-radius: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    background-color: var(--el-fill-color-light);
    color: var(--el-text-color-secondary);
    flex-shrink: 0;

    &--primary {
      background-color: var(--el-color-primary-light-9);
      color: var(--el-color-primary);
    }
    &--success {
      background-color: var(--el-color-success-light-9);
      color: var(--el-color-success);
    }
  }

  .stat-info {
    .stat-label {
      font-size: 13px;
      color: var(--el-text-color-secondary);
      margin-bottom: 4px;
    }
    .stat-value {
      font-size: 20px;
      font-weight: 700;
      color: var(--el-text-color-primary);
      line-height: 1.3;
      &--primary { color: var(--el-color-primary); }
      &--success { color: var(--el-color-success); }
    }
  }
}

.section-card {
  margin-bottom: 16px;
}

.provider-tag {
  transition: transform 0.15s;
  &:hover { transform: translateY(-1px); }
}

.partner-star {
  color: #f59e0b;
  margin-right: 2px;
}
</style>
