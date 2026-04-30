<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getTraces, getTraceDates, type TraceItem } from '@/api/traces'

const { t } = useI18n()
const route = useRoute()
const dates = ref<string[]>([])
const items = ref<TraceItem[]>([])
const total = ref(0)
const loading = ref(false)
const filter = reactive({
  date: '',
  model: '',
  provider: '',
  status: undefined as number | string | undefined,
  session_id: (route.query.session_id as string) || '',
  page: 1,
  page_size: 20
})

const formatTime = (time: string) => new Date(time).toLocaleTimeString('en-US', { hour12: false })
const formatLatency = (ms: number) => ms >= 1000 ? `${(ms/1000).toFixed(1)}s` : `${ms}ms`

const fetchList = async () => {
  loading.value = true
  try {
    const res = await getTraces(filter)
    items.value = res.data.items
    total.value = res.data.total
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  dates.value = await getTraceDates()
  if (dates.value.length > 0) {
    if (!filter.date) filter.date = dates.value[0]
    fetchList()
  }
})
</script>

<template>
  <div class="app-container">
    <div class="page-header">
      <div>
        <h3>{{ t('traces.title') }}</h3>
        <p class="text-sm text-slate-500 mt-1">{{ t('traces.desc') }}</p>
      </div>
      <div class="flex gap-2 flex-wrap">
        <el-select v-model="filter.date" :placeholder="t('traces.filter.date')" class="w-40">
          <el-option v-for="d in dates" :key="d" :label="d" :value="d" />
        </el-select>
        <el-input v-model="filter.model" :placeholder="t('traces.filter.model')" class="w-40" clearable />
        <el-input v-model="filter.provider" :placeholder="t('traces.filter.provider')" class="w-40" clearable />
        <el-select v-model="filter.status" :placeholder="t('traces.filter.status')" class="w-30" clearable>
          <el-option :label="t('traces.filter.all')" value="" />
          <el-option label="2xx" :value="200" />
          <el-option label="4xx" :value="400" />
          <el-option label="5xx" :value="500" />
        </el-select>
        <el-input v-model="filter.session_id" :placeholder="t('traces.filter.sessionId')" class="w-60" clearable />
        <el-button type="primary" @click="fetchList">{{ t('traces.filter.apply') }}</el-button>
      </div>
    </div>

    <el-card shadow="never" class="border-none!">
      <el-table v-loading="loading" :data="items" border stripe>
        <el-table-column :label="t('traces.table.time')" width="120">
          <template #default="{ row }">{{ formatTime(row.time) }}</template>
        </el-table-column>
        <el-table-column :label="t('traces.table.requestId')" min-width="200">
          <template #default="{ row }">
            <router-link :to="{ name: 'TraceDetail', params: { ais_req_id: row.ais_req_id }, query: { date: filter.date } }" class="text-blue-500 hover:underline">
              {{ row.ais_req_id }}
            </router-link>
          </template>
        </el-table-column>
        <el-table-column prop="session_id" :label="t('traces.table.sessionId')" width="200" show-overflow-tooltip />
        <el-table-column prop="client_protocol" :label="t('traces.table.protocol')" width="100" />
        <el-table-column prop="model" :label="t('traces.table.model')" min-width="120" />
        <el-table-column prop="provider" :label="t('traces.table.provider')" min-width="120" />
        <el-table-column :label="t('traces.table.status')" width="80">
          <template #default="{ row }">
            <el-tag :type="row.status >= 200 && row.status < 300 ? 'success' : 'danger'">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('traces.table.latency')" width="100">
          <template #default="{ row }">{{ formatLatency(row.latency_ms) }}</template>
        </el-table-column>
        <el-table-column :label="t('traces.table.tokens')" width="120">
          <template #default="{ row }">
            {{ row.input_tokens }}/{{ row.output_tokens }}
          </template>
        </el-table-column>
        <el-table-column :label="t('traces.table.stream')" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="row.stream ? 'info' : undefined">{{ row.stream ? 'SSE' : 'Raw' }}</el-tag>
          </template>
        </el-table-column>
      </el-table>

      <div class="mt-4">
        <el-pagination
          v-model:current-page="filter.page"
          v-model:page-size="filter.page_size"
          :total="total"
          layout="prev, pager, next"
          @current-change="fetchList"
        />
      </div>
    </el-card>
  </div>
</template>
