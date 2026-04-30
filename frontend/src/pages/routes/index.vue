<script setup lang="ts">
import { ref, computed, onMounted, reactive } from "vue"
import { ElMessage, ElMessageBox } from "element-plus"
import { Plus, Delete, Edit, Close, Search, Refresh, CopyDocument, MagicStick, Warning, Right, QuestionFilled } from "@element-plus/icons-vue"
import { listRoutes, createRoute, updateRoute, deleteRoute, generateKey, updateDefaultRoutes, type Route } from "@/api/routes"
import { listProviders, type Provider } from "@/api/providers"
import { getAdminStatus } from "@/api/stats"
import { useConfirm } from "@@/composables/useConfirm"
import { useI18n } from "vue-i18n"

const { t } = useI18n()
const routes = ref<Route[]>([])
const providers = ref<Provider[]>([])
const loading = ref(true)
const savingDefaults = ref(false)
const showDrawer = ref(false)
const isEdit = ref(false)
const editKey = ref("")
const form = ref<any>({})
const searchQuery = ref("")
const { confirmState, toggle: toggleDelete, reset: resetDelete } = useConfirm()

// Helper to normalize values (null/undefined -> "")
const normalize = (v: any) => v ?? ""

// Current values in the UI
const defaultRoutes = ref({
  default_route: "",
  default_anthropic_route: "",
  default_responses_route: "",
  default_chat_route: ""
})

// Backup for dirty checking
const originalDefaults = ref({
  default_route: "",
  default_anthropic_route: "",
  default_responses_route: "",
  default_chat_route: ""
})

const sceneMapData = ref<{ key: string; provider: string; model: string }[]>([])
const modelMapData = ref<{ key: string; provider: string; model: string }[]>([])
const sceneOptions = ["default", "think", "background", "websearch", "longContext", "image"]

const getProviderModels = (providerKey: string) => {
  if (!providerKey) return []
  const p = providers.value.find(x => x.key === providerKey)
  return p?.models || []
}

const providerModels = computed(() => getProviderModels(form.value.provider))

const filteredRoutes = computed(() => {
  if (!searchQuery.value) return routes.value
  const q = searchQuery.value.toLowerCase()
  return routes.value.filter(r => r.key.toLowerCase().includes(q) || r.provider.toLowerCase().includes(q) || r.default_model.toLowerCase().includes(q))
})

// Reactive dirty checking
const isDirty = computed(() => {
  const curr = defaultRoutes.value
  const orig = originalDefaults.value
  return normalize(curr.default_route) !== orig.default_route ||
         normalize(curr.default_anthropic_route) !== orig.default_anthropic_route ||
         normalize(curr.default_responses_route) !== orig.default_responses_route ||
         normalize(curr.default_chat_route) !== orig.default_chat_route
})

async function load() {
  loading.value = true
  try { 
    const [r, p, s] = await Promise.all([listRoutes(), listProviders(), getAdminStatus()])
    routes.value = (r.data || []).map(route => ({ ...route, disabled: !!route.disabled }))
    providers.value = p.data
    
    const status = s.data
    if (status) {
      const values = {
        default_route: normalize(status.default_route),
        default_anthropic_route: normalize(status.default_anthropic_route),
        default_responses_route: normalize(status.default_responses_route),
        default_chat_route: normalize(status.default_chat_route)
      }
      defaultRoutes.value = { ...values }
      originalDefaults.value = { ...values }
    }
    
    routes.value.forEach(r => resetDelete(r.key))
  } finally { 
    loading.value = false 
  }
}

async function saveDefaultRoutes() {
  const payload: any = {}
  const curr = defaultRoutes.value
  const orig = originalDefaults.value

  if (normalize(curr.default_route) !== orig.default_route) payload.default_route = normalize(curr.default_route)
  if (normalize(curr.default_anthropic_route) !== orig.default_anthropic_route) payload.default_anthropic_route = normalize(curr.default_anthropic_route)
  if (normalize(curr.default_responses_route) !== orig.default_responses_route) payload.default_responses_route = normalize(curr.default_responses_route)
  if (normalize(curr.default_chat_route) !== orig.default_chat_route) payload.default_chat_route = normalize(curr.default_chat_route)

  const changeCount = Object.keys(payload).length
  if (changeCount === 0) {
    ElMessage.info(t("routes.strategy.noChanges"))
    return
  }

  savingDefaults.value = true
  try {
    await updateDefaultRoutes(payload)
    originalDefaults.value = { ...defaultRoutes.value }
    ElMessage.success(t("routes.strategy.success"))
  } catch (e) {
    ElMessage.error(t("routes.strategy.fail"))
  } finally {
    savingDefaults.value = false
  }
}

async function toggleDisabled(row: Route) {
  try {
    await updateRoute(row.key, { disabled: row.disabled })
    ElMessage.success(t("routes.actions.toggleSuccess"))
  } catch (e) {
    row.disabled = !row.disabled // revert on failure
    ElMessage.error(t("routes.actions.failSave"))
  }
}

function openCreate() {
  isEdit.value = false; editKey.value = ""
  form.value = { key: "", provider: "", default_model: "", disabled: false, scene_map: {}, model_map: {}, long_context_threshold: 0 }
  sceneMapData.value = []; modelMapData.value = []; showDrawer.value = true
}

function parseModelValue(v: string, fallbackProvider: string) {
  if (v && v.includes(':')) {
    const idx = v.indexOf(':')
    return { provider: v.substring(0, idx), model: v.substring(idx + 1) }
  }
  return { provider: fallbackProvider, model: v || "" }
}

function openEdit(row: Route) {
  isEdit.value = true; editKey.value = row.key
  form.value = { 
    key: row.key, 
    provider: row.provider, 
    default_model: row.default_model, 
    disabled: !!row.disabled,
    long_context_threshold: row.long_context_threshold || 0 
  }
  sceneMapData.value = Object.entries(row.scene_map || {}).map(([k, v]) => ({ key: k, ...parseModelValue(v, row.provider) }))
  modelMapData.value = Object.entries(row.model_map || {}).map(([k, v]) => ({ key: k, ...parseModelValue(v, row.provider) }))
  showDrawer.value = true
}

async function handleGenerateKey() { 
  const data = await generateKey()
  form.value.key = data.data.key 
}

async function handleDelete(key: string) {
  await deleteRoute(key)
  resetDelete(key)
  ElMessage.success(t("routes.actions.successDelete"))
  load()
}

async function handleSubmit() {
  const sm: Record<string, string> = {}
  for (const item of sceneMapData.value) { 
    if (item.key && item.provider && item.model) sm[item.key] = `${item.provider}:${item.model}` 
  }
  const mm: Record<string, string> = {}
  for (const item of modelMapData.value) { 
    if (item.key && item.provider && item.model) mm[item.key] = `${item.provider}:${item.model}` 
  }
  
  form.value.scene_map = sm
  form.value.model_map = mm

  try {
    let res
    if (isEdit.value) { 
      const data = { ...form.value }; delete data.key
      res = await updateRoute(editKey.value, data)
      ElMessage.success(t("routes.actions.successUpdate")) 
    } else { 
      res = await createRoute(form.value)
      ElMessage.success(t("routes.actions.successAdd")) 
    }

    const warnings = res.data.warnings
    if (Array.isArray(warnings) && warnings.length > 0) {
      warnings.forEach(w => ElMessage.warning({ message: w, duration: 5000, showClose: true }))
    }
    showDrawer.value = false; load()
  } catch (e) {
    ElMessage.error(t("routes.actions.failSave"))
  }
}

function addSceneEntry() { sceneMapData.value.push({ key: "", provider: form.value.provider || (providers.value[0]?.key || ""), model: "" }) }
function removeSceneEntry(idx: number) { sceneMapData.value.splice(idx, 1) }
function addModelEntry() { modelMapData.value.push({ key: "", provider: form.value.provider || (providers.value[0]?.key || ""), model: "" }) }
function removeModelEntry(idx: number) { modelMapData.value.splice(idx, 1) }

function onProviderChange(item: any) {
  item.model = "" // Clear model when provider changes to force re-selection
}

const copyToClipboard = (text: string) => {
  navigator.clipboard.writeText(text)
  ElMessage.success(t("routes.actions.copySuccess"))
}

onMounted(load)
</script>

<template>
  <div class="app-container">
    <div class="page-header">
      <div>
        <h3>{{ $t('routes.title') }}</h3>
        <p class="text-sm text-slate-500 mt-1">{{ $t('routes.desc') }}</p>
      </div>
      <div class="flex gap-3">
        <el-input v-model="searchQuery" :placeholder="$t('routes.search')" :prefix-icon="Search" clearable style="width: 240px" />
        <el-button type="primary" :icon="Plus" @click="openCreate">{{ $t('routes.add') }}</el-button>
        <el-button :icon="Refresh" circle @click="load" />
      </div>
    </div>

    <!-- Default Strategy Card -->
    <el-card shadow="never" :class="['mb-6 border-slate-200! transition-all duration-300 shadow-sm!', isDirty ? 'bg-orange-50! border-orange-300!' : 'bg-slate-50/50!']">
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex flex-col gap-1">
            <div class="flex items-center gap-2">
              <span class="text-sm font-bold text-slate-800">{{ $t('routes.strategy.title') }}</span>
              <el-tag v-if="isDirty" size="small" type="warning" effect="dark" class="animate-pulse border-none!">{{ $t('routes.strategy.pending') }}</el-tag>
            </div>
            <div class="text-[12px] text-slate-500 font-normal">{{ $t('routes.strategy.tip') }}</div>
          </div>
          <el-button 
            :type="isDirty ? 'warning' : 'primary'" 
            size="default" 
            @click="saveDefaultRoutes" 
            :loading="savingDefaults" 
            :plain="!isDirty"
            class="transition-all duration-300 font-bold px-6!"
            :style="isDirty ? 'transform: scale(1.02); box-shadow: 0 4px 12px rgba(230, 162, 60, 0.2)' : ''"
          >
            {{ isDirty ? $t('routes.strategy.apply') : $t('routes.strategy.save') }}
          </el-button>
        </div>
      </template>
      <el-row :gutter="20">
        <el-col :span="6">
          <div class="flex items-center gap-1 mb-2">
            <div class="text-[11px] font-bold text-slate-500 uppercase tracking-wider">{{ $t('routes.strategy.global') }}</div>
            <el-tooltip :content="$t('routes.strategy.globalTip')"><el-icon class="text-slate-400 text-[11px] cursor-help"><QuestionFilled /></el-icon></el-tooltip>
          </div>
          <el-select v-model="defaultRoutes.default_route" :placeholder="$t('routes.strategy.notSet')" clearable filterable class="w-full!">
            <el-option v-for="r in routes" :key="r.key" :label="r.key + (r.disabled ? ` (${$t('routes.table.disabled')})` : '')" :value="r.key" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <div class="flex items-center gap-1 mb-2">
            <div class="text-[11px] font-bold text-slate-500 uppercase tracking-wider">{{ $t('routes.strategy.anthropic') }}</div>
            <el-tooltip :content="$t('routes.strategy.anthropicTip')"><el-icon class="text-slate-400 text-[11px] cursor-help"><QuestionFilled /></el-icon></el-tooltip>
          </div>
          <el-select v-model="defaultRoutes.default_anthropic_route" :placeholder="$t('routes.strategy.notSet')" clearable filterable class="w-full!">
            <el-option v-for="r in routes" :key="r.key" :label="r.key + (r.disabled ? ` (${$t('routes.table.disabled')})` : '')" :value="r.key" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <div class="flex items-center gap-1 mb-2">
            <div class="text-[11px] font-bold text-slate-500 uppercase tracking-wider">{{ $t('routes.strategy.responses') }}</div>
            <el-tooltip :content="$t('routes.strategy.responsesTip')"><el-icon class="text-slate-400 text-[11px] cursor-help"><QuestionFilled /></el-icon></el-tooltip>
          </div>
          <el-select v-model="defaultRoutes.default_responses_route" :placeholder="$t('routes.strategy.notSet')" clearable filterable class="w-full!">
            <el-option v-for="r in routes" :key="r.key" :label="r.key + (r.disabled ? ` (${$t('routes.table.disabled')})` : '')" :value="r.key" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <div class="flex items-center gap-1 mb-2">
            <div class="text-[11px] font-bold text-slate-500 uppercase tracking-wider">{{ $t('routes.strategy.chat') }}</div>
            <el-tooltip :content="$t('routes.strategy.chatTip')"><el-icon class="text-slate-400 text-[11px] cursor-help"><QuestionFilled /></el-icon></el-tooltip>
          </div>
          <el-select v-model="defaultRoutes.default_chat_route" :placeholder="$t('routes.strategy.notSet')" clearable filterable class="w-full!">
            <el-option v-for="r in routes" :key="r.key" :label="r.key + (r.disabled ? ` (${$t('routes.table.disabled')})` : '')" :value="r.key" />
          </el-select>
        </el-col>
      </el-row>
    </el-card>

    <!-- Table -->
    <el-card shadow="never" class="border-none!">
      <el-table :data="filteredRoutes" v-loading="loading" stripe size="large" :row-class-name="({ row }) => row.disabled ? 'opacity-50' : ''">
        <el-table-column :label="$t('routes.table.key')" min-width="220">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="mono text-xs px-2 py-1 rounded truncate max-w-160px border" :style="{ backgroundColor: 'var(--v3-key-bg)', borderColor: 'var(--v3-key-border)', color: 'var(--v3-key-text-color)' }">
                {{ row.key }}
              </span>
              <el-button link @click="copyToClipboard(row.key)"><el-icon><CopyDocument /></el-icon></el-button>
              <el-tag v-if="row.disabled" size="small" type="info" effect="plain" class="ml-1">{{ $t('routes.table.disabled') }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="provider" :label="$t('routes.table.provider')" width="140">
          <template #default="{ row }"><el-tag effect="plain" class="border-slate-200! text-slate-600! font-medium">{{ row.provider }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="default_model" :label="$t('routes.table.model')" min-width="160">
          <template #default="{ row }"><span class="text-sm text-slate-600 truncate block">{{ row.default_model }}</span></template>
        </el-table-column>
        
        <el-table-column :label="$t('routes.table.disabled')" width="100">
          <template #default="{ row }">
            <!-- disabled: false (active) -> green, disabled: true (inactive) -> red -->
            <el-switch v-model="row.disabled" :active-value="true" :inactive-value="false" @change="toggleDisabled(row)" size="small" active-color="var(--el-color-danger)" inactive-color="var(--el-color-success)" />
          </template>
        </el-table-column>

        <el-table-column :label="$t('routes.drawer.form.scenes')" min-width="200">
          <template #default="{ row }">
            <div class="mapping-group">
              <div v-for="(v, k) in row.scene_map" :key="k" class="mapping-pill">
                <span class="m-key">{{ k }}</span>
                <el-icon class="m-arrow"><Right /></el-icon>
                <el-tooltip :content="v" placement="top" :show-after="500">
                  <span class="m-val">{{ v }}</span>
                </el-tooltip>
              </div>
              <span v-if="!Object.keys(row.scene_map || {}).length" class="text-[10px] text-slate-300">—</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column :label="$t('routes.drawer.form.aliases')" min-width="200">
          <template #default="{ row }">
            <div class="mapping-group">
              <div v-for="(v, k) in row.model_map" :key="k" class="mapping-pill">
                <span class="m-key">{{ k }}</span>
                <el-icon class="m-arrow"><Right /></el-icon>
                <el-tooltip :content="v" placement="top" :show-after="500">
                  <span class="m-val">{{ v }}</span>
                </el-tooltip>
              </div>
              <span v-if="!Object.keys(row.model_map || {}).length" class="text-[10px] text-slate-300">—</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column :label="$t('providers.table.actions')" width="140" fixed="right" align="right">
          <template #default="{ row }">
            <div class="flex justify-end gap-1">
              <el-button link type="primary" :icon="Edit" @click="openEdit(row)" />
              <div v-if="confirmState[row.key]" class="flex items-center gap-1">
                <el-button link type="danger" size="small" class="font-medium" @click="handleDelete(row.key)">{{ $t('routes.actions.confirm') }}</el-button>
                <el-button link type="info" size="small" class="font-medium" @click="resetDelete(row.key)">{{ $t('routes.actions.cancel') }}</el-button>
              </div>
              <el-button v-else link type="primary" :icon="Delete" @click="toggleDelete(row.key)" />
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Drawer -->
    <el-drawer
      v-model="showDrawer"
      :title="isEdit ? $t('routes.drawer.edit') : $t('routes.drawer.add')"
      size="680px"
      destroy-on-close
    >
      <div class="px-2 pb-10">
        <div class="logic-flow mb-8 w-full!">
          <span class="logic-title">{{ $t('routes.logic.title') }}:</span>
          <div class="logic-steps">
            <span class="logic-step">{{ $t('routes.logic.aliases') }}</span>
            <el-icon class="logic-arrow"><Right /></el-icon>
            <span class="logic-step">{{ $t('routes.logic.scenes') }}</span>
            <el-icon class="logic-arrow"><Right /></el-icon>
            <span class="logic-step logic-step--final">{{ $t('routes.logic.default') }}</span>
          </div>
        </div>
        <el-form :model="form" label-position="top" class="custom-form">
          <el-form-item :label="$t('routes.drawer.form.key')" required>
            <div class="flex gap-2 w-full">
              <el-input v-model="form.key" :disabled="isEdit" :placeholder="$t('routes.drawer.form.keyPlaceholder')" class="mono-input" />
              <el-button v-if="!isEdit" @click="handleGenerateKey" :icon="MagicStick">{{ $t('routes.drawer.form.autoGen') }}</el-button>
            </div>
          </el-form-item>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item :label="$t('routes.drawer.form.provider')" required>
                <el-select v-model="form.provider" :placeholder="$t('routes.drawer.form.providerPlaceholder')" class="w-full">
                  <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item :label="$t('routes.drawer.form.model')" required>
                <el-select v-model="form.default_model" :placeholder="$t('routes.drawer.form.modelPlaceholder')" class="w-full" filterable allow-create clearable default-first-option>
                  <el-option v-for="m in providerModels" :key="m" :label="m" :value="m" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item :label="$t('routes.drawer.form.threshold')">
                <el-input-number v-model="form.long_context_threshold" :min="0" :step="10000" controls-position="right" class="w-full!" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item :label="$t('routes.drawer.form.disabled')">
                <div class="flex items-center h-32px">
                  <el-switch v-model="form.disabled" :active-value="true" :inactive-value="false" active-color="var(--el-color-danger)" inactive-color="var(--el-color-success)" />
                  <span class="ml-2 text-xs text-slate-400">{{ $t('routes.drawer.form.disabledTip') }}</span>
                </div>
              </el-form-item>
            </el-col>
          </el-row>

          <div class="divider my-6 border-t border-slate-100 border-dashed"></div>

          <!-- Scene Mappings -->
          <div class="mb-6">
            <div class="flex items-center justify-between mb-3">
              <span class="text-sm font-bold text-slate-700">{{ $t('routes.drawer.form.scenes') }}</span>
              <el-button size="small" link type="primary" @click="addSceneEntry">{{ $t('routes.drawer.form.addScene') }}</el-button>
            </div>
            <div class="p-4 rounded-xl border min-h-40px bg-slate-50/50 border-slate-100">
              <div v-for="(item, idx) in sceneMapData" :key="idx" class="flex gap-3 mb-3 items-center last:mb-0">
                <el-select v-model="item.key" :placeholder="$t('routes.drawer.form.scenePlaceholder')" class="w-130px shrink-0" size="default">
                  <el-option v-for="s in sceneOptions" :key="s" :label="s" :value="s" />
                </el-select>
                <el-icon class="text-slate-300"><Right /></el-icon>
                <div class="flex-1 flex gap-2 items-center bg-white p-1 rounded-lg border border-slate-100 shadow-sm">
                  <el-select v-model="item.provider" placeholder="Provider" class="w-130px" filterable size="default" @change="onProviderChange(item)">
                    <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
                  </el-select>
                  <span class="text-slate-300 font-bold">:</span>
                  <el-select v-model="item.model" placeholder="Select Model" class="flex-1" filterable allow-create default-first-option size="default">
                    <el-option v-for="m in getProviderModels(item.provider)" :key="m" :label="m" :value="m" />
                  </el-select>
                </div>
                <el-button link type="danger" :icon="Delete" @click="removeSceneEntry(idx)" />
              </div>
              <div v-if="!sceneMapData.length" class="text-center py-4 text-xs text-slate-400 italic">{{ $t('routes.drawer.form.noScenes') }}</div>
            </div>
          </div>

          <!-- Model Aliases -->
          <div>
            <div class="flex items-center justify-between mb-3">
              <span class="text-sm font-bold text-slate-700">{{ $t('routes.drawer.form.aliases') }}</span>
              <el-button size="small" link type="primary" @click="addModelEntry">{{ $t('routes.drawer.form.addAlias') }}</el-button>
            </div>
            <div class="p-4 rounded-xl border min-h-40px bg-slate-50/50 border-slate-100">
              <div v-for="(item, idx) in modelMapData" :key="idx" class="flex gap-3 mb-3 items-center last:mb-0">
                <el-input v-model="item.key" :placeholder="$t('routes.drawer.form.clientModel')" class="w-130px shrink-0" size="default" />
                <el-icon class="text-slate-300"><Right /></el-icon>
                <div class="flex-1 flex gap-2 items-center bg-white p-1 rounded-lg border border-slate-100 shadow-sm">
                  <el-select v-model="item.provider" placeholder="Provider" class="w-130px" filterable size="default" @change="onProviderChange(item)">
                    <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
                  </el-select>
                  <span class="text-slate-300 font-bold">:</span>
                  <el-select v-model="item.model" placeholder="Select Model" class="flex-1" filterable allow-create default-first-option size="default">
                    <el-option v-for="m in getProviderModels(item.provider)" :key="m" :label="m" :value="m" />
                  </el-select>
                </div>
                <el-button link type="danger" :icon="Delete" @click="removeModelEntry(idx)" />
              </div>
              <div v-if="!modelMapData.length" class="text-center py-4 text-xs text-slate-400 italic">{{ $t('routes.drawer.form.noAliases') }}</div>
            </div>
          </div>
        </el-form>
      </div>
      
      <template #footer>
        <div class="flex justify-end gap-3 px-2">
          <el-button @click="showDrawer = false">{{ $t('routes.actions.cancel') }}</el-button>
          <el-button type="primary" @click="handleSubmit" style="min-width: 100px">
            {{ isEdit ? $t('routes.actions.edit') : $t('routes.drawer.add') }}
          </el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style lang="scss" scoped>
.logic-flow {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background-color: #fbfaf8;
  padding: 10px 16px;
  border-radius: 12px;
  border: 1px solid var(--el-border-color-light);
}

.logic-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  color: #a39e93;
  letter-spacing: 0.05em;
}

.logic-steps {
  display: flex;
  align-items: center;
  gap: 8px;
}

.logic-step {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  background: #ffffff;
  padding: 2px 10px;
  border-radius: 6px;
  border: 1px solid var(--el-border-color);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.03);
}

.logic-step--final {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary-light-7);
  background: var(--el-color-primary-light-9);
}

.logic-arrow {
  color: #d1d1d1;
  font-size: 14px;
}

.mapping-group {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.mapping-pill {
  display: flex;
  align-items: center;
  gap: 4px;
  background-color: #f1f5f9;
  border: 1px solid #e2e8f0;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  max-width: 220px;

  .m-key {
    font-weight: 700;
    color: #475569;
  }
  .m-arrow {
    color: #94a3b8;
    font-size: 10px;
  }
  .m-val {
    color: #0f172a;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 120px;
  }
}

html.dark {
  .logic-flow {
    background-color: #1e293b;
    border-color: #334155;
  }
  .logic-step {
    background: #0f172a;
    border-color: #334155;
    color: #f1f5f9;
  }
  .logic-step--final {
    background: rgba(217, 119, 87, 0.1);
    border-color: rgba(217, 119, 87, 0.4);
    color: var(--el-color-primary);
  }
  .mapping-pill {
    background-color: #1e293b;
    border-color: #334155;
    .m-key { color: #cbd5e1; }
    .m-val { color: #f1f5f9; }
  }
}

.custom-form {
  :deep(.el-form-item__label) {
    font-weight: 600;
    color: var(--v3-form-label-color);
    padding-bottom: 4px;
    font-size: 13px;
  }
}

.mono-input :deep(input) {
  font-family: var(--el-font-family-mono);
  font-size: 12px;
}

.w-full\! {
  width: 100% !important;
}

:deep(.disabled-row) {
  opacity: 0.5;
  filter: grayscale(0.8);
  background-color: var(--el-fill-color-light) !important;
  
  .mono {
    background-color: var(--el-fill-color) !important;
    color: var(--el-text-color-secondary) !important;
    border-color: var(--el-border-color-lighter) !important;
  }
}
</style>
