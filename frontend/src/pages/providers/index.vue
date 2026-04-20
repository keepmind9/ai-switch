<script setup lang="ts">
import { ref, onMounted } from "vue"
import { ElMessage, ElMessageBox } from "element-plus"
import { Plus, View, Edit, Close, Hide, CopyDocument } from "@element-plus/icons-vue"
import { listProviders, createProvider, updateProvider, deleteProvider, revealAPIKey, type Provider } from "@/api/providers"
import { listPresets, type Preset } from "@/api/stats"

const providers = ref<Provider[]>([])
const presets = ref<Preset[]>([])
const loading = ref(true)
const showForm = ref(false)
const isEdit = ref(false)
const selectedPreset = ref("")
const form = ref<any>({})
const revealedKeys = ref<Record<string, string>>({})

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

function applyPreset(key: string) {
  if (selectedPreset.value === key) { selectedPreset.value = ""; return }
  const p = presets.value.find(x => x.key === key)
  if (p) { form.value.base_url = p.base_url; form.value.format = p.format; form.value.name = p.name; form.value.key = p.key }
  selectedPreset.value = key
}

function openCreate() {
  isEdit.value = false; form.value = { ...defaultForm }; selectedPreset.value = ""; showForm.value = true
}

function openEdit(row: Provider) {
  isEdit.value = true
  form.value = { key: row.key, name: row.name, base_url: row.base_url, path: row.path, api_key: "", format: row.format, logo_url: row.logo_url, sponsor: row.sponsor, models: [...(row.models || [])] }
  selectedPreset.value = ""; showForm.value = true
}

function cancelForm() { showForm.value = false }

function addModel() {
  const m = modelInput.value.trim()
  if (m && !form.value.models.includes(m)) { form.value.models.push(m); modelInput.value = "" }
}
function removeModel(idx: number) { form.value.models.splice(idx, 1) }

async function handleDelete(key: string) {
  await ElMessageBox.confirm(`Delete provider "${key}"?`, "Confirm", { type: "warning" })
  await deleteProvider(key); ElMessage.success("Deleted"); load()
}

async function handleSubmit() {
  if (isEdit.value) { const { key, ...data } = form.value; await updateProvider(key, data); ElMessage.success("Updated") }
  else { await createProvider(form.value); ElMessage.success("Created") }
  showForm.value = false; load()
}

function handleToggleReveal(row: Provider) {
  if (revealedKeys.value[row.key]) { delete revealedKeys.value[row.key] } else { revealKey(row) }
}

async function revealKey(row: Provider) {
  const resp = await revealAPIKey(row.key); revealedKeys.value[row.key] = resp.data.data.api_key
}

async function handleCopyKey(row: Provider) {
  const key = revealedKeys.value[row.key] || row.api_key
  try { await navigator.clipboard.writeText(key); ElMessage.success("Copied") } catch { ElMessage.info(key) }
}

function presetTagStyle(p: Preset) {
  const sel = selectedPreset.value === p.key
  return { cursor: "pointer", borderColor: sel ? p.icon_color : "", backgroundColor: sel ? p.icon_color + "22" : "", color: sel ? p.icon_color : "" }
}

onMounted(load)
</script>

<template>
  <div class="app-container">
    <div class="page-header">
      <h3>Providers</h3>
      <el-button type="primary" :icon="showForm ? Close : Plus" @click="showForm ? cancelForm() : openCreate()">
        {{ showForm ? "Cancel" : "Add Provider" }}
      </el-button>
    </div>

    <!-- Form -->
    <el-card v-if="showForm" shadow="never" class="form-card">
      <div v-if="!isEdit" class="preset-section">
        <div class="preset-label">Quick setup:</div>
        <el-space wrap :size="8">
          <el-check-tag
            v-for="p in presets"
            :key="p.key"
            :checked="selectedPreset === p.key"
            @change="applyPreset(p.key)"
            :style="presetTagStyle(p)"
          >
            <span v-if="p.is_partner" class="partner-star">&#9733;</span>{{ p.name }}
          </el-check-tag>
        </el-space>
      </div>
      <el-form :model="form" label-position="top">
        <el-row :gutter="16">
          <el-col :span="6" v-if="!isEdit">
            <el-form-item label="Provider Key">
              <el-input v-model="form.key" placeholder="e.g. minimax" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="Name"><el-input v-model="form.name" /></el-form-item>
          </el-col>
          <el-col :span="isEdit ? 8 : 12">
            <el-form-item label="Base URL"><el-input v-model="form.base_url" placeholder="https://api.example.com" /></el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item :label="isEdit ? 'API Key (keep empty)' : 'API Key'">
              <el-input v-model="form.api_key" type="password" show-password />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="6">
            <el-form-item label="Format">
              <el-select v-model="form.format" class="w-full">
                <el-option label="Chat" value="chat" />
                <el-option label="Anthropic" value="anthropic" />
                <el-option label="Responses" value="responses" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="Path Override"><el-input v-model="form.path" placeholder="(optional)" /></el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="Sponsor"><el-switch v-model="form.sponsor" /></el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="Models">
          <div class="models-editor">
            <div class="models-tags">
              <el-tag v-for="(m, idx) in form.models" :key="idx" closable @close="removeModel(Number(idx))" class="model-tag">{{ m }}</el-tag>
            </div>
            <div class="models-input-row">
              <el-input v-model="modelInput" placeholder="Add model name" @keyup.enter="addModel" />
              <el-button @click="addModel">Add</el-button>
            </div>
          </div>
        </el-form-item>
        <div class="form-actions">
          <el-button @click="cancelForm">Cancel</el-button>
          <el-button type="primary" @click="handleSubmit">{{ isEdit ? "Update" : "Create" }}</el-button>
        </div>
      </el-form>
    </el-card>

    <!-- Table -->
    <el-card shadow="never">
      <el-table :data="providers" v-loading="loading" stripe>
        <el-table-column prop="name" label="Name" width="150" />
        <el-table-column prop="key" label="Key" width="120" />
        <el-table-column prop="base_url" label="Base URL" />
        <el-table-column prop="format" label="Format" width="100" />
        <el-table-column label="Models" min-width="200">
          <template #default="{ row }">
            <el-tag v-for="m in (row.models || [])" :key="m" size="small" class="tag-spacing">{{ m }}</el-tag>
            <span v-if="!row.models?.length" class="text-muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="API Key" width="200">
          <template #default="{ row }">
            <div class="api-key-cell">
              <span class="mono api-key-text" :class="{ 'api-key-truncated': !revealedKeys[row.key] }">{{ revealedKeys[row.key] || row.api_key }}</span>
              <el-button link size="small" @click="handleToggleReveal(row)"><el-icon><component :is="revealedKeys[row.key] ? Hide : View" /></el-icon></el-button>
              <el-button v-if="revealedKeys[row.key]" link size="small" @click="handleCopyKey(row)"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
          </template>
        </el-table-column>
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

<style lang="scss" scoped>
.form-card {
  margin-bottom: 16px;
}

.preset-section {
  margin-bottom: 20px;
  .preset-label {
    font-size: 13px;
    color: var(--el-text-color-secondary);
    margin-bottom: 10px;
  }
}

.w-full {
  width: 100%;
}

.api-key-cell {
  display: flex;
  align-items: center;
  gap: 4px;
}

.api-key-text {
  min-width: 0;
  word-break: break-all;
}

.api-key-truncated {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 120px;
}

.partner-star {
  color: #f59e0b;
  margin-right: 2px;
}

.models-editor {
  width: 100%;
}

.models-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 8px;
}

.models-input-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.model-tag {
  margin-right: 0;
}

.tag-spacing {
  margin-right: 4px;
  margin-bottom: 2px;
}

.text-muted {
  color: var(--el-text-color-placeholder);
}
</style>
