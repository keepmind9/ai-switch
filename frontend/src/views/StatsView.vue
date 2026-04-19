<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { queryStats, type UsageRecord } from '../api/stats'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'

use([CanvasRenderer, BarChart, GridComponent, TooltipComponent, LegendComponent])

const records = ref<UsageRecord[]>([])
const loading = ref(true)
const dateRange = ref<string[]>([])
const providerFilter = ref('')
const modelFilter = ref('')

const providers = computed(() => [...new Set(records.value.map((r) => r.provider))])
const models = computed(() => [...new Set(records.value.map((r) => r.model))])

const filteredRecords = computed(() => {
  let data = records.value
  if (providerFilter.value) data = data.filter((r) => r.provider === providerFilter.value)
  if (modelFilter.value) data = data.filter((r) => r.model === modelFilter.value)
  return data
})

const totalRequests = computed(() => filteredRecords.value.reduce((s, r) => s + r.requests, 0))
const totalTokens = computed(() => filteredRecords.value.reduce((s, r) => s + r.total_tokens, 0))
const totalInput = computed(() => filteredRecords.value.reduce((s, r) => s + r.input_tokens, 0))
const totalOutput = computed(() => filteredRecords.value.reduce((s, r) => s + r.output_tokens, 0))

const chartOption = computed(() => {
  const byDate: Record<string, { input: number; output: number }> = {}
  for (const r of filteredRecords.value) {
    if (!byDate[r.date]) byDate[r.date] = { input: 0, output: 0 }
    byDate[r.date].input += r.input_tokens
    byDate[r.date].output += r.output_tokens
  }
  const dates = Object.keys(byDate).sort()
  return {
    backgroundColor: 'transparent',
    tooltip: { trigger: 'axis' },
    legend: { data: ['Input Tokens', 'Output Tokens'], textStyle: { color: '#999' } },
    grid: { left: 60, right: 20, top: 40, bottom: 30 },
    xAxis: { type: 'category', data: dates, axisLabel: { color: '#999' } },
    yAxis: { type: 'value', axisLabel: { color: '#999' }, splitLine: { lineStyle: { color: '#333' } } },
    series: [
      { name: 'Input Tokens', type: 'bar', data: dates.map((d) => byDate[d].input), itemStyle: { color: '#409eff' } },
      { name: 'Output Tokens', type: 'bar', data: dates.map((d) => byDate[d].output), itemStyle: { color: '#67c23a' } },
    ],
  }
})

async function load() {
  loading.value = true
  try {
    const params: any = {}
    if (dateRange.value?.length === 2) {
      params.start_date = dateRange.value[0]
      params.end_date = dateRange.value[1]
    }
    const { data } = await queryStats(params)
    records.value = data.data || []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page-header">
    <h2>Usage Statistics</h2>
  </div>

  <div style="display: flex; gap: 12px; margin-bottom: 20px; flex-wrap: wrap">
    <el-date-picker
      v-model="dateRange"
      type="daterange"
      range-separator="—"
      start-placeholder="Start"
      end-placeholder="End"
      value-format="YYYY-MM-DD"
      @change="load"
    />
    <el-select v-model="providerFilter" placeholder="Provider" clearable style="width: 160px">
      <el-option v-for="p in providers" :key="p" :label="p" :value="p" />
    </el-select>
    <el-select v-model="modelFilter" placeholder="Model" clearable style="width: 200px">
      <el-option v-for="m in models" :key="m" :label="m" :value="m" />
    </el-select>
    <el-button @click="load">Refresh</el-button>
  </div>

  <el-row :gutter="16" style="margin-bottom: 20px">
    <el-col :span="6">
      <div class="card" style="text-align: center">
        <div style="color: var(--text-secondary); font-size: 13px; margin-bottom: 4px">Total Requests</div>
        <div style="font-size: 24px; font-weight: 600">{{ totalRequests.toLocaleString() }}</div>
      </div>
    </el-col>
    <el-col :span="6">
      <div class="card" style="text-align: center">
        <div style="color: var(--text-secondary); font-size: 13px; margin-bottom: 4px">Total Tokens</div>
        <div style="font-size: 24px; font-weight: 600">{{ totalTokens.toLocaleString() }}</div>
      </div>
    </el-col>
    <el-col :span="6">
      <div class="card" style="text-align: center">
        <div style="color: var(--text-secondary); font-size: 13px; margin-bottom: 4px">Input Tokens</div>
        <div style="font-size: 24px; font-weight: 600; color: #409eff">{{ totalInput.toLocaleString() }}</div>
      </div>
    </el-col>
    <el-col :span="6">
      <div class="card" style="text-align: center">
        <div style="color: var(--text-secondary); font-size: 13px; margin-bottom: 4px">Output Tokens</div>
        <div style="font-size: 24px; font-weight: 600; color: #67c23a">{{ totalOutput.toLocaleString() }}</div>
      </div>
    </el-col>
  </el-row>

  <div class="card" style="margin-bottom: 20px">
    <v-chart :option="chartOption" style="height: 350px" autoresize />
  </div>

  <div class="card">
    <el-table :data="filteredRecords" v-loading="loading" stripe max-height="400">
      <el-table-column prop="date" label="Date" width="120" />
      <el-table-column prop="provider" label="Provider" width="140" />
      <el-table-column prop="model" label="Model" width="180" />
      <el-table-column prop="requests" label="Requests" width="100" />
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
  </div>
</template>
