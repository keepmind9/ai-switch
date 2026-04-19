<script setup lang="ts">
import { ref, onMounted } from "vue"
import { ElMessage, ElMessageBox } from "element-plus"
import { Plus, View, Delete, Edit, Close } from "@element-plus/icons-vue"
import { listProviders, createProvider, updateProvider, deleteProvider, revealAPIKey, type Provider } from "@/api/providers"
import { listPresets, type Preset } from "@/api/stats"

const providers = ref<Provider[]>([])
const presets = ref<Preset[]>([])
const loading = ref(true)
const showForm = ref(false)
const isEdit = ref(false)
const selectedPreset = ref("")
const form = ref<any>({})
const mapEditorData = ref<{ key: string; value: string }[]>([])
const revealedKeys = ref<Record<string, string>>({})

const defaultForm = { key: "", name: "", base_url: "", path: "", api_key: "", model: "", format: "chat", model_map: {}, logo_url: "", sponsor: false }

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

function applyPreset(key: string) {
  if (selectedPreset.value === key) { selectedPreset.value = ""; return }
  const p = presets.value.find(x => x.key === key)
  if (p) { form.value.base_url = p.base_url; form.value.format = p.format; form.value.name = p.name; form.value.key = p.key }
  selectedPreset.value = key
}

function openCreate() {
  isEdit.value = false; form.value = { ...defaultForm }; mapEditorData.value = []; selectedPreset.value = ""; showForm.value = true
}

function openEdit(row: Provider) {
  isEdit.value = true
  form.value = { key: row.key, name: row.name, base_url: row.base_url, path: row.path, api_key: "", model: row.model, format: row.format, logo_url: row.logo_url, sponsor: row.sponsor }
  mapEditorData.value = Object.entries(row.model_map || {}).map(([k, v]) => ({ key: k, value: v }))
  selectedPreset.value = ""; showForm.value = true
}

function cancelForm() { showForm.value = false }

async function handleDelete(key: string) {
  await ElMessageBox.confirm(`Delete provider "${key}"?`, "Confirm", { type: "warning" })
  await deleteProvider(key); ElMessage.success("Deleted"); load()
}

async function handleSubmit() {
  const mm: Record<string, string> = {}
  for (const item of mapEditorData.value) { if (item.key) mm[item.key] = item.value }
  form.value.model_map = mm
  if (isEdit.value) { const { key, ...data } = form.value; await updateProvider(key, data); ElMessage.success("Updated") }
  else { await createProvider(form.value); ElMessage.success("Created") }
  showForm.value = false; load()
}

function addMapEntry() { mapEditorData.value.push({ key: "", value: "" }) }
function removeMapEntry(idx: number) { mapEditorData.value.splice(idx, 1) }

async function handleReveal(row: Provider) {
  if (revealedKeys.value[row.key]) {
    try { await navigator.clipboard.writeText(revealedKeys.value[row.key]); ElMessage.success("Copied") } catch { ElMessage.info(revealedKeys.value[row.key]) }
    return
  }
  const resp = await revealAPIKey(row.key); const key = resp.data.data.api_key; revealedKeys.value[row.key] = key
  try { await navigator.clipboard.writeText(key); ElMessage.success("API key copied") } catch { ElMessage.info(key) }
}

function presetTagStyle(p: Preset) {
  const sel = selectedPreset.value === p.key
  return { cursor: "pointer", borderColor: sel ? p.icon_color : "", backgroundColor: sel ? p.icon_color + "22" : "", color: sel ? p.icon_color : "" }
}

onMounted(load)
</script>

<template>
  <div class="app-container">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
      <h3 style="margin: 0">Providers</h3>
      <el-button type="primary" :icon="showForm ? Close : Plus" @click="showForm ? cancelForm() : openCreate()">
        {{ showForm ? "Cancel" : "Add Provider" }}
      </el-button>
    </div>

    <!-- Inline Form -->
    <el-card v-if="showForm" shadow="never" style="margin-bottom: 16px">
      <div v-if="!isEdit" style="margin-bottom: 16px">
        <div style="margin-bottom: 8px; color: var(--el-text-color-secondary); font-size: 13px">Quick setup:</div>
        <el-space wrap>
          <el-check-tag v-for="p in presets" :key="p.key" :checked="selectedPreset === p.key" @change="applyPreset(p.key)" :style="presetTagStyle(p)">
            <span v-if="p.is_partner" style="color: #f59e0b; margin-right: 2px">&#9733;</span>{{ p.name }}
          </el-check-tag>
        </el-space>
      </div>
      <el-form :model="form" label-position="top">
        <el-row :gutter="16">
          <el-col :span="6" v-if="!isEdit"><el-form-item label="Provider Key"><el-input v-model="form.key" placeholder="e.g. minimax" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="Name"><el-input v-model="form.name" /></el-form-item></el-col>
          <el-col :span="isEdit ? 8 : 12"><el-form-item label="Base URL"><el-input v-model="form.base_url" placeholder="https://api.example.com" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="isEdit ? 'API Key (keep empty)' : 'API Key'"><el-input v-model="form.api_key" type="password" show-password /></el-form-item></el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="6"><el-form-item label="Default Model"><el-input v-model="form.model" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="Format"><el-select v-model="form.format" style="width:100%"><el-option label="Chat" value="chat" /><el-option label="Anthropic" value="anthropic" /><el-option label="Responses" value="responses" /></el-select></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="Path Override"><el-input v-model="form.path" placeholder="(optional)" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="Sponsor"><el-switch v-model="form.sponsor" /></el-form-item></el-col>
        </el-row>
        <el-form-item label="Model Map">
          <div style="width:100%">
            <div v-for="(item, idx) in mapEditorData" :key="idx" style="display:flex;gap:8px;margin-bottom:8px">
              <el-input v-model="item.key" placeholder="Client model" style="flex:1" />
              <el-input v-model="item.value" placeholder="Upstream model" style="flex:1" />
              <el-button link type="danger" @click="removeMapEntry(idx)"><el-icon><Delete /></el-icon></el-button>
            </div>
            <el-button size="small" @click="addMapEntry">+ Add Mapping</el-button>
          </div>
        </el-form-item>
        <div style="text-align:right"><el-button @click="cancelForm">Cancel</el-button><el-button type="primary" @click="handleSubmit">{{ isEdit ? "Update" : "Create" }}</el-button></div>
      </el-form>
    </el-card>

    <el-card shadow="never">
      <el-table :data="providers" v-loading="loading" stripe>
        <el-table-column prop="name" label="Name" width="150" />
        <el-table-column prop="key" label="Key" width="120" />
        <el-table-column prop="base_url" label="Base URL" />
        <el-table-column prop="format" label="Format" width="100" />
        <el-table-column prop="model" label="Model" width="150" />
        <el-table-column label="API Key" width="160">
          <template #default="{ row }">
            <div style="display:flex;align-items:center;gap:4px">
              <span style="font-family:monospace;font-size:12px">{{ revealedKeys[row.key] || row.api_key }}</span>
              <el-button link size="small" @click="handleReveal(row)"><el-icon><View /></el-icon></el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Default" width="80"><template #default="{ row }"><el-tag v-if="row.is_default" type="success" size="small">Default</el-tag></template></el-table-column>
        <el-table-column label="Actions" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" :icon="Edit" @click="openEdit(row)">Edit</el-button>
            <el-button link type="danger" size="small" @click="handleDelete(row.key)">Delete</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>
