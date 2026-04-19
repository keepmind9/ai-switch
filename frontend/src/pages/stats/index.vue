<script setup lang="ts">
import { ref, onMounted, computed } from "vue"
import { queryStats, type UsageRecord } from "@/api/stats"
import { listProviders, type Provider } from "@/api/providers"
import { use } from "echarts/core"
import { CanvasRenderer } from "echarts/renderers"
import { BarChart } from "echarts/charts"
import { GridComponent, TooltipComponent, LegendComponent } from "echarts/components"
import VChart from "vue-echarts"

use([CanvasRenderer, BarChart, GridComponent, TooltipComponent, LegendComponent])

const loading = ref(true)
const records = ref<UsageRecord[]>([])
const providers = ref<Provider[]>([])
const dateRange = ref<[string, string]>(getDefaultRange())
const filterProvider = ref("")
const filterModel = ref("")

function getDefaultRange(): [string, string] {
  const end = new Date()
  const start = new Date()
  start.setDate(start.getDate() - 14)
  return [fmt(start), fmt(end)]
}

function fmt(d: Date): string {
  return d.toISOString().slice(0, 10)
}

const modelOptions = computed(() => {
  const models = new Set<string>()
  for (const r of records.value) {
    if (!filterProvider.value || r.provider === filterProvider.value) models.add(r.model)
  }
  return [...models].sort()
})

const filtered = computed(() => {
  let data = records.value
  if (filterProvider.value) data = data.filter(r => r.provider === filterProvider.value)
  if (filterModel.value) data = data.filter(r => r.model === filterModel.value)
  return data
})

const summary = computed(() => {
  const d = filtered.value
  return {
    requests: d.reduce((s, r) => s + r.requests, 0),
    input_tokens: d.reduce((s, r) => s + r.input_tokens, 0),
    output_tokens: d.reduce((s, r) => s + r.output_tokens, 0),
    total_tokens: d.reduce((s, r) => s + r.total_tokens, 0),
  }
})

const chartOption = computed(() => {
  const daily: Record<string, { input: number; output: number }> = {}
  for (const r of filtered.value) {
    if (!daily[r.date]) daily[r.date] = { input: 0, output: 0 }
    daily[r.date].input += r.input_tokens
    daily[r.date].output += r.output_tokens
  }
  const dates = Object.keys(daily).sort()
  return {
    tooltip: { trigger: "axis" },
    legend: { data: ["Input Tokens", "Output Tokens"] },
    grid: { left: 60, right: 20, top: 40, bottom: 30 },
    xAxis: { type: "category", data: dates },
    yAxis: { type: "value" },
    series: [
      { name: "Input Tokens", type: "bar", stack: "tokens", data: dates.map(d => daily[d].input) },
      { name: "Output Tokens", type: "bar", stack: "tokens", data: dates.map(d => daily[d].output) },
    ],
  }
})

async function load() {
  loading.value = true
  try {
    const [r, p] = await Promise.all([
      queryStats({ start_date: dateRange.value[0], end_date: dateRange.value[1] }),
      listProviders(),
    ])
    records.value = r.data.data
    providers.value = p.data.data
  } finally {
    loading.value = false
  }
}

function handleSearch() { load() }

onMounted(load)
</script>

<template>
  <div class="app-container">
    <el-card shadow="never" style="margin-bottom: 16px">
      <el-row :gutter="16" align="middle">
        <el-col :span="10">
          <el-date-picker v-model="dateRange" type="daterange" range-separator="—" start-placeholder="Start" end-placeholder="End" value-format="YYYY-MM-DD" style="width: 100%" />
        </el-col>
        <el-col :span="5">
          <el-select v-model="filterProvider" placeholder="All Providers" clearable style="width: 100%">
            <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
          </el-select>
        </el-col>
        <el-col :span="5">
          <el-select v-model="filterModel" placeholder="All Models" clearable style="width: 100%">
            <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
          </el-select>
        </el-col>
        <el-col :span="4">
          <el-button type="primary" @click="handleSearch">Search</el-button>
        </el-col>
      </el-row>
    </el-card>

    <el-row :gutter="16" style="margin-bottom: 16px">
      <el-col :span="6">
        <el-card shadow="never">
          <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Total Requests</span></template>
          <div style="font-size: 24px; font-weight: 600; color: var(--el-color-primary)">{{ summary.requests.toLocaleString() }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never">
          <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Total Tokens</span></template>
          <div style="font-size: 24px; font-weight: 600; color: var(--el-color-primary)">{{ summary.total_tokens.toLocaleString() }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never">
          <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Input Tokens</span></template>
          <div style="font-size: 24px; font-weight: 600">{{ summary.input_tokens.toLocaleString() }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never">
          <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Output Tokens</span></template>
          <div style="font-size: 24px; font-weight: 600">{{ summary.output_tokens.toLocaleString() }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" style="margin-bottom: 16px">
      <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Daily Token Usage</span></template>
      <v-chart :option="chartOption" style="height: 350px" autoresize />
    </el-card>

    <el-card shadow="never">
      <template #header><span style="font-size: 13px; color: var(--el-text-color-secondary)">Usage Records</span></template>
      <el-table :data="filtered" v-loading="loading" stripe max-height="400">
        <el-table-column prop="date" label="Date" width="120" />
        <el-table-column prop="provider" label="Provider" width="120" />
        <el-table-column prop="model" label="Model" width="200" />
        <el-table-column prop="requests" label="Requests" width="100">
          <template #default="{ row }">{{ row.requests.toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="input_tokens" label="Input" width="120">
          <template #default="{ row }">{{ row.input_tokens.toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="output_tokens" label="Output" width="120">
          <template #default="{ row }">{{ row.output_tokens.toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="total_tokens" label="Total" width="120">
          <template #default="{ row }">{{ row.total_tokens.toLocaleString() }}</template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>
