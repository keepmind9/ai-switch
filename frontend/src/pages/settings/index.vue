<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getSettings, updateSettings, restartServer } from '@/api/settings'
import type { Settings } from '@/api/settings'
import { useConfirm } from '@@/composables/useConfirm'

const { t } = useI18n()
const { confirmState, toggle: toggleRestart, reset: resetRestart } = useConfirm()

const loading = ref(true)
const saving = ref(false)
const restarting = ref(false)
const restartUrl = ref('')
const pendingRestart = ref(false)

const form = ref<Settings>({
  host: '127.0.0.1',
  port: 12345,
  log_retention_days: 30,
})

const originalPort = ref(12345)
const originalHost = ref('127.0.0.1')
const originalLogRetention = ref(30)

function buildUrl(host: string, port: number) {
  const h = host === '0.0.0.0' ? 'localhost' : host
  return `http://${h}:${port}/ui`
}

const hasUnsavedChanges = computed(() =>
  form.value.host !== originalHost.value
  || form.value.port !== originalPort.value
  || form.value.log_retention_days !== originalLogRetention.value
)

async function load() {
  loading.value = true
  try {
    const { data } = await getSettings()
    form.value = { ...data }
    originalHost.value = data.host
    originalPort.value = data.port
    originalLogRetention.value = data.log_retention_days
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    const { data } = await updateSettings({
      host: form.value.host,
      port: form.value.port,
      log_retention_days: form.value.log_retention_days,
    })
    const hostOrPortChanged = form.value.host !== originalHost.value || form.value.port !== originalPort.value
    form.value = { ...data }
    originalHost.value = data.host
    originalPort.value = data.port
    originalLogRetention.value = data.log_retention_days
    ElMessage.success(t('settings.successSave'))
    pendingRestart.value = true
    if (hostOrPortChanged) {
      restartUrl.value = buildUrl(form.value.host, form.value.port)
    }
  } catch {
    ElMessage.error(t('settings.failSave'))
  } finally {
    saving.value = false
  }
}

async function handleRestart() {
  restarting.value = true
  const newUrl = buildUrl(form.value.host, form.value.port)
  restartUrl.value = newUrl

  try {
    await restartServer()
    ElMessage.success(t('settings.successRestart'))
  } catch {
    ElMessage.success(t('settings.successRestart'))
  } finally {
    restarting.value = false
    resetRestart('restart')
  }
}

onMounted(load)
</script>

<template>
  <div class="app-container">
    <el-page-header :title="t('settings.title')" :content="t('settings.desc')" />

    <el-card shadow="never" style="margin-top: 20px" v-loading="loading">
      <el-form label-position="top" style="max-width: 500px">
        <el-form-item :label="t('settings.form.host')">
          <el-input v-model="form.host" :placeholder="t('settings.form.hostPlaceholder')" />
        </el-form-item>

        <el-form-item :label="t('settings.form.port')">
          <el-input-number v-model="form.port" :min="1" :max="65535" controls-position="right"
            :placeholder="t('settings.form.portPlaceholder')" style="width: 100%" />
        </el-form-item>

        <el-form-item>
          <template #label>
            {{ t('settings.form.logRetention') }}
            <el-text type="info" size="small" style="margin-left: 8px">
              {{ t('settings.form.logRetentionTip') }}
            </el-text>
          </template>
          <el-input-number v-model="form.log_retention_days" :min="1" :max="365" controls-position="right"
            style="width: 100%" />
        </el-form-item>
      </el-form>

      <el-divider />

      <div v-if="restartUrl" style="margin-bottom: 16px">
        <el-alert :title="t('settings.newUrl')" type="success" :closable="false" show-icon>
          <template #default>
            <a :href="restartUrl" target="_blank" style="font-size: 16px; font-weight: 500">{{ restartUrl }}</a>
          </template>
        </el-alert>
      </div>

      <div style="display: flex; gap: 12px; align-items: center">
        <el-button type="primary" :loading="saving" @click="handleSave">
          {{ t('settings.save') }}
        </el-button>
        <template v-if="pendingRestart && !hasUnsavedChanges">
          <template v-if="confirmState.restart">
            <el-button @click="resetRestart('restart')">
              {{ t('settings.cancel') }}
            </el-button>
            <el-button type="danger" :loading="restarting" @click="handleRestart">
              {{ t('settings.confirm') }}
            </el-button>
          </template>
          <el-button v-else type="warning" @click="toggleRestart('restart')">
            {{ t('settings.restart') }}
          </el-button>
        </template>
      </div>
    </el-card>
  </div>
</template>
