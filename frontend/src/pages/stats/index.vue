<script setup lang="ts">
import { ref, onMounted, computed } from "vue"
import { queryStats, type UsageRecord } from "@/api/stats"
import { listProviders, type Provider } from "@/api/providers"
import { useI18n } from "vue-i18n"
import { use } from "echarts/core"
import { CanvasRenderer } from "echarts/renderers"
import { LineChart, PieChart } from "echarts/charts"
import { GridComponent, TooltipComponent, LegendComponent, TitleComponent } from "echarts/components"
import VChart from "vue-echarts"

use([CanvasRenderer, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent, TitleComponent])

const { t } = useI18n()
const loading = ref(true)
const records = ref<UsageRecord[]>([])
const providers = ref<Provider[]>([])
const dateRange = ref("7d")
const startDate = ref("")
const endDate = ref("")
const filterProvider = ref("")
const filterModel = ref("")

function fmt(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${y}-${m}-${day}`
}

function resolveRange(): [string, string] {
  const today = fmt(new Date())
  switch (dateRange.value) {
    case "today": return [today, today]
    case "7d": return [fmt(new Date(Date.now() - 6 * 86400000)), today]
    case "30d": return [fmt(new Date(Date.now() - 29 * 86400000)), today]
    default: return [startDate.value || today, endDate.value || today]
  }
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
    cache_read_tokens: d.reduce((s, r) => s + r.cache_read_tokens, 0),
  }
})

function fmtNum(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, "") + "M"
  return n.toLocaleString()
}

const trendChart = computed(() => {
  const isDark = document.documentElement.classList.contains("dark-blue")
  const daily: Record<string, { input: number; output: number }> = {}
  for (const r of filtered.value) {
    if (!daily[r.date]) daily[r.date] = { input: 0, output: 0 }
    daily[r.date].input += r.input_tokens
    daily[r.date].output += r.output_tokens
  }
  const dates = Object.keys(daily).sort()
  const textColor = isDark ? "#94a3b8" : "#64748b"
  const gridColor = isDark ? "#1e293b" : "#f1f5f9"

  return {
    backgroundColor: "transparent",
    tooltip: {
      trigger: "axis",
      backgroundColor: isDark ? "#0f172a" : "#ffffff",
      borderColor: isDark ? "#1e293b" : "#e2e8f0",
      textStyle: { color: isDark ? "#f1f5f9" : "#1e293b", fontSize: 12 },
      padding: [12, 16],
      borderWidth: 1,
    },
    legend: {
      data: [t("stats.cards.inputTokens"), t("stats.cards.outputTokens")],
      bottom: 0,
      icon: "circle",
      itemWidth: 8,
      itemGap: 20,
      textStyle: { color: textColor, fontSize: 12 },
    },
    grid: { left: "2%", right: "2%", top: "8%", bottom: "14%", containLabel: true },
    xAxis: {
      type: "category",
      data: dates,
      boundaryGap: false,
      axisLine: { lineStyle: { color: isDark ? "#1e293b" : "#e5e2da" } },
      axisLabel: { color: textColor, fontSize: 11, margin: 12 },
      axisTick: { show: false },
    },
    yAxis: {
      type: "value",
      splitLine: { lineStyle: { color: gridColor, type: "dashed" } },
      axisLabel: { color: textColor, fontSize: 11, formatter: (v: number) => fmtNum(v) },
    },
    series: [
      {
        name: t("stats.cards.inputTokens"),
        type: "line",
        smooth: true,
        symbol: "none",
        lineStyle: { width: 2, color: "#3b82f6" },
        areaStyle: { color: { type: "linear", x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: "rgba(59,130,246,0.2)" }, { offset: 1, color: "rgba(59,130,246,0.01)" }] } },
        emphasis: { focus: "series" },
        data: dates.map(d => daily[d].input),
      },
      {
        name: t("stats.cards.outputTokens"),
        type: "line",
        smooth: true,
        symbol: "none",
        lineStyle: { width: 2, color: "#458854" },
        areaStyle: { color: { type: "linear", x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: "rgba(69,136,84,0.2)" }, { offset: 1, color: "rgba(69,136,84,0.01)" }] } },
        emphasis: { focus: "series" },
        data: dates.map(d => daily[d].output),
      },
    ],
  }
})

const distChart = computed(() => {
  const isDark = document.documentElement.classList.contains("dark-blue")
  const byProvider: Record<string, number> = {}
  for (const r of filtered.value) {
    byProvider[r.provider || "unknown"] = (byProvider[r.provider || "unknown"] || 0) + r.total_tokens
  }
  let entries = Object.entries(byProvider).sort((a, b) => b[1] - a[1])
  if (entries.length > 5) {
    const top = entries.slice(0, 5)
    const rest = entries.slice(5).reduce((s, e) => s + e[1], 0)
    entries = [...top, ["Other", rest]]
  }
  const total = entries.reduce((s, e) => s + e[1], 0)
  const palette = ["#d97757", "#3b82f6", "#458854", "#8b5cf6", "#f59e0b", "#94a3b8"]

  return {
    backgroundColor: "transparent",
    tooltip: {
      trigger: "item",
      backgroundColor: isDark ? "#0f172a" : "#ffffff",
      borderColor: isDark ? "#1e293b" : "#e2e8f0",
      textStyle: { color: isDark ? "#f1f5f9" : "#1e293b", fontSize: 12 },
      formatter: (p: { name: string; value: number; percent: number }) =>
        `${p.name}<br/><b>${fmtNum(p.value)}</b> (${p.percent}%)`,
    },
    title: {
      text: fmtNum(total),
      subtext: t("stats.cards.totalTokens"),
      left: "center",
      top: "38%",
      textStyle: { color: isDark ? "#f1f5f9" : "#1d1c16", fontSize: 22, fontWeight: 800, letterSpacing: "-0.02em" },
      subtextStyle: { color: isDark ? "#94a3b8" : "#66635c", fontSize: 11 },
    },
    series: [{
      type: "pie",
      radius: ["58%", "82%"],
      center: ["50%", "50%"],
      avoidLabelOverlap: false,
      itemStyle: { borderColor: isDark ? "#0f172a" : "#ffffff", borderWidth: 3, borderRadius: 6 },
      label: { show: false },
      emphasis: {
        scaleSize: 6,
        label: { show: true, fontSize: 13, fontWeight: 700, color: isDark ? "#f1f5f9" : "#1d1c16" },
      },
      data: entries.map(([name, value], i) => ({ name, value, itemStyle: { color: palette[i % palette.length] } })),
    }],
  }
})

const tableData = computed(() => filtered.value)

function getSummaries(param: { columns: { property: string }[]; data: UsageRecord[] }) {
  const sums: string[] = []
  for (const col of param.columns) {
    const p = col.property
    if (!p) { sums.push(""); continue }
    if (p === "date") { sums.push(t("stats.table.totalRow")); continue }
    if (p === "requests") { sums.push(fmtNum(param.data.reduce((s, r) => s + r.requests, 0))); continue }
    if (p === "input_tokens") { sums.push(fmtNum(param.data.reduce((s, r) => s + r.input_tokens, 0))); continue }
    if (p === "output_tokens") { sums.push(fmtNum(param.data.reduce((s, r) => s + r.output_tokens, 0))); continue }
    if (p === "cache_read_tokens") { sums.push(fmtNum(param.data.reduce((s, r) => s + r.cache_read_tokens, 0))); continue }
    if (p === "total_tokens") { sums.push(fmtNum(param.data.reduce((s, r) => s + r.total_tokens, 0))); continue }
    sums.push("")
  }
  return sums
}

async function load() {
  loading.value = true
  try {
    const [range, p] = await Promise.all([
      queryStats({ start_date: resolveRange()[0], end_date: resolveRange()[1] }),
      listProviders(),
    ])
    records.value = range.data
    providers.value = p.data
  } finally {
    loading.value = false
  }
}

function onRangeChange() {
  if (dateRange.value !== "custom") load()
}

onMounted(load)
</script>

<template>
  <div class="app-container">
    <!-- Header -->
    <div class="page-header">
      <div>
        <h3>{{ t("stats.title") }}</h3>
        <p class="text-sm text-slate-500 mt-1">{{ t("stats.desc") }}</p>
      </div>
    </div>

    <!-- Toolbar -->
    <div class="toolbar">
      <div class="flex items-center gap-2">
        <el-radio-group v-model="dateRange" size="default" @change="onRangeChange" class="range-tabs">
          <el-radio-button value="today">{{ t("stats.range.today") }}</el-radio-button>
          <el-radio-button value="7d">{{ t("stats.range.days7") }}</el-radio-button>
          <el-radio-button value="30d">{{ t("stats.range.days30") }}</el-radio-button>
          <el-radio-button value="custom">{{ t("stats.range.custom") }}</el-radio-button>
        </el-radio-group>
        <template v-if="dateRange === 'custom'">
          <el-date-picker v-model="startDate" type="date" :placeholder="t('stats.start')" value-format="YYYY-MM-DD" size="default" style="width: 140px" />
          <el-date-picker v-model="endDate" type="date" :placeholder="t('stats.end')" value-format="YYYY-MM-DD" size="default" style="width: 140px" />
          <el-button type="primary" @click="load" size="default">{{ t("stats.apply") }}</el-button>
        </template>
      </div>
      <div class="flex items-center gap-3">
        <el-select v-model="filterProvider" :placeholder="t('stats.provider')" clearable size="default" style="width: 150px" @change="load">
          <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
        </el-select>
        <el-select v-model="filterModel" :placeholder="t('stats.table.filterModel')" clearable size="default" style="width: 200px">
          <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
        </el-select>
      </div>
    </div>

    <!-- Summary Cards -->
    <div class="cards-grid">
      <div class="stat-card" data-accent="peach">
        <div class="stat-accent" />
        <div class="stat-body">
          <span class="stat-label">{{ t("stats.cards.requests") }}</span>
          <span class="stat-value">{{ fmtNum(summary.requests) }}</span>
        </div>
      </div>
      <div class="stat-card" data-accent="blue">
        <div class="stat-accent" />
        <div class="stat-body">
          <span class="stat-label">{{ t("stats.cards.totalTokens") }}</span>
          <span class="stat-value text-blue-600!">{{ fmtNum(summary.total_tokens) }}</span>
        </div>
      </div>
      <div class="stat-card" data-accent="slate">
        <div class="stat-accent" />
        <div class="stat-body">
          <span class="stat-label">{{ t("stats.cards.inputTokens") }}</span>
          <span class="stat-value">{{ fmtNum(summary.input_tokens) }}</span>
        </div>
      </div>
      <div class="stat-card" data-accent="green">
        <div class="stat-accent" />
        <div class="stat-body">
          <span class="stat-label">{{ t("stats.cards.outputTokens") }}</span>
          <span class="stat-value" style="color: #458854">{{ fmtNum(summary.output_tokens) }}</span>
        </div>
      </div>
      <div class="stat-card" data-accent="purple">
        <div class="stat-accent" />
        <div class="stat-body">
          <span class="stat-label">{{ t("stats.cards.cacheTokens") }}</span>
          <span class="stat-value" style="color: #8b5cf6">{{ fmtNum(summary.cache_read_tokens) }}</span>
        </div>
      </div>
    </div>

    <!-- Charts -->
    <div class="charts-row">
      <el-card shadow="never" class="chart-card border-none!">
        <template #header>
          <span class="card-header-label">{{ t("stats.chart.dailyTrend") }}</span>
        </template>
        <v-chart :option="trendChart" class="chart" autoresize />
      </el-card>
      <el-card shadow="never" class="chart-card chart-card--dist border-none!">
        <template #header>
          <span class="card-header-label">{{ t("stats.chart.providerDist") }}</span>
        </template>
        <v-chart :option="distChart" class="chart" autoresize />
      </el-card>
    </div>

    <!-- Table -->
    <el-card shadow="never" class="border-none!">
      <template #header>
        <span class="card-header-label">{{ t("stats.table.title") }}</span>
      </template>
      <el-table :data="tableData" v-loading="loading" stripe size="default" show-summary :summary-method="getSummaries">
        <el-table-column prop="date" :label="t('stats.table.date')" width="120" sortable />
        <el-table-column prop="provider" :label="t('stats.table.provider')" min-width="130" />
        <el-table-column prop="model" :label="t('stats.table.model')" min-width="180">
          <template #default="{ row }">
            <el-tag size="small" effect="plain" round>{{ row.model }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="requests" :label="t('stats.table.requests')" width="100" align="right">
          <template #default="{ row }">{{ fmtNum(row.requests) }}</template>
        </el-table-column>
        <el-table-column prop="input_tokens" :label="t('stats.table.input')" width="120" align="right">
          <template #default="{ row }">{{ fmtNum(row.input_tokens) }}</template>
        </el-table-column>
        <el-table-column prop="output_tokens" :label="t('stats.table.output')" width="120" align="right">
          <template #default="{ row }">{{ fmtNum(row.output_tokens) }}</template>
        </el-table-column>
        <el-table-column prop="cache_read_tokens" :label="t('stats.table.cacheRead')" width="120" align="right">
          <template #default="{ row }">
            <span v-if="row.cache_read_tokens > 0" style="color: #8b5cf6">{{ fmtNum(row.cache_read_tokens) }}</span>
            <span v-else class="text-slate-400">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="total_tokens" :label="t('stats.table.total')" width="120" align="right">
          <template #default="{ row }">
            <span class="font-semibold">{{ fmtNum(row.total_tokens) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<style lang="scss" scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 20px;
}

.range-tabs {
  :deep(.el-radio-button__inner) {
    border-radius: 8px !important;
    border: 1px solid var(--el-border-color-light) !important;
    box-shadow: none !important;
    font-size: 13px;
    padding: 6px 16px;
    font-weight: 500;
    transition: all 0.2s ease;
    margin-right: 6px;
  }
  :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
    background: var(--el-color-primary);
    border-color: var(--el-color-primary) !important;
    color: #fff;
  }
}

// Stat Cards
.cards-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 16px;
  margin-bottom: 24px;

  @media (max-width: 1200px) { grid-template-columns: repeat(2, 1fr); }
  @media (max-width: 480px) { grid-template-columns: 1fr; }
}

.stat-card {
  background: var(--el-bg-color);
  border-radius: var(--v3-border-radius-base);
  overflow: hidden;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
  box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.04);

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 8px 24px -4px rgba(0, 0, 0, 0.08);
  }
}

.stat-accent {
  height: 3px;

  [data-accent="peach"] & { background: linear-gradient(90deg, #d97757, #e5a089); }
  [data-accent="blue"] & { background: linear-gradient(90deg, #3b82f6, #93c5fd); }
  [data-accent="slate"] & { background: linear-gradient(90deg, #64748b, #94a3b8); }
  [data-accent="green"] & { background: linear-gradient(90deg, #458854, #86efac); }
  [data-accent="purple"] & { background: linear-gradient(90deg, #8b5cf6, #c4b5fd); }
}

.stat-body {
  padding: 20px;
}

.stat-label {
  display: block;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--el-text-color-regular);
  margin-bottom: 10px;
}

.stat-value {
  display: block;
  font-size: 26px;
  font-weight: 800;
  letter-spacing: -0.03em;
  color: var(--v3-title-text-color);
  line-height: 1.1;
}

// Charts
.charts-row {
  display: grid;
  grid-template-columns: 1fr 380px;
  gap: 16px;
  margin-bottom: 24px;

  @media (max-width: 1200px) { grid-template-columns: 1fr; }
}

.chart-card {
  :deep(.el-card__header) {
    padding: 16px 20px;
    border-bottom: 1px solid var(--el-border-color-lighter);
  }
}

.chart {
  height: 360px;
}

.card-header-label {
  font-size: 13px;
  font-weight: 700;
  color: var(--v3-title-text-color);
  letter-spacing: -0.01em;
}

// Dark theme overrides
html.dark-blue {
  .stat-card:hover {
    box-shadow: 0 8px 24px -4px rgba(0, 0, 0, 0.3);
  }
}
</style>
