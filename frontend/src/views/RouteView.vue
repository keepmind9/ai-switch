<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  listRoutes,
  createRoute,
  updateRoute,
  deleteRoute,
  generateKey,
  revealRouteKey,
  type Route,
} from '../api/routes'
import { listProviders, type Provider } from '../api/providers'

const routes = ref<Route[]>([])
const providers = ref<Provider[]>([])
const loading = ref(true)
const dialogVisible = ref(false)
const isEdit = ref(false)
const editKey = ref('')
const revealedKeys = ref<Record<string, string>>({})

const form = ref<any>({})
const sceneMapData = ref<{ key: string; value: string }[]>([])
const modelMapData = ref<{ key: string; value: string }[]>([])

const sceneOptions = ['default', 'think', 'background', 'websearch']

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
  isEdit.value = false
  editKey.value = ''
  form.value = { key: '', provider: '', default_model: '', scene_map: {}, model_map: {} }
  sceneMapData.value = []
  modelMapData.value = []
  dialogVisible.value = true
}

function openEdit(row: Route) {
  isEdit.value = true
  editKey.value = row.key
  form.value = {
    key: row.key,
    provider: row.provider,
    default_model: row.default_model,
  }
  sceneMapData.value = Object.entries(row.scene_map || {}).map(([k, v]) => ({ key: k, value: v }))
  modelMapData.value = Object.entries(row.model_map || {}).map(([k, v]) => ({ key: k, value: v }))
  dialogVisible.value = true
}

async function handleGenerateKey() {
  const { data } = await generateKey()
  form.value.key = data.data.key
}

async function handleDelete(key: string) {
  await ElMessageBox.confirm(`Delete route?`, 'Confirm', { type: 'warning' })
  await deleteRoute(key)
  ElMessage.success('Deleted')
  load()
}

async function handleSubmit() {
  const sm: Record<string, string> = {}
  for (const item of sceneMapData.value) {
    if (item.key) sm[item.key] = item.value
  }
  const mm: Record<string, string> = {}
  for (const item of modelMapData.value) {
    if (item.key) mm[item.key] = item.value
  }
  form.value.scene_map = sm
  form.value.model_map = mm

  if (isEdit.value) {
    const data = { ...form.value }
    delete data.key
    await updateRoute(editKey.value, data)
    ElMessage.success('Updated')
  } else {
    await createRoute(form.value)
    ElMessage.success('Created')
  }
  dialogVisible.value = false
  load()
}

async function handleReveal(row: Route) {
  if (revealedKeys.value[row.key]) {
    try {
      await navigator.clipboard.writeText(revealedKeys.value[row.key])
      ElMessage.success('Key copied to clipboard')
    } catch {
      ElMessage.info(revealedKeys.value[row.key])
    }
    return
  }
  // For routes, the key is already the identifier but it's masked in the list
  // We need the actual key to reveal - use the row's masked key won't work
  // Instead, store the key when editing and use it for reveal
  ElMessage.info('Key: ' + row.key)
}

function addSceneEntry() {
  sceneMapData.value.push({ key: '', value: '' })
}

function removeSceneEntry(idx: number) {
  sceneMapData.value.splice(idx, 1)
}

function addModelEntry() {
  modelMapData.value.push({ key: '', value: '' })
}

function removeModelEntry(idx: number) {
  modelMapData.value.splice(idx, 1)
}

onMounted(load)
</script>

<template>
  <div class="page-header" style="display: flex; justify-content: space-between; align-items: center">
    <h2>Routes</h2>
    <el-button type="primary" :icon="Plus" @click="openCreate">Add Route</el-button>
  </div>

  <div class="card">
    <el-table :data="routes" v-loading="loading" stripe>
      <el-table-column label="Gateway Key" width="180">
        <template #default="{ row }">
          <span style="font-family: monospace; font-size: 12px">{{ row.key }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="provider" label="Provider" width="140" />
      <el-table-column prop="default_model" label="Default Model" width="180" />
      <el-table-column label="Scene Map" min-width="200">
        <template #default="{ row }">
          <el-tag v-for="(v, k) in row.scene_map" :key="k" size="small" style="margin-right: 4px">
            {{ k }}: {{ v }}
          </el-tag>
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

  <el-dialog v-model="dialogVisible" :title="isEdit ? 'Edit Route' : 'Add Route'" width="600px" destroy-on-close>
    <el-form :model="form" label-position="top">
      <el-form-item label="Gateway Key">
        <div style="display: flex; gap: 8px">
          <el-input v-model="form.key" :disabled="isEdit" placeholder="Gateway API key" style="flex: 1" />
          <el-button v-if="!isEdit" @click="handleGenerateKey">Generate</el-button>
        </div>
      </el-form-item>
      <el-form-item label="Provider">
        <el-select v-model="form.provider" placeholder="Select provider" style="width: 100%">
          <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
        </el-select>
      </el-form-item>
      <el-form-item label="Default Model">
        <el-input v-model="form.default_model" />
      </el-form-item>

      <el-divider>Scene Map</el-divider>
      <div v-for="(item, idx) in sceneMapData" :key="idx" style="display: flex; gap: 8px; margin-bottom: 8px">
        <el-select v-model="item.key" placeholder="Scene" style="width: 150px">
          <el-option v-for="s in sceneOptions" :key="s" :label="s" :value="s" />
        </el-select>
        <el-input v-model="item.value" placeholder="Model name" style="flex: 1" />
        <el-button link type="danger" @click="removeSceneEntry(idx)">
          <el-icon><Delete /></el-icon>
        </el-button>
      </div>
      <el-button size="small" @click="addSceneEntry">+ Add Scene</el-button>

      <el-divider>Model Map</el-divider>
      <div v-for="(item, idx) in modelMapData" :key="idx" style="display: flex; gap: 8px; margin-bottom: 8px">
        <el-input v-model="item.key" placeholder="Client model" style="flex: 1" />
        <el-input v-model="item.value" placeholder="Upstream model" style="flex: 1" />
        <el-button link type="danger" @click="removeModelEntry(idx)">
          <el-icon><Delete /></el-icon>
        </el-button>
      </div>
      <el-button size="small" @click="addModelEntry">+ Add Model Mapping</el-button>
    </el-form>

    <template #footer>
      <el-button @click="dialogVisible = false">Cancel</el-button>
      <el-button type="primary" @click="handleSubmit">{{ isEdit ? 'Update' : 'Create' }}</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts">
import { Delete } from '@element-plus/icons-vue'
export default { components: { Delete } }
</script>
