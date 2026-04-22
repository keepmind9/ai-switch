<script setup lang="ts">
import { ref, onMounted, computed } from "vue"
import { queryStats, type UsageRecord } from "@/api/stats"
import { listProviders, type Provider } from "@/api/providers"
import { DataAnalysis, Histogram, Files, Mouse } from "@element-plus/icons-vue"
import { useI18n } from "vue-i18n"
import { use } from "echarts/core"
import { CanvasRenderer } from "echarts/renderers"
import { BarChart, LineChart } from "echarts/charts"
import { GridComponent, TooltipComponent, LegendComponent } from "echarts/components"
import VChart from "vue-echarts"

use([CanvasRenderer, BarChart, LineChart, GridComponent, TooltipComponent, LegendComponent])

const { t } = useI18n()
const loading = ref(true)
const records = ref<UsageRecord[]>([])
const providers = ref<Provider[]>([])
const startDate = ref(getDefaultRange()[0])
const endDate = ref(getDefaultRange()[1])
const filterProvider = ref("")
const filterModel = ref("")

function getDefaultRange(): [string, string] {
  const today = fmt(new Date())
  return [today, today]
}

function fmt(d: Date): string {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
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
  const isDark = document.documentElement.classList.contains('dark')
  const daily: Record<string, { input: number; output: number }> = {}
  for (const r of filtered.value) {
    if (!daily[r.date]) daily[r.date] = { input: 0, output: 0 }
    daily[r.date].input += r.input_tokens
    daily[r.date].output += r.output_tokens
  }
  const dates = Object.keys(daily).sort()
  
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: "axis",
      backgroundColor: isDark ? "#1e293b" : "#ffffff",
      borderColor: isDark ? "#334155" : "#e2e8f0",
      textStyle: { color: isDark ? "#f1f5f9" : "#1e293b" },
      padding: [10, 14],
      borderRadius: 8,
      boxShadow: "0 4px 6px -1px rgb(0 0 0 / 0.1)"
    },
    legend: {
      data: [t("stats.cards.inputTokens"), t("stats.cards.outputTokens")],
      bottom: 0,
      icon: "circle",
      itemWidth: 8,
      textStyle: { color: isDark ? "#94a3b8" : "#64748b" }
    },
    grid: { left: "3%", right: "2%", top: "10%", bottom: "12%", containLabel: true },
    xAxis: {
      type: "category",
      data: dates,
      axisLine: { lineStyle: { color: isDark ? "#334155" : "#e2e8f0" } },
      axisLabel: { color: "#94a3b8", fontSize: 11, margin: 12 },
      axisTick: { show: false }
    },
    yAxis: {
      type: "value",
      splitLine: { lineStyle: { color: isDark ? "#1e293b" : "#f1f5f9", type: 'dashed' } },
      axisLabel: { color: "#94a3b8", fontSize: 11 },
    },
    series: [
      {
        name: t("stats.cards.inputTokens"),
        type: "bar",
        stack: "tokens",
        barMaxWidth: 20,
        itemStyle: { borderRadius: [0, 0, 0, 0], color: "#3b82f6" },
        data: dates.map(d => daily[d].input),
      },
      {
        name: t("stats.cards.outputTokens"),
        type: "bar",
        stack: "tokens",
        barMaxWidth: 20,
        itemStyle: { borderRadius: [4, 4, 0, 0], color: "#10b981" },
        data: dates.map(d => daily[d].output),
      },
    ],
  }
})

async function load() {
  loading.value = true
  try {
    const [r, p] = await Promise.all([
      queryStats({ start_date: startDate.value, end_date: endDate.value }),
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
    <div class="page-header">
      <div>
        <h3>{{ $t('stats.title') }}</h3>
        <p class="text-sm text-slate-500 mt-1">{{ $t('stats.desc') }}</p>
      </div>
      <div class="flex gap-3">
        <el-date-picker
          v-model="startDate"
          type="date"
          :placeholder="$t('stats.start')"
          value-format="YYYY-MM-DD"
          size="default"
          style="width: 140px"
        />
        <el-date-picker
          v-model="endDate"
          type="date"
          :placeholder="$t('stats.end')"
          value-format="YYYY-MM-DD"
          size="default"
          style="width: 140px"
        />
        <el-select v-model="filterProvider" :placeholder="$t('stats.provider')" clearable style="width: 150px">
          <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
        </el-select>
        <el-button type="primary" @click="handleSearch" :icon="Mouse">{{ $t('stats.apply') }}</el-button>
      </div>
    </div>

    <!-- Summary Cards -->
    <el-row :gutter="20" class="mb-6">
      <el-col :xs="24" :sm="6">
        <el-card shadow="never" class="stat-card border-none!">
          <div class="stat-label flex items-center gap-2"><el-icon><Histogram /></el-icon> {{ $t('stats.cards.requests') }}</div>
          <div class="stat-value">{{ summary.requests.toLocaleString() }}</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="6">
        <el-card shadow="never" class="stat-card border-none!">
          <div class="stat-label flex items-center gap-2"><el-icon><DataAnalysis /></el-icon> {{ $t('stats.cards.totalTokens') }}</div>
          <div class="stat-value text-blue-600!">{{ summary.total_tokens.toLocaleString() }}</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="6">
        <el-card shadow="never" class="stat-card border-none!">
          <div class="stat-label flex items-center gap-2"><el-icon><Files /></el-icon> {{ $t('stats.cards.inputTokens') }}</div>
          <div class="stat-value text-slate-600!">{{ summary.input_tokens.toLocaleString() }}</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="6">
        <el-card shadow="never" class="stat-card border-none!">
          <div class="stat-label flex items-center gap-2"><el-icon><Files /></el-icon> {{ $t('stats.cards.outputTokens') }}</div>
          <div class="stat-value text-emerald-600!">{{ summary.output_tokens.toLocaleString() }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- Chart -->
    <el-card shadow="never" class="mb-6 border-none!">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="card-header-label">{{ $t('stats.chart.title') }}</span>
          <div class="text-xs text-slate-400">{{ $t('stats.chart.unit') }}</div>
        </div>
      </template>
      <v-chart :option="chartOption" class="chart" autoresize />
    </el-card>

    <!-- Table -->
    <el-card shadow="never" class="border-none!">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="card-header-label">{{ $t('stats.table.title') }}</span>
          <el-select v-model="filterModel" :placeholder="$t('stats.table.filterModel')" clearable size="small" style="width: 200px">
            <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
          </el-select>
        </div>
      </template>
      <el-table :data="filtered" v-loading="loading" stripe size="default">
        <el-table-column prop="date" :label="$t('stats.table.date')" width="120" sortable />
        <el-table-column prop="provider" :label="$t('stats.table.provider')" min-width="150" />
        <el-table-column prop="model" :label="$t('stats.table.model')" min-width="200" />
        <el-table-column prop="requests" :label="$t('stats.table.requests')" width="120" align="right">
          <template #default="{ row }">
            <span class="font-medium">{{ row.requests.toLocaleString() }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="input_tokens" :label="$t('stats.table.input')" width="140" align="right">
          <template #default="{ row }">{{ row.input_tokens.toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="output_tokens" :label="$t('stats.table.output')" width="140" align="right">
          <template #default="{ row }">{{ row.output_tokens.toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="total_tokens" :label="$t('stats.table.total')" width="140" align="right">
          <template #default="{ row }">
            <span class="font-bold text-blue-600">{{ row.total_tokens.toLocaleString() }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<style lang="scss" scoped>
.chart {
  height: 400px;
}

:deep(.el-card__body) {
  padding: 24px;
}

.stat-card {
  .stat-label {
    font-size: 13px;
    margin-bottom: 12px;
    color: #64748b;
  }
  .stat-value {
    font-size: 24px;
    font-weight: 800;
    letter-spacing: -0.02em;
  }
}

html.dark {
  .stat-card .stat-label { color: #94a3b8; }
}
</style>
