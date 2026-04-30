<script setup lang="ts">
import { ref, onMounted, reactive, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getTraces, type TraceItem } from '@/api/traces'

const { t } = useI18n()
const route = useRoute()
const items = ref<TraceItem[]>([])
const has_prev = ref(false)
const has_next = ref(false)
const prev_cursor = ref('')
const next_cursor = ref('')
const loading = ref(false)
const filter = reactive({
  start_time: '',
  end_time: '',
  model: '',
  provider: '',
  status: undefined as number | string | undefined,
  session_id: (route.query.session_id as string) || '',
  page_size: 20
})

const formatTime = (time: string) => {
  const d = new Date(time)
  return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${d.toLocaleTimeString('en-US', { hour12: false })}`
}

const formatLatency = (ms: number | undefined) => (!ms || ms <= 0) ? '-' : (ms >= 1000 ? `${(ms/1000).toFixed(1)}s` : `${ms}ms`)

const canApply = computed(() => filter.start_time && filter.end_time)

const fetchList = async (c?: string) => {
  if (!filter.start_time || !filter.end_time) return
  loading.value = true
  try {
    const res = await getTraces({
        ...filter, 
        cursor: c || '', 
        status: filter.status ? Number(filter.status) : undefined
    })
    items.value = res.items
    has_prev.value = res.has_prev
    has_next.value = res.has_next
    prev_cursor.value = res.prev_cursor
    next_cursor.value = res.next_cursor
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (route.query.start_time) filter.start_time = route.query.start_time as string
  if (route.query.end_time) filter.end_time = route.query.end_time as string
  if (route.query.session_id) filter.session_id = route.query.session_id as string
  
  if (filter.start_time && filter.end_time) {
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
    </div>

    <el-card shadow="never" class="mb-4">
      <div class="flex flex-wrap gap-3">
        <el-date-picker v-model="filter.start_time" type="datetime" :placeholder="t('traces.filter.startTime')" value-format="YYYY-MM-DD HH:mm:ss" class="w-50" />
        <el-date-picker v-model="filter.end_time" type="datetime" :placeholder="t('traces.filter.endTime')" value-format="YYYY-MM-DD HH:mm:ss" class="w-50" />
        <el-input v-model="filter.model" :placeholder="t('traces.filter.model')" class="w-35" clearable />
        <el-input v-model="filter.provider" :placeholder="t('traces.filter.provider')" class="w-35" clearable />
        <el-select v-model="filter.status" :placeholder="t('traces.filter.status')" class="w-25" clearable>
          <el-option :label="t('traces.filter.all')" value="" />
          <el-option label="2xx" :value="200" />
          <el-option label="4xx" :value="400" />
          <el-option label="5xx" :value="500" />
        </el-select>
        <el-input v-model="filter.session_id" :placeholder="t('traces.filter.sessionId')" class="w-50" clearable />
        <el-button type="primary" :disabled="!canApply" @click="fetchList()">{{ t('traces.filter.apply') }}</el-button>
      </div>
    </el-card>

    <el-card shadow="never" class="border-none!">
      <el-table v-if="items.length > 0" v-loading="loading" :data="items" border stripe>
        <el-table-column :label="t('traces.table.time')" width="160">
          <template #default="{ row }">{{ formatTime(row.time) }}</template>
        </el-table-column>
        <el-table-column :label="t('traces.table.requestId')" min-width="200">
          <template #default="{ row }">
            <router-link :to="{ name: 'TraceDetail', params: { ais_req_id: row.ais_req_id }, query: { start_time: filter.start_time, end_time: filter.end_time } }" class="text-blue-500 hover:underline">
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
      <div v-else class="text-center py-20 text-slate-400">
        {{ loading ? 'Loading...' : 'Please select a time range to search.' }}
      </div>

      <div class="mt-4 flex justify-between items-center">
        <el-button :disabled="!has_prev" @click="() => fetchList()">{{ t('traces.nav.first') }}</el-button>
        <div class="flex gap-2">
            <el-button :disabled="!has_prev" @click="() => fetchList(prev_cursor)">{{ t('traces.nav.prev') }}</el-button>
            <el-button :disabled="!has_next" @click="() => fetchList(next_cursor)">{{ t('traces.nav.next') }}</el-button>
        </div>
      </div>
    </el-card>
  </div>
</template>
