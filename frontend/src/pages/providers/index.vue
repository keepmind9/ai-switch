<script setup lang="ts">
import { ref, onMounted, computed } from "vue"
import { ElMessage, ElMessageBox } from "element-plus"
import { Plus, View, Edit, Close, Hide, CopyDocument, Search, Refresh, Link, Delete } from "@element-plus/icons-vue"
import { listProviders, createProvider, updateProvider, deleteProvider, revealAPIKey, type Provider } from "@/api/providers"
import { listPresets, type Preset } from "@/api/stats"

const providers = ref<Provider[]>([])
const presets = ref<Preset[]>([])
const loading = ref(true)
const showDrawer = ref(false)
const isEdit = ref(false)
const selectedPreset = ref("")
const form = ref<any>({})
const revealedKeys = ref<Record<string, string>>({})
const searchQuery = ref("")

const defaultForm = { key: "", name: "", base_url: "", path: "", api_key: "", format: "chat", logo_url: "", sponsor: false, models: [] as string[] }
const modelInput = ref("")

async function load() {
  loading.value = true
  try {
    const [p, pr] = await Promise.all([listProviders(), listPresets()])
    providers.value = p.data.data
    presets.value = pr.data.data
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
  isEdit.value = false; form.value = { ...defaultForm }; selectedPreset.value = ""; showDrawer.value = true
}

function openEdit(row: Provider) {
  isEdit.value = true
  form.value = { 
    key: row.key, 
    name: row.name, 
    base_url: row.base_url, 
    path: row.path, 
    api_key: "", 
    format: row.format, 
    logo_url: row.logo_url, 
    sponsor: row.sponsor, 
    models: [...(row.models || [])] 
  }
  selectedPreset.value = ""; showDrawer.value = true
}

function addModel() {
  const m = modelInput.value.trim()
  if (m && !form.value.models.includes(m)) { form.value.models.push(m); modelInput.value = "" }
}
function removeModel(idx: number) { form.value.models.splice(idx, 1) }

async function handleDelete(key: string) {
  try {
    await ElMessageBox.confirm(`Are you sure you want to delete provider "${key}"? This action cannot be undone.`, "Delete Provider", { 
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      confirmButtonClass: 'el-button--danger',
      type: "warning" 
    })
    await deleteProvider(key)
    ElMessage.success("Provider deleted successfully")
    load()
  } catch (e) {}
}

async function handleSubmit() {
  try {
    if (isEdit.value) { 
      const { key, ...data } = form.value
      await updateProvider(key, data)
      ElMessage.success("Provider updated successfully") 
    } else { 
      await createProvider(form.value)
      ElMessage.success("Provider created successfully") 
    }
    showDrawer.value = false
    load()
  } catch (e) {
    ElMessage.error("Failed to save provider")
  }
}

function handleToggleReveal(row: Provider) {
  if (revealedKeys.value[row.key]) { delete revealedKeys.value[row.key] } else { revealKey(row) }
}

async function revealKey(row: Provider) {
  const resp = await revealAPIKey(row.key); revealedKeys.value[row.key] = resp.data.data.api_key
}

async function handleCopyKey(row: Provider) {
  const key = revealedKeys.value[row.key] || row.api_key
  try { await navigator.clipboard.writeText(key); ElMessage.success("Copied to clipboard") } catch { ElMessage.info(key) }
}

onMounted(load)
</script>

<template>
  <div class="app-container">
    <div class="page-header">
      <div>
        <h3>AI Providers</h3>
        <p class="text-sm text-slate-500 mt-1">Manage upstream LLM services and API connections.</p>
      </div>
      <div class="flex gap-3">
        <el-input v-model="searchQuery" placeholder="Search providers..." :prefix-icon="Search" clearable style="width: 240px" />
        <el-button type="primary" :icon="Plus" @click="openCreate">Add Provider</el-button>
        <el-button :icon="Refresh" circle @click="load" />
      </div>
    </div>

    <el-card shadow="never" class="border-none!">
      <el-table :data="filteredProviders" v-loading="loading" stripe size="large" class="provider-table">
        <el-table-column prop="name" label="Provider" min-width="200">
          <template #default="{ row }">
            <div class="flex items-center gap-3">
              <div class="w-8 h-8 rounded bg-slate-100 flex items-center justify-center overflow-hidden shrink-0">
                <img v-if="row.logo_url" :src="row.logo_url" class="max-w-full max-h-full object-contain" />
                <span v-else class="text-xs font-bold text-slate-400">{{ row.key.slice(0,2).toUpperCase() }}</span>
              </div>
              <div>
                <div class="font-bold text-slate-700">{{ row.name }}</div>
                <div class="text-xs text-slate-400 mono! mt-0.5">{{ row.key }}</div>
              </div>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="base_url" label="Endpoint" min-width="220">
          <template #default="{ row }">
            <div class="flex items-center gap-1 group">
              <span class="text-sm text-slate-500 truncate max-w-200px">{{ row.base_url }}</span>
              <el-link :href="row.base_url" target="_blank" :icon="Link" class="opacity-0 group-hover:opacity-100 transition-opacity" />
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="format" label="Format" width="120">
          <template #default="{ row }">
            <el-tag :type="row.format === 'chat' ? 'primary' : 'warning'" size="small" effect="light" class="capitalize">
              {{ row.format }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="API Key" width="220">
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

        <el-table-column label="Actions" width="120" fixed="right" align="right">
          <template #default="{ row }">
            <div class="flex justify-end gap-1">
              <el-tooltip content="Edit Settings" placement="top">
                <el-button link type="primary" :icon="Edit" @click="openEdit(row)" />
              </el-tooltip>
              <el-tooltip content="Delete Provider" placement="top">
                <el-button link type="danger" :icon="Delete" @click="handleDelete(row.key)" />
              </el-tooltip>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Create/Edit Drawer -->
    <el-drawer
      v-model="showDrawer"
      :title="isEdit ? 'Edit AI Provider' : 'Add New Provider'"
      size="500px"
      destroy-on-close
    >
      <div class="px-2">
        <div v-if="!isEdit" class="mb-8">
          <div class="text-xs font-bold text-slate-400 uppercase tracking-wider mb-3">Quick Setup Presets</div>
          <div class="grid grid-cols-2 gap-2">
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

        <el-form :model="form" label-position="top" class="custom-form">
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Provider ID" required>
                <el-input v-model="form.key" :disabled="isEdit" placeholder="e.g. openai" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Display Name" required>
                <el-input v-model="form.name" placeholder="e.g. OpenAI (Primary)" />
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item label="Base API URL" required>
            <el-input v-model="form.base_url" placeholder="https://api.openai.com/v1" />
          </el-form-item>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item>
                <template #label>
                  <div class="flex items-center gap-1">
                    <span>URL Path Override</span>
                    <span class="text-[10px] text-slate-400 font-normal">(Optional)</span>
                    <el-tooltip content="Custom endpoint path. If empty, the gateway uses the default path based on the protocol format." placement="top">
                      <el-icon :size="12" class="text-slate-400 cursor-help"><QuestionFilled /></el-icon>
                    </el-tooltip>
                  </div>
                </template>
                <el-input v-model="form.path" placeholder="/v1/chat/completions" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="API Key">
                <el-input v-model="form.api_key" type="password" show-password :placeholder="isEdit ? 'Keep empty to remain unchanged' : 'sk-••••••••'" />
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Protocol Format">
                <el-select v-model="form.format" class="w-full">
                  <el-option label="OpenAI Chat" value="chat" />
                  <el-option label="Anthropic" value="anthropic" />
                  <el-option label="Legacy Responses" value="responses" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Provider Type">
                <div class="flex items-center h-40px gap-2">
                  <el-switch v-model="form.sponsor" />
                  <span class="text-sm text-slate-500">Is Partner/Sponsor</span>
                </div>
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item label="Available Models">
            <div class="rounded-lg p-3" style="background-color: var(--v3-section-bg); border: 1px solid var(--el-border-color-light)">
              <div class="flex flex-wrap gap-2 mb-3">
                <el-tag v-for="(m, idx) in form.models" :key="idx" closable @close="removeModel(Number(idx))" size="small" type="info">
                  {{ m }}
                </el-tag>
                <span v-if="!form.models.length" class="text-xs text-slate-400 italic mt-1">No models added yet.</span>
              </div>
              <div class="flex gap-2">
                <el-input v-model="modelInput" placeholder="Add model (e.g. gpt-4o)" size="small" @keyup.enter="addModel" />
                <el-button @click="addModel" size="small" :icon="Plus">Add</el-button>
              </div>
            </div>
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-2">
          <el-button @click="showDrawer = false">Cancel</el-button>
          <el-button type="primary" @click="handleSubmit" style="min-width: 100px">
            {{ isEdit ? 'Save Changes' : 'Create Provider' }}
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

html.dark {
  .preset-card {
    background: #1e293b;
    border-color: #334155;
    color: #f1f5f9;
    
    &:hover, &.active { 
      background: #334155; 
      border-color: #3b82f6; 
      color: #3b82f6;
    }

    :deep(.bg-slate-100) {
      background-color: #0f172a;
    }
  }
}
</style>
