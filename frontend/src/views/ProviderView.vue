<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  listProviders,
  createProvider,
  updateProvider,
  deleteProvider,
  revealAPIKey,
  type Provider,
} from '../api/providers'
import { listPresets, type Preset } from '../api/stats'

const providers = ref<Provider[]>([])
const presets = ref<Preset[]>([])
const loading = ref(true)
const dialogVisible = ref(false)
const isEdit = ref(false)
const selectedPreset = ref('')

const form = ref<any>({})
const defaultForm = {
  key: '',
  name: '',
  base_url: '',
  path: '',
  api_key: '',
  model: '',
  format: 'chat',
  model_map: {},
  logo_url: '',
  sponsor: false,
}

const mapEditorData = ref<{ key: string; value: string }[]>([])

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
  const p = presets.value.find((x) => x.key === key)
  if (p) {
    form.value.base_url = p.base_url
    form.value.format = p.format
    form.value.name = p.name
    form.value.logo_url = p.logo_url
  }
  selectedPreset.value = key
}

function openCreate() {
  isEdit.value = false
  form.value = { ...defaultForm }
  mapEditorData.value = []
  selectedPreset.value = ''
  dialogVisible.value = true
}

function openEdit(row: Provider) {
  isEdit.value = true
  form.value = {
    key: row.key,
    name: row.name,
    base_url: row.base_url,
    path: row.path,
    api_key: '',
    model: row.model,
    format: row.format,
    model_map: { ...row.model_map },
    logo_url: row.logo_url,
    sponsor: row.sponsor,
  }
  mapEditorData.value = Object.entries(row.model_map || {}).map(([k, v]) => ({ key: k, value: v }))
  dialogVisible.value = true
}

async function handleDelete(key: string) {
  await ElMessageBox.confirm(`Delete provider "${key}"?`, 'Confirm', { type: 'warning' })
  await deleteProvider(key)
  ElMessage.success('Deleted')
  load()
}

async function handleSubmit() {
  const mm: Record<string, string> = {}
  for (const item of mapEditorData.value) {
    if (item.key) mm[item.key] = item.value
  }
  form.value.model_map = mm

  if (isEdit.value) {
    const { key, ...data } = form.value
    await updateProvider(key, data)
    ElMessage.success('Updated')
  } else {
    await createProvider(form.value)
    ElMessage.success('Created')
  }
  dialogVisible.value = false
  load()
}

function addMapEntry() {
  mapEditorData.value.push({ key: '', value: '' })
}

function removeMapEntry(idx: number) {
  mapEditorData.value.splice(idx, 1)
}

async function handleReveal(row: Provider) {
  const { data } = (await revealAPIKey(row.key)).data
  try {
    await navigator.clipboard.writeText(data.api_key)
    ElMessage.success('API key copied to clipboard')
  } catch {
    ElMessage.info(data.api_key)
  }
}

onMounted(load)
</script>

<template>
  <div class="page-header" style="display: flex; justify-content: space-between; align-items: center">
    <h2>Providers</h2>
    <el-button type="primary" :icon="Plus" @click="openCreate">Add Provider</el-button>
  </div>

  <div class="card">
    <el-table :data="providers" v-loading="loading" stripe>
      <el-table-column prop="name" label="Name" width="150" />
      <el-table-column prop="key" label="Key" width="120" />
      <el-table-column prop="base_url" label="Base URL" />
      <el-table-column prop="format" label="Format" width="100" />
      <el-table-column prop="model" label="Default Model" width="150" />
      <el-table-column label="API Key" width="160">
        <template #default="{ row }">
          <div style="display: flex; align-items: center; gap: 4px">
            <span style="font-family: monospace; font-size: 12px">{{ row.api_key }}</span>
            <el-button link size="small" @click="handleReveal(row)" title="Reveal and copy">
              <el-icon><View /></el-icon>
            </el-button>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Default" width="80">
        <template #default="{ row }">
          <el-tag v-if="row.is_default" type="success" size="small">Default</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="150" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="openEdit(row)">Edit</el-button>
          <el-button link type="danger" size="small" @click="handleDelete(row.key)">Delete</el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>

  <el-dialog v-model="dialogVisible" :title="isEdit ? 'Edit Provider' : 'Add Provider'" width="600px" destroy-on-close>
    <!-- Preset tags -->
    <div v-if="!isEdit" style="margin-bottom: 16px">
      <div style="margin-bottom: 8px; color: var(--text-secondary); font-size: 13px">Quick setup:</div>
      <el-check-tag
        v-for="p in presets"
        :key="p.key"
        :checked="selectedPreset === p.key"
        @change="applyPreset(p.key)"
        style="margin-right: 8px; margin-bottom: 4px"
      >
        {{ p.name }}
      </el-check-tag>
    </div>

    <el-form :model="form" label-width="120px" label-position="top">
      <el-form-item label="Provider Key" v-if="!isEdit">
        <el-input v-model="form.key" placeholder="e.g. minimax" />
      </el-form-item>
      <el-form-item label="Name">
        <el-input v-model="form.name" />
      </el-form-item>
      <el-form-item label="Base URL">
        <el-input v-model="form.base_url" placeholder="https://api.example.com" />
      </el-form-item>
      <el-form-item label="Path Override (optional)">
        <el-input v-model="form.path" placeholder="/custom/path" />
      </el-form-item>
      <el-form-item :label="isEdit ? 'API Key (leave empty to keep)' : 'API Key'">
        <el-input v-model="form.api_key" type="password" show-password />
      </el-form-item>
      <el-form-item label="Default Model">
        <el-input v-model="form.model" />
      </el-form-item>
      <el-form-item label="Format">
        <el-select v-model="form.format">
          <el-option label="Chat" value="chat" />
          <el-option label="Anthropic" value="anthropic" />
          <el-option label="Responses" value="responses" />
        </el-select>
      </el-form-item>
      <el-form-item label="Logo URL">
        <el-input v-model="form.logo_url" />
      </el-form-item>
      <el-form-item label="Sponsor">
        <el-switch v-model="form.sponsor" />
      </el-form-item>

      <el-divider>Model Map</el-divider>
      <div v-for="(item, idx) in mapEditorData" :key="idx" style="display: flex; gap: 8px; margin-bottom: 8px">
        <el-input v-model="item.key" placeholder="Client model" style="flex: 1" />
        <el-input v-model="item.value" placeholder="Upstream model" style="flex: 1" />
        <el-button link type="danger" @click="removeMapEntry(idx)">
          <el-icon><Delete /></el-icon>
        </el-button>
      </div>
      <el-button size="small" @click="addMapEntry">+ Add Mapping</el-button>
    </el-form>

    <template #footer>
      <el-button @click="dialogVisible = false">Cancel</el-button>
      <el-button type="primary" @click="handleSubmit">{{ isEdit ? 'Update' : 'Create' }}</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts">
import { View, Delete } from '@element-plus/icons-vue'
export default { components: { View, Delete } }
</script>
