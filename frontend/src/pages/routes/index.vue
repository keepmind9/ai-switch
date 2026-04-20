<script setup lang="ts">
import { ref, computed, onMounted } from "vue"
import { ElMessage, ElMessageBox } from "element-plus"
import { Plus, Delete, Edit, Close, Search, Refresh, CopyDocument, MagicStick, Warning } from "@element-plus/icons-vue"
import { listRoutes, createRoute, updateRoute, deleteRoute, generateKey, type Route } from "@/api/routes"
import { listProviders, type Provider } from "@/api/providers"

const routes = ref<Route[]>([])
const providers = ref<Provider[]>([])
const loading = ref(true)
const showDrawer = ref(false)
const isEdit = ref(false)
const editKey = ref("")
const form = ref<any>({})
const searchQuery = ref("")

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

async function load() {
  loading.value = true
  try { 
    const [r, p] = await Promise.all([listRoutes(), listProviders()])
    routes.value = r.data.data
    providers.value = p.data.data 
  } finally { 
    loading.value = false 
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
  try {
    await ElMessageBox.confirm(`Delete route "${key}"? This will immediately stop access for clients using this key.`, "Delete Route", { 
      type: "warning",
      confirmButtonClass: 'el-button--danger'
    })
    await deleteRoute(key)
    ElMessage.success("Route deleted")
    load()
  } catch (e) {}
}

async function handleSubmit() {
  const sm: Record<string, string> = {}
  for (const item of sceneMapData.value) { if (item.key) sm[item.key] = item.value }
  const mm: Record<string, string> = {}
  for (const item of modelMapData.value) { if (item.key) mm[item.key] = item.value }
  
  form.value.scene_map = sm
  form.value.model_map = mm

  try {
    if (isEdit.value) { 
      const data = { ...form.value }
      delete data.key
      await updateRoute(editKey.value, data)
      ElMessage.success("Route updated") 
    } else { 
      await createRoute(form.value)
      ElMessage.success("Route created") 
    }
    showDrawer.value = false
    load()
  } catch (e) {
    ElMessage.error("Failed to save route")
  }
}

function addSceneEntry() { sceneMapData.value.push({ key: "", value: "" }) }
function removeSceneEntry(idx: number) { sceneMapData.value.splice(idx, 1) }
function addModelEntry() { modelMapData.value.push({ key: "", value: "" }) }
function removeModelEntry(idx: number) { modelMapData.value.splice(idx, 1) }

const copyToClipboard = (text: string) => {
  navigator.clipboard.writeText(text)
  ElMessage.success("Copied to clipboard")
}

onMounted(load)
</script>

<template>
  <div class="app-container">
    <div class="page-header">
      <div>
        <h3>Access Routes</h3>
        <p class="text-sm text-slate-500 mt-1">Configure API keys and model routing logic for your clients.</p>
      </div>
      <div class="flex gap-3">
        <el-input v-model="searchQuery" placeholder="Search routes..." :prefix-icon="Search" clearable style="width: 240px" />
        <el-button type="primary" :icon="Plus" @click="openCreate">Add Route</el-button>
        <el-button :icon="Refresh" circle @click="load" />
      </div>
    </div>

    <!-- Table -->
    <el-card shadow="never" class="border-none!">
      <el-table :data="filteredRoutes" v-loading="loading" stripe size="large">
        <el-table-column label="Gateway Key" min-width="240">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="mono text-xs px-2 py-1 rounded truncate max-w-180px border" :style="{ backgroundColor: 'var(--v3-key-bg)', borderColor: 'var(--v3-key-border)', color: 'var(--v3-key-text-color)' }">
                {{ row.key }}
              </span>
              <el-button link @click="copyToClipboard(row.key)"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="provider" label="Provider" width="140">
          <template #default="{ row }">
            <el-tag effect="plain" class="border-slate-200! text-slate-600! font-medium capitalize">{{ row.provider }}</el-tag>
          </template>
        </el-table-column>
        
        <el-table-column prop="default_model" label="Default Model" min-width="180">
          <template #default="{ row }">
            <span class="text-sm font-semibold text-slate-700">{{ row.default_model }}</span>
          </template>
        </el-table-column>
        
        <el-table-column label="Mappings" min-width="300">
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
        
        <el-table-column label="Actions" width="100" fixed="right" align="right">
          <template #default="{ row }">
            <div class="flex justify-end gap-1">
              <el-button link type="primary" :icon="Edit" @click="openEdit(row)" />
              <el-button link type="danger" :icon="Delete" @click="handleDelete(row.key)" />
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Form Drawer -->
    <el-drawer
      v-model="showDrawer"
      :title="isEdit ? 'Edit Routing Configuration' : 'Create New Route'"
      size="550px"
      destroy-on-close
    >
      <div class="px-2 pb-10">
        <el-form :model="form" label-position="top" class="custom-form">
          <el-form-item label="Gateway API Key" required>
            <div class="flex gap-2 w-full">
              <el-input v-model="form.key" :disabled="isEdit" placeholder="Client will use this key" class="mono-input" />
              <el-button v-if="!isEdit" @click="handleGenerateKey" :icon="MagicStick">Auto-Gen</el-button>
            </div>
            <div class="text-[11px] text-slate-400 mt-1 flex items-center gap-1">
              <el-icon><Warning /></el-icon> This key is what clients will put in their `Authorization` header.
            </div>
          </el-form-item>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Upstream Provider" required>
                <el-select v-model="form.provider" placeholder="Select target" class="w-full">
                  <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Default Model" required>
                <el-select v-model="form.default_model" placeholder="Fallback model" class="w-full" filterable allow-create clearable default-first-option>
                  <el-option v-for="m in providerModels" :key="m" :label="m" :value="m" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item label="Long Context Threshold">
            <el-input-number v-model="form.long_context_threshold" :min="0" :step="10000" controls-position="right" class="w-full!" />
            <div class="text-[11px] text-slate-400 mt-1">Switch to 'longContext' scene if total tokens exceed this value. 0 to disable.</div>
          </el-form-item>

          <div class="divider my-6 border-t border-slate-100 border-dashed"></div>

          <!-- Scene Map Editor -->
          <div class="mb-6">
            <div class="flex items-center justify-between mb-3">
              <span class="text-sm font-bold text-slate-700 dark:text-slate-200">Scene Mappings</span>
              <el-button size="small" link type="primary" @click="addSceneEntry">+ Add Scene</el-button>
            </div>
            <div class="p-3 rounded-xl border min-h-40px" style="background-color: var(--v3-section-bg); border-color: var(--el-border-color-light)">
              <div v-for="(item, idx) in sceneMapData" :key="idx" class="flex gap-2 mb-2 items-center">
                <el-select v-model="item.key" placeholder="Scene" class="w-140px shrink-0" size="default">
                  <el-option v-for="s in sceneOptions" :key="s" :label="s" :value="s" />
                </el-select>
                <el-icon class="text-slate-300"><Right /></el-icon>
                <el-select v-model="item.value" placeholder="Map to model" class="flex-1" filterable allow-create default-first-option size="default">
                  <el-option v-for="m in providerModels" :key="m" :label="m" :value="m" />
                </el-select>
                <el-button link type="danger" :icon="Delete" @click="removeSceneEntry(idx)" />
              </div>
              <div v-if="!sceneMapData.length" class="text-center py-4 text-xs text-slate-400 italic">No scene-based routing defined.</div>
            </div>
          </div>

          <!-- Model Map Editor -->
          <div>
            <div class="flex items-center justify-between mb-3">
              <span class="text-sm font-bold text-slate-700 dark:text-slate-200">Model Aliases</span>
              <el-button size="small" link type="primary" @click="addModelEntry">+ Add Mapping</el-button>
            </div>
            <div class="p-3 rounded-xl border min-h-40px" style="background-color: var(--v3-section-bg); border-color: var(--el-border-color-light)">
              <div v-for="(item, idx) in modelMapData" :key="idx" class="flex gap-2 mb-2 items-center">
                <el-input v-model="item.key" placeholder="Client model name" class="flex-1" size="default" />
                <el-icon class="text-slate-300"><Right /></el-icon>
                <el-select v-model="item.value" placeholder="Upstream model" class="flex-1" filterable allow-create default-first-option size="default">
                  <el-option v-for="m in providerModels" :key="m" :label="m" :value="m" />
                </el-select>
                <el-button link type="danger" :icon="Delete" @click="removeModelEntry(idx)" />
              </div>
              <div v-if="!modelMapData.length" class="text-center py-4 text-xs text-slate-400 italic">No model aliases defined.</div>
            </div>
          </div>
        </el-form>
      </div>
      
      <template #footer>
        <div class="flex justify-end gap-3 px-2">
          <el-button @click="showDrawer = false">Cancel</el-button>
          <el-button type="primary" @click="handleSubmit" style="min-width: 100px">
            {{ isEdit ? 'Save Route' : 'Create Route' }}
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

:deep(.el-table__header) th {
  background-color: #f8fafc;
  color: #64748b;
  font-weight: 600;
  text-transform: uppercase;
  font-size: 11px;
  letter-spacing: 0.05em;
}

.w-full\! {
  width: 100% !important;
}

html.dark {
  .bg-slate-50\/50 {
    background-color: #1e293b;
    border-color: #334155;
  }
}
</style>
