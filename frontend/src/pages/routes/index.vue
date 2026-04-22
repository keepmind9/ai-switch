<script setup lang="ts">
import { ref, computed, onMounted } from "vue"
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

const sceneMapData = ref<{ key: string; value: string }[]>([])
const modelMapData = ref<{ key: string; value: string }[]>([])
const sceneOptions = ["default", "think", "background", "websearch", "longContext", "image"]

const providerModels = computed(() => {
  const p = providers.value.find(x => x.key === form.value.provider)
  return p?.models || []
})

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
    routes.value = r.data.data
    providers.value = p.data.data 
    
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

  // Check each field and explicitly use "" for cleared fields (Partial Update)
  if (normalize(curr.default_route) !== orig.default_route) {
    payload.default_route = normalize(curr.default_route)
  }
  if (normalize(curr.default_anthropic_route) !== orig.default_anthropic_route) {
    payload.default_anthropic_route = normalize(curr.default_anthropic_route)
  }
  if (normalize(curr.default_responses_route) !== orig.default_responses_route) {
    payload.default_responses_route = normalize(curr.default_responses_route)
  }
  if (normalize(curr.default_chat_route) !== orig.default_chat_route) {
    payload.default_chat_route = normalize(curr.default_chat_route)
  }

  const changeCount = Object.keys(payload).length
  if (changeCount === 0) {
    ElMessage.info(t("routes.strategy.noChanges"))
    return
  }

  savingDefaults.value = true
  try {
    console.log(`[DefaultRoutes] Saving ${changeCount} field(s):`, payload)
    await updateDefaultRoutes(payload)
    
    // Sync UI state back to original to clear 'isDirty'
    originalDefaults.value = {
      default_route: normalize(curr.default_route),
      default_anthropic_route: normalize(curr.default_anthropic_route),
      default_responses_route: normalize(curr.default_responses_route),
      default_chat_route: normalize(curr.default_chat_route)
    }
    ElMessage.success(t("routes.strategy.success"))
  } catch (e) {
    console.error("[DefaultRoutes] Update failed:", e)
    ElMessage.error(t("routes.strategy.fail"))
  } finally {
    savingDefaults.value = false
  }
}

function openCreate() {
  isEdit.value = false; editKey.value = ""
  form.value = { key: "", provider: "", default_model: "", scene_map: {}, model_map: {}, long_context_threshold: 0 }
  sceneMapData.value = []; modelMapData.value = []; showDrawer.value = true
}

function openEdit(row: Route) {
  isEdit.value = true; editKey.value = row.key
  form.value = { 
    key: row.key, 
    provider: row.provider, 
    default_model: row.default_model, 
    long_context_threshold: row.long_context_threshold || 0 
  }
  sceneMapData.value = Object.entries(row.scene_map || {}).map(([k, v]) => ({ key: k, value: v }))
  modelMapData.value = Object.entries(row.model_map || {}).map(([k, v]) => ({ key: k, value: v }))
  showDrawer.value = true
}

async function handleGenerateKey() { 
  const { data } = await generateKey()
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
  for (const item of sceneMapData.value) { if (item.key) sm[item.key] = item.value }
  const mm: Record<string, string> = {}
  for (const item of modelMapData.value) { if (item.key) mm[item.key] = item.value }
  
  form.value.scene_map = sm
  form.value.model_map = mm

  try {
    let res
    if (isEdit.value) { 
      const data = { ...form.value }
      delete data.key
      res = await updateRoute(editKey.value, data)
      ElMessage.success(t("routes.actions.successUpdate")) 
    } else { 
      res = await createRoute(form.value)
      ElMessage.success(t("routes.actions.successAdd")) 
    }

    const warnings = res.data?.warnings
    if (Array.isArray(warnings) && warnings.length > 0) {
      warnings.forEach(w => ElMessage.warning({ message: w, duration: 5000, showClose: true }))
    }
    
    showDrawer.value = false
    load()
  } catch (e) {
    ElMessage.error(t("routes.actions.failSave"))
  }
}

function addSceneEntry() { sceneMapData.value.push({ key: "", value: "" }) }
function removeSceneEntry(idx: number) { sceneMapData.value.splice(idx, 1) }
function addModelEntry() { modelMapData.value.push({ key: "", value: "" }) }
function removeModelEntry(idx: number) { modelMapData.value.splice(idx, 1) }

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

    <!-- Default Routing Strategy Panel -->
    <el-card shadow="never" :class="['mb-6 border-slate-200! transition-all duration-300 shadow-sm!', isDirty ? 'bg-orange-50! border-orange-300!' : 'bg-slate-50/50!']">
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="text-sm font-bold text-slate-800">{{ $t('routes.strategy.title') }}</span>
            <el-tooltip :content="$t('routes.strategy.tip')">
              <el-icon class="text-slate-500 cursor-help"><QuestionFilled /></el-icon>
            </el-tooltip>
            <el-tag v-if="isDirty" size="small" type="warning" effect="dark" class="animate-pulse border-none!">{{ $t('routes.strategy.pending') }}</el-tag>
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
            <el-tooltip :content="$t('routes.strategy.globalTip')">
              <el-icon class="text-slate-400 text-[11px] cursor-help"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <el-select v-model="defaultRoutes.default_route" :placeholder="$t('routes.strategy.notSet')" clearable filterable class="w-full!">
            <el-option v-for="r in routes" :key="r.key" :label="r.key" :value="r.key" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <div class="flex items-center gap-1 mb-2">
            <div class="text-[11px] font-bold text-slate-500 uppercase tracking-wider">{{ $t('routes.strategy.anthropic') }}</div>
            <el-tooltip :content="$t('routes.strategy.anthropicTip')">
              <el-icon class="text-slate-400 text-[11px] cursor-help"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <el-select v-model="defaultRoutes.default_anthropic_route" :placeholder="$t('routes.strategy.notSet')" clearable filterable class="w-full!">
            <el-option v-for="r in routes" :key="r.key" :label="r.key" :value="r.key" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <div class="flex items-center gap-1 mb-2">
            <div class="text-[11px] font-bold text-slate-500 uppercase tracking-wider">{{ $t('routes.strategy.responses') }}</div>
            <el-tooltip :content="$t('routes.strategy.responsesTip')">
              <el-icon class="text-slate-400 text-[11px] cursor-help"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <el-select v-model="defaultRoutes.default_responses_route" :placeholder="$t('routes.strategy.notSet')" clearable filterable class="w-full!">
            <el-option v-for="r in routes" :key="r.key" :label="r.key" :value="r.key" />
          </el-select>
        </el-col>
        <el-col :span="6">
          <div class="flex items-center gap-1 mb-2">
            <div class="text-[11px] font-bold text-slate-500 uppercase tracking-wider">{{ $t('routes.strategy.chat') }}</div>
            <el-tooltip :content="$t('routes.strategy.chatTip')">
              <el-icon class="text-slate-400 text-[11px] cursor-help"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <el-select v-model="defaultRoutes.default_chat_route" :placeholder="$t('routes.strategy.notSet')" clearable filterable class="w-full!">
            <el-option v-for="r in routes" :key="r.key" :label="r.key" :value="r.key" />
          </el-select>
        </el-col>
      </el-row>
    </el-card>

    <el-card shadow="never" class="border-none!">
      <el-table :data="filteredRoutes" v-loading="loading" stripe size="large">
        <el-table-column :label="$t('routes.table.key')" min-width="240">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="mono text-xs px-2 py-1 rounded truncate max-w-180px border" :style="{ backgroundColor: 'var(--v3-key-bg)', borderColor: 'var(--v3-key-border)', color: 'var(--v3-key-text-color)' }">
                {{ row.key }}
              </span>
              <el-button link @click="copyToClipboard(row.key)"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="provider" :label="$t('routes.table.provider')" width="140">
          <template #default="{ row }">
            <el-tag effect="plain" class="border-slate-200! text-slate-600! font-medium capitalize">{{ row.provider }}</el-tag>
          </template>
        </el-table-column>
        
        <el-table-column prop="default_model" :label="$t('routes.table.model')" min-width="180">
          <template #default="{ row }">
            <span class="text-sm text-slate-600">{{ row.default_model }}</span>
          </template>
        </el-table-column>
        
        <el-table-column :label="$t('routes.table.mappings')" min-width="300">
          <template #default="{ row }">
            <div class="flex flex-wrap gap-1">
              <el-tooltip v-for="(v, k) in row.scene_map" :key="'s'+k" :content="`Scene Map: ${k} → ${v}`">
                <el-tag size="small" type="primary" effect="light" class="rounded-md!">S:{{ k }}</el-tag>
              </el-tooltip>
              <el-tooltip v-for="(v, k) in row.model_map" :key="'m'+k" :content="`Model Map: ${k} → ${v}`">
                <el-tag size="small" type="info" effect="light" class="rounded-md!">M:{{ k }}</el-tag>
              </el-tooltip>
              <span v-if="!Object.keys(row.scene_map || {}).length && !Object.keys(row.model_map || {}).length" class="text-xs text-slate-300">—</span>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column :label="$t('providers.table.actions')" width="160" fixed="right" align="right">
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

    <el-drawer
      v-model="showDrawer"
      :title="isEdit ? $t('routes.drawer.edit') : $t('routes.drawer.add')"
      size="550px"
      destroy-on-close
    >
      <div class="px-2 pb-10">
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

          <el-form-item :label="$t('routes.drawer.form.threshold')">
            <el-input-number v-model="form.long_context_threshold" :min="0" :step="10000" controls-position="right" class="w-full!" />
          </el-form-item>

          <div class="divider my-6 border-t border-slate-100 border-dashed"></div>

          <div class="mb-6">
            <div class="flex items-center justify-between mb-3">
              <span class="text-sm font-bold text-slate-700">{{ $t('routes.drawer.form.scenes') }}</span>
              <el-button size="small" link type="primary" @click="addSceneEntry">{{ $t('routes.drawer.form.addScene') }}</el-button>
            </div>
            <div class="p-3 rounded-xl border min-h-40px" style="background-color: var(--v3-section-bg); border-color: var(--el-border-color-light)">
              <div v-for="(item, idx) in sceneMapData" :key="idx" class="flex gap-2 mb-2 items-center">
                <el-select v-model="item.key" :placeholder="$t('routes.drawer.form.scenePlaceholder')" class="w-140px shrink-0" size="default">
                  <el-option v-for="s in sceneOptions" :key="s" :label="s" :value="s" />
                </el-select>
                <el-icon class="text-slate-300"><Right /></el-icon>
                <el-select v-model="item.value" :placeholder="$t('routes.drawer.form.mapTo')" class="flex-1" filterable allow-create default-first-option size="default">
                  <el-option v-for="m in providerModels" :key="m" :label="m" :value="m" />
                </el-select>
                <el-button link type="danger" :icon="Delete" @click="removeSceneEntry(idx)" />
              </div>
              <div v-if="!sceneMapData.length" class="text-center py-4 text-xs text-slate-400 italic">{{ $t('routes.drawer.form.noScenes') }}</div>
            </div>
          </div>

          <div>
            <div class="flex items-center justify-between mb-3">
              <span class="text-sm font-bold text-slate-700">{{ $t('routes.drawer.form.aliases') }}</span>
              <el-button size="small" link type="primary" @click="addModelEntry">{{ $t('routes.drawer.form.addAlias') }}</el-button>
            </div>
            <div class="p-3 rounded-xl border min-h-40px" style="background-color: var(--v3-section-bg); border-color: var(--el-border-color-light)">
              <div v-for="(item, idx) in modelMapData" :key="idx" class="flex gap-2 mb-2 items-center">
                <el-input v-model="item.key" :placeholder="$t('routes.drawer.form.clientModel')" class="flex-1" size="default" />
                <el-icon class="text-slate-300"><Right /></el-icon>
                <el-select v-model="item.value" :placeholder="$t('routes.drawer.form.upstreamModel')" class="flex-1" filterable allow-create default-first-option size="default">
                  <el-option v-for="m in providerModels" :key="m" :label="m" :value="m" />
                </el-select>
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
</style>
