<script setup lang="ts">
import { ref, onMounted } from "vue"
import { ElMessage, ElMessageBox } from "element-plus"
import { Plus, Delete, Edit, Close } from "@element-plus/icons-vue"
import { listRoutes, createRoute, updateRoute, deleteRoute, generateKey, type Route } from "@/api/routes"
import { listProviders, type Provider } from "@/api/providers"

const routes = ref<Route[]>([])
const providers = ref<Provider[]>([])
const loading = ref(true)
const showForm = ref(false)
const isEdit = ref(false)
const editKey = ref("")
const form = ref<any>({})
const sceneMapData = ref<{ key: string; value: string }[]>([])
const modelMapData = ref<{ key: string; value: string }[]>([])
const sceneOptions = ["default", "think", "background", "websearch"]

async function load() {
  loading.value = true
  try { const [r, p] = await Promise.all([listRoutes(), listProviders()]); routes.value = r.data.data; providers.value = p.data.data }
  finally { loading.value = false }
}

function openCreate() {
  isEdit.value = false; editKey.value = ""
  form.value = { key: "", provider: "", default_model: "", scene_map: {}, model_map: {} }
  sceneMapData.value = []; modelMapData.value = []; showForm.value = true
}

function openEdit(row: Route) {
  isEdit.value = true; editKey.value = row.key
  form.value = { key: row.key, provider: row.provider, default_model: row.default_model }
  sceneMapData.value = Object.entries(row.scene_map || {}).map(([k, v]) => ({ key: k, value: v }))
  modelMapData.value = Object.entries(row.model_map || {}).map(([k, v]) => ({ key: k, value: v }))
  showForm.value = true
}

function cancelForm() { showForm.value = false }

async function handleGenerateKey() { const { data } = await generateKey(); form.value.key = data.data.key }

async function handleDelete(key: string) {
  await ElMessageBox.confirm("Delete route?", "Confirm", { type: "warning" })
  await deleteRoute(key); ElMessage.success("Deleted"); load()
}

async function handleSubmit() {
  const sm: Record<string, string> = {}; for (const item of sceneMapData.value) { if (item.key) sm[item.key] = item.value }
  const mm: Record<string, string> = {}; for (const item of modelMapData.value) { if (item.key) mm[item.key] = item.value }
  form.value.scene_map = sm; form.value.model_map = mm
  if (isEdit.value) { const data = { ...form.value }; delete data.key; await updateRoute(editKey.value, data); ElMessage.success("Updated") }
  else { await createRoute(form.value); ElMessage.success("Created") }
  showForm.value = false; load()
}

function addSceneEntry() { sceneMapData.value.push({ key: "", value: "" }) }
function removeSceneEntry(idx: number) { sceneMapData.value.splice(idx, 1) }
function addModelEntry() { modelMapData.value.push({ key: "", value: "" }) }
function removeModelEntry(idx: number) { modelMapData.value.splice(idx, 1) }

onMounted(load)
</script>

<template>
  <div class="app-container">
    <div class="page-header">
      <h3>Routes</h3>
      <el-button type="primary" :icon="showForm ? Close : Plus" @click="showForm ? cancelForm() : openCreate()">
        {{ showForm ? "Cancel" : "Add Route" }}
      </el-button>
    </div>

    <!-- Form -->
    <el-card v-if="showForm" shadow="never" class="form-card">
      <el-form :model="form" label-position="top">
        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item label="Gateway Key">
              <div class="key-input-row">
                <el-input v-model="form.key" :disabled="isEdit" placeholder="Gateway API key" />
                <el-button v-if="!isEdit" @click="handleGenerateKey">Generate</el-button>
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="Provider">
              <el-select v-model="form.provider" placeholder="Select provider" class="w-full">
                <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="Default Model"><el-input v-model="form.default_model" /></el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="Scene Map">
          <div class="map-editor">
            <div v-for="(item, idx) in sceneMapData" :key="idx" class="map-editor-row">
              <el-select v-model="item.key" placeholder="Scene" class="scene-select">
                <el-option v-for="s in sceneOptions" :key="s" :label="s" :value="s" />
              </el-select>
              <el-input v-model="item.value" placeholder="Model name" class="map-input" />
              <el-button link type="danger" @click="removeSceneEntry(idx)"><el-icon><Delete /></el-icon></el-button>
            </div>
            <el-button size="small" @click="addSceneEntry">+ Add Scene</el-button>
          </div>
        </el-form-item>
        <el-form-item label="Model Map">
          <div class="map-editor">
            <div v-for="(item, idx) in modelMapData" :key="idx" class="map-editor-row">
              <el-input v-model="item.key" placeholder="Client model" class="map-input" />
              <el-input v-model="item.value" placeholder="Upstream model" class="map-input" />
              <el-button link type="danger" @click="removeModelEntry(idx)"><el-icon><Delete /></el-icon></el-button>
            </div>
            <el-button size="small" @click="addModelEntry">+ Add Model Mapping</el-button>
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
      <el-table :data="routes" v-loading="loading" stripe>
        <el-table-column label="Gateway Key" width="200">
          <template #default="{ row }"><span class="mono">{{ row.key }}</span></template>
        </el-table-column>
        <el-table-column prop="provider" label="Provider" width="140" />
        <el-table-column prop="default_model" label="Default Model" width="180" />
        <el-table-column label="Scene Map" min-width="200">
          <template #default="{ row }">
            <el-tag v-for="(v, k) in row.scene_map" :key="k" size="small" class="tag-spacing">{{ k }}: {{ v }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Model Map" min-width="200">
          <template #default="{ row }">
            <el-tag v-for="(v, k) in row.model_map" :key="k" type="info" size="small" class="tag-spacing">{{ k }}: {{ v }}</el-tag>
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

.key-input-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.map-editor {
  width: 100%;
}

.map-editor-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
  align-items: center;
}

.scene-select {
  width: 150px;
  flex-shrink: 0;
}

.map-input {
  flex: 1;
}

.w-full {
  width: 100%;
}

.tag-spacing {
  margin-right: 4px;
  margin-bottom: 2px;
}
</style>
