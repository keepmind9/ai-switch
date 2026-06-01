<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getSettings, updateSettings, restartServer, stopServer, listConfigBackups, restoreConfigBackup, cleanConfigBackups } from '@/api/settings'
import type { Settings, ConfigBackup } from '@/api/settings'
import { useConfirm } from '@@/composables/useConfirm'

const { t } = useI18n()
const { confirmState, toggle: toggleRestart, reset: resetRestart } = useConfirm()
const { confirmState: stopConfirmState, toggle: toggleStop, reset: resetStop } = useConfirm()
const { confirmState: restoreConfirmState, toggle: toggleRestore, reset: resetRestore } = useConfirm()

const loading = ref(true)
const saving = ref(false)
const restarting = ref(false)
const stopping = ref(false)
const restartUrl = ref('')
const pendingRestart = ref(false)
const newTagValue = ref('')

const backupsLoading = ref(false)
const backups = ref<ConfigBackup[]>([])
const cleanKeep = ref(10)
const cleaning = ref(false)
const pendingRestoreName = ref('')

const form = ref<Settings>({
  host: '127.0.0.1',
  port: 12345,
  allowed_ips: [],
  log_retention_days: 30,
  proxy_url: '',
})

const originalPort = ref(12345)
const originalHost = ref('127.0.0.1')
const originalLogRetention = ref(30)
const originalAllowedIps = ref<string[]>([])
const originalProxyUrl = ref('')

function buildUrl(host: string, port: number) {
  const h = host === '0.0.0.0' ? 'localhost' : host
  return `http://${h}:${port}/ui`
}

const isLocalhost = computed(() => form.value.host === '127.0.0.1' || form.value.host === 'localhost')

const hasUnsavedChanges = computed(() =>
  form.value.host !== originalHost.value
  || form.value.port !== originalPort.value
  || form.value.log_retention_days !== originalLogRetention.value
  || JSON.stringify(form.value.allowed_ips) !== JSON.stringify(originalAllowedIps.value)
  || form.value.proxy_url !== originalProxyUrl.value
)

function isValidIpOrCidr(s: string): boolean {
  const cidrRegex = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,3}$/
  const ipv6CidrRegex = /^([0-9a-fA-F:]+)\/\d{1,3}$/
  const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/
  const ipv6Regex = /^([0-9a-fA-F:]+)$/

  if (cidrRegex.test(s) || ipv6CidrRegex.test(s)) {
    return true
  }
  if (ipv4Regex.test(s) || ipv6Regex.test(s)) {
    return true
  }
  return false
}

function handleAddTag() {
  const raw = newTagValue.value.trim()
  if (!raw) return
  const parts = raw.split(',').map(s => s.trim()).filter(Boolean)
  const added: string[] = []
  for (const part of parts) {
    if (!isValidIpOrCidr(part)) {
      ElMessage.warning(`${t('settings.form.allowedIpsTagError')}: ${part}`)
      return
    }
    if (!form.value.allowed_ips.includes(part)) {
      added.push(part)
    }
  }
  if (added.length) {
    form.value.allowed_ips = [...form.value.allowed_ips, ...added]
  }
  newTagValue.value = ''
}

function handleRemoveTag(tag: string) {
  form.value.allowed_ips = form.value.allowed_ips.filter(t => t !== tag)
}

async function load() {
  loading.value = true
  try {
    const { data } = await getSettings()
    form.value = { ...data, allowed_ips: data.allowed_ips || [] }
    originalHost.value = data.host
    originalPort.value = data.port
    originalLogRetention.value = data.log_retention_days
    originalAllowedIps.value = data.allowed_ips || []
    originalProxyUrl.value = data.proxy_url
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
      allowed_ips: form.value.allowed_ips,
      proxy_url: form.value.proxy_url,
    })
    const hostOrPortChanged = form.value.host !== originalHost.value || form.value.port !== originalPort.value
    form.value = { ...data, allowed_ips: data.allowed_ips || [] }
    originalHost.value = data.host
    originalPort.value = data.port
    originalLogRetention.value = data.log_retention_days
    originalAllowedIps.value = data.allowed_ips || []
    originalProxyUrl.value = data.proxy_url
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

async function handleStop() {
  stopping.value = true
  try {
    await stopServer()
    ElMessage.success(t('settings.successStop'))
  } catch {
    ElMessage.success(t('settings.successStop'))
  } finally {
    stopping.value = false
    resetStop('stop')
  }
}

async function loadBackups() {
  backupsLoading.value = true
  try {
    const { data } = await listConfigBackups()
    backups.value = data || []
  } catch {
    ElMessage.warning(t('settings.backups.failLoad'))
  } finally {
    backupsLoading.value = false
  }
}

function askRestore(name: string) {
  pendingRestoreName.value = name
  toggleRestore('restore')
}

async function doRestore() {
  const name = pendingRestoreName.value
  resetRestore('restore')
  if (!name) return
  try {
    await restoreConfigBackup(name)
    ElMessage.success(t('settings.backups.successRestore', { name }))
    await loadBackups()
    await load() // re-fetch main settings to reflect restored values
  } catch {
    ElMessage.error(t('settings.backups.failRestore'))
  }
}

async function doClean() {
  cleaning.value = true
  try {
    await cleanConfigBackups(cleanKeep.value)
    ElMessage.success(t('settings.backups.successClean', { keep: cleanKeep.value }))
    await loadBackups()
  } catch {
    ElMessage.error(t('settings.backups.failClean'))
  } finally {
    cleaning.value = false
  }
}

onMounted(() => {
  load()
  loadBackups()
})
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

        <el-form-item v-if="!isLocalhost">
          <template #label>
            {{ t('settings.form.allowedIps') }}
            <el-text type="info" size="small" style="margin-left: 8px">
              {{ t('settings.form.allowedIpsTip') }}
            </el-text>
          </template>
          <div style="width: 100%">
            <div class="flex flex-wrap gap-2" style="margin-bottom: 8px">
              <el-tag v-for="tag in form.allowed_ips" :key="tag" closable @close="handleRemoveTag(tag)">
                {{ tag }}
              </el-tag>
            </div>
            <el-input v-model="newTagValue" :placeholder="t('settings.form.allowedIpsPlaceholder')"
              @keyup.enter="handleAddTag" />
          </div>
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

        <el-form-item>
          <template #label>
            {{ t('settings.form.proxyUrl') }}
            <el-text type="info" size="small" style="margin-left: 8px">
              {{ t('settings.form.proxyUrlTip') }}
            </el-text>
          </template>
          <el-input v-model="form.proxy_url" :placeholder="t('settings.form.proxyUrlPlaceholder')" clearable />
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

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>
        <span>{{ t('settings.stop') }}</span>
      </template>
      <div style="margin-bottom: 16px; color: var(--el-text-color-secondary); font-size: 13px">{{ t('settings.stopDesc') }}</div>
      <div style="display: flex; gap: 12px; align-items: center">
        <template v-if="stopConfirmState.stop">
          <el-button @click="resetStop('stop')">
            {{ t('settings.cancel') }}
          </el-button>
          <el-button type="danger" :loading="stopping" @click="handleStop">
            {{ t('settings.confirmStop') }}
          </el-button>
        </template>
        <el-button v-else type="danger" plain @click="toggleStop('stop')">
          {{ t('settings.stop') }}
        </el-button>
      </div>
    </el-card>

    <el-card shadow="never" style="margin-top: 16px" v-loading="backupsLoading">
      <template #header>
        <div style="display: flex; align-items: center; justify-content: space-between">
          <span>{{ t('settings.backups.title') }}</span>
          <el-button size="small" @click="loadBackups">{{ t('settings.backups.refresh') }}</el-button>
        </div>
      </template>
      <div style="margin-bottom: 12px; color: var(--el-text-color-secondary); font-size: 13px">
        {{ t('settings.backups.desc') }}
      </div>

      <el-empty v-if="!backups.length" :description="t('settings.backups.empty')" :image-size="60" />

      <el-table v-else :data="backups" size="small" style="width: 100%">
        <el-table-column prop="name" :label="t('settings.backups.title')" />
        <el-table-column :label="t('settings.backups.sizeLabel')" width="120">
          <template #default="{ row }">{{ t('settings.backups.size', { n: row.size }) }}</template>
        </el-table-column>
        <el-table-column prop="mod_time" :label="t('settings.backups.modTime')" width="220" />
        <el-table-column :label="t('settings.backups.restore')" width="200" align="right">
          <template #default="{ row }">
            <template v-if="restoreConfirmState.restore && pendingRestoreName === row.name">
              <el-button size="small" @click="resetRestore('restore')">
                {{ t('settings.cancel') }}
              </el-button>
              <el-button size="small" type="danger" @click="doRestore">
                {{ t('settings.confirm') }}
              </el-button>
            </template>
            <el-button v-else size="small" type="primary" plain @click="askRestore(row.name)">
              {{ t('settings.backups.restore') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-divider v-if="backups.length" />

      <div v-if="backups.length" style="display: flex; gap: 12px; align-items: center">
        <el-text size="small">{{ t('settings.backups.cleanTitle') }}</el-text>
        <el-input-number v-model="cleanKeep" :min="0" :max="100" controls-position="right" size="small" />
        <el-button size="small" :loading="cleaning" :disabled="!cleanKeep && cleanKeep !== 0" @click="doClean">
          {{ t('settings.backups.clean') }}
        </el-button>
      </div>
    </el-card>
  </div>
</template>
