<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from "vue"
import { ElMessage } from "element-plus"
import { Plus, View, Edit, Hide, CopyDocument, Search, Refresh, Link, Delete, QuestionFilled, Check, Promotion, DocumentCopy, Odometer } from "@element-plus/icons-vue"
import { listProviders, createProvider, updateProvider, deleteProvider, revealAPIKey, fetchModels, getUsage, type Provider, type ModelInfo, type UsageInfo } from "@/api/providers"
import { listPresets, type Preset } from "@/api/stats"
import { useConfirm } from "@@/composables/useConfirm"
import { useI18n } from "vue-i18n"

const { t } = useI18n()
const providers = ref<Provider[]>([])
const presets = ref<Preset[]>([])
const loading = ref(true)
const fetchingModels = ref(false)
const fetchedModels = ref<ModelInfo[]>([])
const showDrawer = ref(false)
const isEdit = ref(false)
const selectedPreset = ref("")
const form = ref<any>({})
const revealedKeys = ref<Record<string, string>>({})
const searchQuery = ref("")
const { confirmState, toggle: toggleDelete, reset: resetDelete } = useConfirm()

const defaultForm = { key: "", name: "", base_url: "", path: "", api_key: "", fallback_keys: [] as string[], format: "chat", logo_url: "", default_model: "", models: [] as string[], enable_proxy: false, think_tag: "", custom_headers: {} as Record<string, string> }
const modelInput = ref("")

async function load() {
  loading.value = true
  try {
    const [p, pr] = await Promise.all([listProviders(), listPresets()])
    providers.value = p.data
    presets.value = pr.data
    providers.value.forEach(p => resetDelete(p.key))
  } finally {
    loading.value = false
  }
}

const filteredProviders = computed(() => {
  if (!searchQuery.value) return providers.value
  const q = searchQuery.value.toLowerCase()
  return providers.value.filter(p => p.name.toLowerCase().includes(q) || p.key.toLowerCase().includes(q) || p.base_url.toLowerCase().includes(q))
})

function applyPreset(key: string) {
  if (selectedPreset.value === key) { selectedPreset.value = ""; return }
  const p = presets.value.find(x => x.key === key)
  if (p) { 
    form.value.base_url = p.base_url
    form.value.format = p.format
    form.value.name = p.name
    form.value.key = p.key 
  }
  selectedPreset.value = key
}

function openCreate() {
  isEdit.value = false; form.value = { ...defaultForm, custom_headers: {} }; selectedPreset.value = ""; fetchedModels.value = []; showDrawer.value = true
}

function openEdit(row: Provider) {
  isEdit.value = true
  form.value = {
    key: row.key,
    name: row.name,
    base_url: row.base_url,
    path: row.path,
    api_key: "",
    fallback_keys: [...(row.fallback_keys || [])],
    format: row.format,
    logo_url: row.logo_url,
    models: [...(row.models || [])],
    enable_proxy: row.enable_proxy || false,
    think_tag: row.think_tag || "",
    custom_headers: { ...(row.custom_headers || {}) }
  }
  selectedPreset.value = ""; fetchedModels.value = []; showDrawer.value = true
}

function openDuplicate(row: Provider) {
  isEdit.value = false
  form.value = {
    key: "",
    name: row.name,
    base_url: row.base_url,
    path: row.path,
    api_key: "",
    fallback_keys: [...(row.fallback_keys || [])],
    format: row.format,
    logo_url: row.logo_url,
    default_model: "",
    models: [...(row.models || [])],
    enable_proxy: row.enable_proxy || false,
    think_tag: row.think_tag || "",
    custom_headers: { ...(row.custom_headers || {}) }
  }
  selectedPreset.value = ""; fetchedModels.value = []; showDrawer.value = true
}

async function handleFetchModels() {
  if (!form.value.base_url || !form.value.format) return
  fetchingModels.value = true
  try {
    const payload: any = {
      base_url: form.value.base_url,
      api_key: form.value.api_key || "",
      format: form.value.format,
      custom_headers: { ...form.value.custom_headers }
    }
    if (isEdit.value && form.value.key) {
      payload.key = form.value.key
    }
    const res = await fetchModels(payload)
    fetchedModels.value = res.data
    ElMessage.success(t("providers.drawer.form.fetchSuccess"))
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || t("providers.drawer.form.fetchFail"))
  } finally {
    fetchingModels.value = false
  }
}

function addModel() {
  const m = modelInput.value.trim()
  if (m && !form.value.models.includes(m)) { form.value.models.push(m); modelInput.value = "" }
}
function removeModel(idx: number) { form.value.models.splice(idx, 1) }

const fallbackKeyInput = ref("")
function addFallbackKey() {
  const k = fallbackKeyInput.value.trim()
  if (k && !form.value.fallback_keys.includes(k)) { form.value.fallback_keys.push(k); fallbackKeyInput.value = "" }
}
function removeFallbackKey(idx: number | string) { form.value.fallback_keys.splice(Number(idx), 1) }

const headerKeyInput = ref("")
const headerValueInput = ref("")
function addCustomHeader() {
  const k = headerKeyInput.value.trim()
  if (k && !(k in form.value.custom_headers)) {
    form.value.custom_headers[k] = headerValueInput.value.trim()
    headerKeyInput.value = ""; headerValueInput.value = ""
  }
}
function removeCustomHeader(k: string) { delete form.value.custom_headers[k] }

async function handleDelete(key: string) {
  await deleteProvider(key)
  resetDelete(key)
  ElMessage.success(t("providers.actions.successDelete"))
  load()
}

async function handleSubmit() {
  try {
    if (isEdit.value) {
      const { key, ...data } = form.value
      await updateProvider(key, data)
      ElMessage.success(t("providers.actions.successUpdate"))
    } else {
      const res = await createProvider(form.value)
      ElMessage.success(t("providers.actions.successAdd"))
      if (res.data?.auto_route_created) {
        ElMessage.success(t("providers.actions.autoRouteCreated", { key: form.value.key }))
      }
      if (res.data?.warnings?.length) {
        ElMessage.warning(t("providers.actions.routeWarnings", { warnings: res.data.warnings.join("; ") }))
      }
    }
    showDrawer.value = false
    load()
  } catch (e) {
    ElMessage.error(t("providers.actions.failSave"))
  }
}

function handleToggleReveal(row: Provider) {
  if (revealedKeys.value[row.key]) { delete revealedKeys.value[row.key] } else { revealKey(row) }
}

async function revealKey(row: Provider) {
  const resp = await revealAPIKey(row.key); revealedKeys.value[row.key] = resp.data.api_key
}

async function handleCopyKey(row: Provider) {
  const key = revealedKeys.value[row.key] || row.api_key
  try {
    await navigator.clipboard.writeText(key)
    ElMessage.success(t("providers.actions.copySuccess"))
  } catch {
    ElMessage.info(key)
  }
}

// ---- Coding-plan usage query (GLM only for now) ----
const USAGE_HOSTS = ["open.bigmodel.cn", "api.z.ai"]
function usageSupported(baseUrl: string) {
  try { return USAGE_HOSTS.includes(new URL(baseUrl).host) } catch { return false }
}

const usageDialog = ref(false)
const usageLoading = ref(false)
const usageInfo = ref<UsageInfo | null>(null)
const usageProviderName = ref("")
const now = ref(Date.now())
let usageTimer: ReturnType<typeof setInterval> | undefined

const usageCountdown = computed(() => {
  if (!usageInfo.value?.resets_at_ms) return ""
  const diff = usageInfo.value.resets_at_ms - now.value
  if (diff <= 0) return "0s"
  const h = Math.floor(diff / 3600000)
  const m = Math.floor((diff % 3600000) / 60000)
  const s = Math.floor((diff % 60000) / 1000)
  return h > 0 ? `${h}h ${m}m` : m > 0 ? `${m}m ${s}s` : `${s}s`
})

const usageStatus = computed(() => {
  const u = usageInfo.value?.utilization ?? 0
  if (u >= 90) return "exception"
  if (u >= 70) return "warning"
  return "success"
})

function formatResetTime(ms: number) {
  return new Date(ms).toLocaleString(undefined, { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
}

// Request generation guard: stale responses (dialog closed, or another
// provider opened meanwhile) are dropped so they can neither leak a countdown
// timer nor overwrite the newer provider's data.
let usageReqId = 0

async function openUsage(row: Provider) {
  const reqId = ++usageReqId
  usageDialog.value = true
  usageLoading.value = true
  usageInfo.value = null
  usageProviderName.value = row.name
  closeUsage()
  try {
    const res = await getUsage(row.key)
    if (reqId !== usageReqId || !usageDialog.value) return
    usageInfo.value = res.data
    if (res.data.window_active) {
      now.value = Date.now()
      usageTimer = setInterval(() => { now.value = Date.now() }, 1000)
    }
  } catch (e: any) {
    if (reqId !== usageReqId) return
    ElMessage.error(e.response?.data?.msg || t("providers.usage.failLoad"))
    usageDialog.value = false
  } finally {
    if (reqId === usageReqId) usageLoading.value = false
  }
}

function closeUsage() {
  if (usageTimer) { clearInterval(usageTimer); usageTimer = undefined }
}

onMounted(load)
onUnmounted(closeUsage)
</script>

<template>
  <div class="app-container">
    <div class="page-header">
      <div>
        <h3>{{ $t('providers.title') }}</h3>
        <p class="text-sm text-slate-500 mt-1">{{ $t('providers.desc') }}</p>
      </div>
      <div class="flex gap-3">
        <el-input v-model="searchQuery" :placeholder="$t('providers.search')" :prefix-icon="Search" clearable style="width: 240px" />
        <el-button type="primary" :icon="Plus" @click="openCreate">{{ $t('providers.add') }}</el-button>
        <el-button :icon="Refresh" circle @click="load" />
      </div>
    </div>

    <el-card shadow="never" class="border-none!">
      <el-table :data="filteredProviders" v-loading="loading" stripe size="large" class="provider-table">
        <el-table-column prop="name" :label="$t('providers.table.provider')" min-width="200">
          <template #default="{ row }">
            <div class="flex items-center gap-3">
              <div class="w-8 h-8 rounded bg-slate-100 flex items-center justify-center overflow-hidden shrink-0">
                <img v-if="row.logo_url" :src="row.logo_url" class="max-w-full max-h-full object-contain" />
                <span v-else class="text-xs font-bold text-slate-400">{{ row.key.slice(0,2).toUpperCase() }}</span>
              </div>
              <div>
                <div class="flex items-center gap-1.5">
                  <span class="font-bold text-slate-700">{{ row.name }}</span>
                  <el-tag v-if="presets.find(p => p.key === row.key)?.is_partner" size="small" type="warning" effect="plain" class="rounded! text-[10px]! leading-none! px-1! py-0.5!">{{ $t('providers.table.sponsor') }}</el-tag>
                  <el-tooltip v-if="row.enable_proxy" :content="$t('providers.table.proxyEnabled')" placement="top">
                    <el-icon class="text-slate-400 text-sm"><Promotion /></el-icon>
                  </el-tooltip>
                </div>
                <div class="text-xs text-slate-400 mono! mt-0.5">{{ row.key }}</div>
              </div>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="base_url" :label="$t('providers.table.endpoint')" min-width="220">
          <template #default="{ row }">
            <div class="flex items-center gap-1 group">
              <span class="text-sm text-slate-500 truncate max-w-200px">{{ row.base_url }}</span>
              <el-link :href="row.base_url" target="_blank" :icon="Link" class="opacity-0 group-hover:opacity-100 transition-opacity" />
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="format" :label="$t('providers.table.format')" width="120">
          <template #default="{ row }">
            <el-tag :type="row.format === 'chat' ? 'primary' : 'warning'" size="small" effect="light">
              {{ row.format }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="$t('providers.table.models')" min-width="240">
          <template #default="{ row }">
            <div class="flex flex-wrap gap-1 max-w-300px">
              <el-tag v-for="m in (row.models || []).slice(0, 5)" :key="m" size="small" type="info" effect="plain" class="rounded-md! border-slate-200! text-slate-500!">
                {{ m }}
              </el-tag>
              <el-tooltip v-if="(row.models || []).length > 5" placement="top">
                <template #content>
                  <div class="flex flex-col gap-1 p-1">
                    <div v-for="m in row.models.slice(5)" :key="m">{{ m }}</div>
                  </div>
                </template>
                <el-tag size="small" type="info" effect="plain" class="rounded-md! border-slate-200! text-slate-400! cursor-help">
                  +{{ row.models.length - 5 }}
                </el-tag>
              </el-tooltip>
              <span v-if="!(row.models || []).length" class="text-xs text-slate-300 italic">{{ $t('providers.table.noModels') }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column :label="$t('providers.table.apiKey')" width="180">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="mono text-xs px-2 py-1 rounded truncate max-w-120px" :style="{ backgroundColor: 'var(--v3-key-bg)', color: revealedKeys[row.key] ? 'var(--v3-key-text-color)' : 'var(--el-text-color-placeholder)' }">
                {{ revealedKeys[row.key] || '••••••••••••••••' }}
              </span>
              <el-button link size="small" @click="handleToggleReveal(row)"><el-icon><component :is="revealedKeys[row.key] ? Hide : View" /></el-icon></el-button>
              <el-button v-if="revealedKeys[row.key]" link size="small" @click="handleCopyKey(row)"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
          </template>
        </el-table-column>

        <el-table-column :label="$t('providers.table.actions')" width="200" fixed="right" align="right">
          <template #default="{ row }">
            <div class="flex justify-end gap-1">
              <el-tooltip v-if="usageSupported(row.base_url)" :content="$t('providers.actions.usage')" placement="top">
                <el-button link type="primary" :icon="Odometer" @click="openUsage(row)" />
              </el-tooltip>
              <el-tooltip :content="$t('providers.actions.edit')" placement="top">
                <el-button link type="primary" :icon="Edit" @click="openEdit(row)" />
              </el-tooltip>
              <el-tooltip :content="$t('providers.actions.duplicate')" placement="top">
                <el-button link type="primary" :icon="DocumentCopy" @click="openDuplicate(row)" />
              </el-tooltip>
              <div class="flex items-center gap-1">
                <template v-if="confirmState[row.key]">
                  <el-button link type="danger" size="small" class="font-medium" @click="handleDelete(row.key)">{{ $t('providers.actions.confirm') }}</el-button>
                  <el-button link type="info" size="small" class="font-medium" @click="resetDelete(row.key)">{{ $t('providers.actions.cancel') }}</el-button>
                </template>
                <el-button v-else link type="primary" :icon="Delete" @click="toggleDelete(row.key)" />
              </div>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="usageDialog"
      :title="$t('providers.usage.title', { name: usageProviderName })"
      width="420px"
      @closed="closeUsage"
    >
      <div v-loading="usageLoading" class="min-h-120px">
        <template v-if="usageInfo">
          <div class="text-center py-2">
            <div class="text-3xl font-bold text-slate-700">{{ usageInfo.utilization.toFixed(1) }}%</div>
            <div class="text-xs text-slate-400 mt-1">{{ $t('providers.usage.used') }}</div>
            <el-progress
              :percentage="Math.min(100, usageInfo.utilization)"
              :status="usageStatus"
              :stroke-width="14"
              class="mt-4"
            />
          </div>
          <el-divider />
          <template v-if="usageInfo.window_active">
            <div class="flex justify-between text-sm">
              <span class="text-slate-500">{{ $t('providers.usage.resetsAt') }}</span>
              <span class="font-medium">{{ formatResetTime(usageInfo.resets_at_ms) }}</span>
            </div>
            <div class="flex justify-between text-sm mt-2">
              <span class="text-slate-500">{{ $t('providers.usage.countdown') }}</span>
              <span class="font-medium">{{ usageCountdown }}</span>
            </div>
          </template>
          <div v-else class="text-sm text-slate-400 text-center py-2">{{ $t('providers.usage.waiting') }}</div>
        </template>
      </div>
    </el-dialog>

    <el-drawer
      v-model="showDrawer"
      :title="isEdit ? $t('providers.drawer.edit') : $t('providers.drawer.add')"
      size="640px"
      destroy-on-close
    >
      <div class="px-4">
        <div v-if="!isEdit" class="mb-8">
          <div class="text-xs font-bold text-slate-400 uppercase tracking-wider mb-3">{{ $t('providers.drawer.presets') }}</div>
          <div class="grid grid-cols-3 gap-2">
            <div
              v-for="p in presets"
              :key="p.key"
              class="preset-card"
              :class="{ 'active': selectedPreset === p.key }"
              @click="applyPreset(p.key)"
            >
              <div class="w-6 h-6 rounded shrink-0 flex items-center justify-center bg-slate-100 mr-2">
                <span class="text-[10px] font-bold" :style="{ color: p.icon_color }">{{ p.key.slice(0,2).toUpperCase() }}</span>
              </div>
              <span class="text-sm font-medium truncate">{{ p.name }}</span>
            </div>
          </div>
        </div>

        <el-form :model="form" label-position="top" class="provider-form">
          <div class="section-title">{{ $t('providers.drawer.section.basic') }}</div>
          <el-form-item :label="$t('providers.drawer.form.id')" required>
            <el-input v-model="form.key" :disabled="isEdit" placeholder="e.g. openai" />
          </el-form-item>
          <el-form-item :label="$t('providers.drawer.form.name')" required>
            <el-input v-model="form.name" placeholder="e.g. OpenAI (Primary)" />
          </el-form-item>
          <el-form-item :label="$t('providers.drawer.form.baseUrl')" required>
            <el-input v-model="form.base_url" placeholder="https://api.openai.com/v1" />
          </el-form-item>
          <el-form-item>
            <template #label>
              <div class="flex items-center gap-1">
                <span>{{ $t('providers.drawer.form.path') }}</span>
                <span class="text-[10px] text-slate-400 font-normal">(Optional)</span>
                <el-tooltip :content="$t('providers.drawer.form.pathTip')" placement="top">
                  <el-icon :size="12" class="text-slate-400 cursor-help"><QuestionFilled /></el-icon>
                </el-tooltip>
              </div>
            </template>
            <el-input v-model="form.path" placeholder="/v1/chat/completions" />
          </el-form-item>

          <el-divider />

          <div class="section-title">{{ $t('providers.drawer.section.auth') }}</div>
          <el-form-item :label="$t('providers.drawer.form.apiKey')">
            <el-input v-model="form.api_key" type="password" show-password :placeholder="isEdit ? $t('providers.drawer.form.apiKeyEditTip') : $t('providers.drawer.form.apiKeyPlaceholder')" />
          </el-form-item>
          <el-form-item>
            <template #label>
              <div class="flex items-center gap-1">
                <span>{{ $t('providers.drawer.form.fallbackKeys') }}</span>
                <span class="text-[10px] text-slate-400 font-normal">(Optional)</span>
                <el-tooltip :content="$t('providers.drawer.form.fallbackKeysTip')" placement="top">
                  <el-icon :size="12" class="text-slate-400 cursor-help"><QuestionFilled /></el-icon>
                </el-tooltip>
              </div>
            </template>
            <div class="flex items-center gap-2 mb-2">
              <el-input
                v-model="fallbackKeyInput"
                :placeholder="$t('providers.drawer.form.fallbackKeysPlaceholder')"
                @keyup.enter="addFallbackKey"
                class="flex-1"
              />
              <el-button @click="addFallbackKey" :disabled="!fallbackKeyInput.trim()">
                <el-icon><Plus /></el-icon>
              </el-button>
            </div>
            <div v-if="form.fallback_keys.length" class="flex flex-wrap gap-2">
              <el-tag
                v-for="(key, idx) in form.fallback_keys"
                :key="idx"
                closable
                @close="() => removeFallbackKey(idx)"
                size="default"
                type="info"
                effect="plain"
                class="rounded-md!"
              >
                {{ key.slice(0, 8) }}{{ key.length > 8 ? '...' : '' }}
              </el-tag>
            </div>
          </el-form-item>

          <el-divider />

          <div class="section-title">{{ $t('providers.drawer.section.config') }}</div>
          <el-form-item :label="$t('providers.drawer.form.format')">
            <el-select v-model="form.format" class="w-full">
              <el-option label="chat" value="chat" />
              <el-option label="anthropic" value="anthropic" />
              <el-option label="responses" value="responses" />
              <el-option label="gemini" value="gemini" />
            </el-select>
          </el-form-item>

          <el-form-item v-if="!isEdit">
            <template #label>
              <div class="flex items-center gap-1">
                <span>{{ $t('providers.drawer.form.defaultModel') }}</span>
                <span class="text-[10px] text-slate-400 font-normal">(Optional)</span>
                <el-tooltip :content="$t('providers.drawer.form.defaultModelTip')" placement="top">
                  <el-icon :size="12" class="text-slate-400 cursor-help"><QuestionFilled /></el-icon>
                </el-tooltip>
              </div>
            </template>
            <el-input
              v-model="form.default_model"
              :placeholder="$t('providers.drawer.form.defaultModelPlaceholder')"
            />
          </el-form-item>

          <el-form-item>
            <template #label>
              {{ $t('providers.drawer.form.enableProxy') }}
              <el-text type="info" size="small" style="margin-left: 8px">
                {{ $t('providers.drawer.form.enableProxyTip') }}
              </el-text>
            </template>
            <el-switch v-model="form.enable_proxy" />
          </el-form-item>

          <el-form-item>
            <template #label>
              <div class="flex items-center gap-1">
                <span>{{ $t('providers.drawer.form.thinkTag') }}</span>
                <span class="text-[10px] text-slate-400 font-normal">(Optional)</span>
                <el-tooltip :content="$t('providers.drawer.form.thinkTagTip')" placement="top">
                  <el-icon :size="12" class="text-slate-400 cursor-help"><QuestionFilled /></el-icon>
                </el-tooltip>
              </div>
            </template>
            <el-input v-model="form.think_tag" placeholder="e.g. think" />
          </el-form-item>

          <el-form-item>
            <template #label>
              <div class="flex items-center gap-1">
                <span>{{ $t('providers.drawer.form.customHeaders') }}</span>
                <span class="text-[10px] text-slate-400 font-normal">(Optional)</span>
                <el-tooltip :content="$t('providers.drawer.form.customHeadersTip')" placement="top">
                  <el-icon :size="12" class="text-slate-400 cursor-help"><QuestionFilled /></el-icon>
                </el-tooltip>
              </div>
            </template>
            <div class="flex items-center gap-2 mb-2 w-full">
              <el-input v-model="headerKeyInput" placeholder="Header name (e.g. User-Agent)" @keyup.enter="addCustomHeader" class="flex-1" />
              <el-input v-model="headerValueInput" placeholder="Value (e.g. claude-code/1.0.0)" @keyup.enter="addCustomHeader" class="flex-1" />
              <el-button @click="addCustomHeader" :disabled="!headerKeyInput.trim()">
                <el-icon><Plus /></el-icon>
              </el-button>
            </div>
            <div v-if="Object.keys(form.custom_headers).length" class="flex flex-wrap gap-2 w-full">
              <el-tag
                v-for="(v, k) in form.custom_headers"
                :key="k"
                closable
                @close="() => removeCustomHeader(String(k))"
                size="default"
                type="info"
                effect="plain"
                class="rounded-md!"
              >
                {{ k }}: {{ String(v).slice(0, 16) }}{{ String(v).length > 16 ? '...' : '' }}
              </el-tag>
            </div>
          </el-form-item>

          <el-divider />

          <div class="section-title">{{ $t('providers.drawer.form.models') }}</div>
          <div class="flex items-center gap-2 mb-3">
            <el-button
              type="primary"
              :loading="fetchingModels"
              :disabled="!form.base_url || !form.format"
              @click="handleFetchModels"
              class="flex-1 shadow-sm"
            >
              <el-icon class="mr-1"><Refresh /></el-icon> {{ $t('providers.drawer.form.fetchModels') }}
            </el-button>
          </div>
          <div v-if="fetchedModels.length > 0" class="text-xs text-emerald-600 mb-2 flex items-center">
            <el-icon class="mr-1"><Check /></el-icon>
            {{ $t('providers.drawer.form.fetchSuccessDesc', { count: fetchedModels.length }) }}
          </div>
          <el-select
            v-model="form.models"
            multiple
            filterable
            allow-create
            default-first-option
            :placeholder="$t('providers.drawer.form.addModel')"
            class="w-full"
          >
            <el-option
              v-for="m in fetchedModels"
              :key="m.id"
              :label="m.id"
              :value="m.id"
            />
          </el-select>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-2">
          <el-button @click="showDrawer = false">{{ $t('providers.actions.cancel') }}</el-button>
          <el-button type="primary" @click="handleSubmit" style="min-width: 100px">
            {{ isEdit ? $t('providers.actions.edit') : $t('providers.add') }}
          </el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style lang="scss" scoped>
.provider-table {
  :deep(.el-table__header) th {
    background-color: #f8fafc;
    color: #64748b;
    font-weight: 600;
    text-transform: uppercase;
    font-size: 11px;
    letter-spacing: 0.05em;
  }
}

.preset-card {
  display: flex;
  align-items: center;
  padding: 10px;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  cursor: pointer;
  transition: all 0.2s;
  background: #ffffff;
  color: #1e293b;
  
  &:hover {
    border-color: var(--el-color-primary);
    background: #eff6ff;
  }
  
  &.active {
    border-color: var(--el-color-primary);
    background: #eff6ff;
    color: var(--el-color-primary);
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
  }
}

.custom-form {
  :deep(.el-form-item__label) {
    font-weight: 600;
    color: var(--v3-form-label-color);
    padding-bottom: 4px;
  }
}

.provider-form {
  :deep(.el-form-item__label) {
    font-weight: 600;
    color: #475569;
    padding-bottom: 4px;
  }
  :deep(.el-form-item) {
    margin-bottom: 20px;
  }
}

.section-title {
  font-size: 13px;
  font-weight: 700;
  color: #334155;
  margin-bottom: 16px;
  padding-left: 8px;
  border-left: 3px solid var(--el-color-primary);
}
</style>
