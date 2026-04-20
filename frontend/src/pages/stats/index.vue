<script setup lang="ts">
import { ref, onMounted, computed } from "vue"
import { queryStats, type UsageRecord } from "@/api/stats"
import { listProviders, type Provider } from "@/api/providers"
import { TrendCharts } from "@element-plus/icons-vue"
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
    tooltip: {
      trigger: "axis",
      backgroundColor: "rgba(0, 0, 0, 0.8)",
      borderWidth: 0,
      textStyle: { color: "#fff" },
    },
    legend: {
      data: ["Input Tokens", "Output Tokens"],
      bottom: 0,
      icon: "roundRect",
      itemWidth: 12,
      itemHeight: 8,
    },
    grid: { left: 60, right: 20, top: 20, bottom: 40 },
    xAxis: {
      type: "category",
      data: dates,
      axisLine: { lineStyle: { color: "#dcdfe6" } },
      axisLabel: { color: "#909399", fontSize: 11 },
    },
    yAxis: {
      type: "value",
      splitLine: { lineStyle: { color: "#e4e7ed" } },
      axisLabel: { color: "#909399", fontSize: 11 },
    },
    series: [
      {
        name: "Input Tokens",
        type: "bar",
        stack: "tokens",
        barMaxWidth: 24,
        itemStyle: { borderRadius: [0, 0, 0, 0] },
        color: "#409eff",
        data: dates.map(d => daily[d].input),
      },
      {
        name: "Output Tokens",
        type: "bar",
        stack: "tokens",
        barMaxWidth: 24,
        itemStyle: { borderRadius: [4, 4, 0, 0] },
        color: "#67c23a",
        data: dates.map(d => daily[d].output),
      },
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
    <!-- Filters -->
    <el-card shadow="never" class="filter-card">
      <el-row :gutter="16" align="middle">
        <el-col :span="10">
          <el-date-picker
            v-model="dateRange"
            type="daterange"
            range-separator="—"
            start-placeholder="Start"
            end-placeholder="End"
            value-format="YYYY-MM-DD"
            class="w-full"
          />
        </el-col>
        <el-col :span="5">
          <el-select v-model="filterProvider" placeholder="All Providers" clearable class="w-full">
            <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
          </el-select>
        </el-col>
        <el-col :span="5">
          <el-select v-model="filterModel" placeholder="All Models" clearable class="w-full">
            <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
          </el-select>
        </el-col>
        <el-col :span="4">
          <el-button type="primary" @click="handleSearch">Search</el-button>
        </el-col>
      </el-row>
    </el-card>

    <!-- Summary Cards -->
    <el-row :gutter="16" class="stat-row">
      <el-col :span="6">
        <el-card shadow="never" class="stat-card">
          <div class="stat-icon stat-icon--primary">
            <el-icon :size="22"><TrendCharts /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Total Requests</div>
            <div class="stat-value">{{ summary.requests.toLocaleString() }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never" class="stat-card">
          <div class="stat-icon stat-icon--success">
            <el-icon :size="22"><TrendCharts /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Total Tokens</div>
            <div class="stat-value stat-value--primary">{{ summary.total_tokens.toLocaleString() }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never" class="stat-card">
          <div class="stat-icon">
            <el-icon :size="22"><TrendCharts /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Input Tokens</div>
            <div class="stat-value">{{ summary.input_tokens.toLocaleString() }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never" class="stat-card">
          <div class="stat-icon">
            <el-icon :size="22"><TrendCharts /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">Output Tokens</div>
            <div class="stat-value">{{ summary.output_tokens.toLocaleString() }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- Chart -->
    <el-card shadow="never" class="section-card">
      <template #header>
        <div class="card-header-label">Daily Token Usage</div>
      </template>
      <v-chart :option="chartOption" class="chart" autoresize />
    </el-card>

    <!-- Table -->
    <el-card shadow="never" class="section-card">
      <template #header>
        <div class="card-header-label">Usage Records</div>
      </template>
      <el-table :data="filtered" v-loading="loading" stripe max-height="500">
        <el-table-column prop="date" label="Date" width="120" />
        <el-table-column prop="provider" label="Provider" min-width="160" />
        <el-table-column prop="model" label="Model" min-width="180" />
        <el-table-column prop="requests" label="Requests" width="110" align="right">
          <template #default="{ row }">{{ row.requests.toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="input_tokens" label="Input Tokens" width="130" align="right">
          <template #default="{ row }">{{ row.input_tokens.toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="output_tokens" label="Output Tokens" width="130" align="right">
          <template #default="{ row }">{{ row.output_tokens.toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="total_tokens" label="Total Tokens" width="130" align="right">
          <template #default="{ row }">{{ row.total_tokens.toLocaleString() }}</template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<style lang="scss" scoped>
.filter-card {
  margin-bottom: 16px;
}

.stat-row {
  margin-bottom: 16px;
}

.stat-card {
  :deep(.el-card__body) {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 16px 20px;
  }

  .stat-icon {
    width: 44px;
    height: 44px;
    border-radius: 10px;
    display: flex;
    align-items: center;
    justify-content: center;
    background-color: var(--el-fill-color-light);
    color: var(--el-text-color-secondary);
    flex-shrink: 0;

    &--primary {
      background-color: var(--el-color-primary-light-9);
      color: var(--el-color-primary);
    }
    &--success {
      background-color: var(--el-color-success-light-9);
      color: var(--el-color-success);
    }
  }

  .stat-info {
    .stat-label {
      font-size: 13px;
      color: var(--el-text-color-secondary);
      margin-bottom: 4px;
    }
    .stat-value {
      font-size: 24px;
      font-weight: 700;
      color: var(--el-text-color-primary);
      line-height: 1.2;
      &--primary { color: var(--el-color-primary); }
    }
  }
}

.section-card {
  margin-bottom: 16px;
}

.chart {
  height: 350px;
}

.w-full {
  width: 100%;
}
</style>
