<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowDown, ArrowUp, Document } from '@element-plus/icons-vue'
import { getTraceDetail, type TraceDetail, type TraceDetailRecord } from '@/api/traces'

const { t } = useI18n()
const route = useRoute()

const goBack = () => window.history.back()
const detail = ref<TraceDetail | null>(null)
const loading = ref(false)

const getRecordStyle = (type: string) => {
  switch (type) {
    case 'request': return { backgroundColor: '#eff6ff', color: '#1e40af', border: '1px solid #bfdbfe' }
    case 'upstream_req': return { backgroundColor: '#fff7ed', color: '#9a3412', border: '1px solid #fed7aa' }
    case 'upstream_resp': return { backgroundColor: '#f0fdf4', color: '#166534', border: '1px solid #bbf7d0' }
    case 'response': return { backgroundColor: '#f5f3ff', color: '#6b21a8', border: '1px solid #ddd6fe' }
    default: return { backgroundColor: '#f1f5f9', color: '#475569', border: '1px solid #cbd5e1' }
  }
}

const formatBody = (body: string) => {
  try {
    const parsed = JSON.parse(body)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return body
  }
}

const formatHeaders = (headers: Record<string, string>) => {
  return JSON.stringify(headers, null, 2)
}

const sessionId = computed(() => {
  return detail.value?.records.find((r: TraceDetailRecord) => r.session_id)?.session_id
})

const viewSession = () => {
  return {
    name: 'Traces',
    query: {
      session_id: sessionId.value,
      start_time: route.query.start_time,
      end_time: route.query.end_time
    }
  }
}

onMounted(async () => {
  loading.value = true
  try {
    const ais_req_id = route.params.ais_req_id as string
    detail.value = await getTraceDetail(ais_req_id)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="app-container" v-loading="loading">
    <div v-if="detail">
      <div class="page-header mb-4">
        <div>
          <h3>{{ t('traces.detail.title') }}</h3>
          <p class="text-sm text-slate-500 mt-1">Request ID: {{ detail.ais_req_id }}</p>
        </div>
        <div class="flex gap-2">
          <a class="el-button el-button--default" @click="goBack">{{ t('traces.back') }}</a>
          <router-link v-if="sessionId" :to="viewSession()" class="el-button el-button--primary">
            {{ t('traces.viewSession') }}
          </router-link>
        </div>
      </div>

      <div class="space-y-6">
        <div v-for="(record, index) in detail.records" :key="index" class="relative">
          <div v-if="index < detail.records.length - 1" class="absolute left-6 top-10 w-0.5 h-12 bg-gray-300"></div>
          
          <el-card shadow="never" class="ml-12 border-none!">
            <template #header>
              <span class="px-2 py-1 rounded text-xs font-medium" :style="getRecordStyle(record.type)">
                {{ record.type }}
              </span>
              <span class="ml-2 text-xs text-gray-400">{{ record.time }}</span>
            </template>

            <div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm text-gray-600 mb-4">
              <div v-if="record.provider"><strong>Provider:</strong> {{ record.provider }}</div>
              <div v-if="record.model"><strong>Model:</strong> {{ record.model }}</div>
              <div v-if="record.status"><strong>Status:</strong> {{ record.status }}</div>
              <div v-if="record.latency_ms"><strong>Latency:</strong> {{ record.latency_ms }}ms</div>
            </div>

            <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <div v-if="record.headers" class="border border-slate-200 rounded-lg shadow-sm bg-white overflow-hidden">
                <button 
                  @click="record.showHeaders = !record.showHeaders"
                  class="w-full flex items-center justify-between p-3 bg-slate-50 hover:bg-slate-100 text-slate-700 font-medium transition-all"
                >
                  <span class="flex items-center gap-2">
                    <el-icon><Document /></el-icon>
                    {{ t('traces.detail.headers') }}
                  </span>
                  <el-icon><ArrowDown v-if="!record.showHeaders" /><ArrowUp v-else /></el-icon>
                </button>
                <div v-if="record.showHeaders" class="p-3 border-t border-slate-200">
                  <pre class="bg-slate-50 p-3 overflow-auto text-xs rounded border border-slate-200 max-h-80">{{ formatHeaders(record.headers) }}</pre>
                </div>
              </div>
              <div class="border border-slate-200 rounded-lg shadow-sm bg-white overflow-hidden">
                <button 
                  @click="record.showBody = !record.showBody"
                  class="w-full flex items-center justify-between p-3 bg-slate-50 hover:bg-slate-100 text-slate-700 font-medium transition-all"
                >
                  <span class="flex items-center gap-2">
                    <el-icon><Document /></el-icon>
                    {{ t('traces.detail.body') }}
                  </span>
                  <el-icon><ArrowDown v-if="!record.showBody" /><ArrowUp v-else /></el-icon>
                </button>
                <div v-if="record.showBody" class="p-3 border-t border-slate-200">
                  <pre class="bg-slate-50 p-3 overflow-auto text-xs rounded border border-slate-200 max-h-80">{{ formatBody(record.body) }}</pre>
                </div>
              </div>
            </div>
          </el-card>
        </div>
      </div>
    </div>
  </div>
</template>
