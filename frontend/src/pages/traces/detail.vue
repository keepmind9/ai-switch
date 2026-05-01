<script setup lang="ts">
import { ref, onMounted, computed, reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowDown, ArrowUp, Document, View, Switch } from '@element-plus/icons-vue'
import { getTraceDetail, type TraceDetail, type TraceDetailRecord } from '@/api/traces'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const goBack = () => router.back()
const detail = ref<TraceDetail | null>(null)
const loading = ref(false)
const expanded = reactive<Record<string, { headers: boolean; body: boolean }>>({})

const toggle = (index: number, key: 'headers' | 'body') => {
  if (!expanded[index]) expanded[index] = { headers: false, body: false }
  expanded[index][key] = !expanded[index][key]
}

const isOpen = (index: number, key: 'headers' | 'body') => !!expanded[index]?.[key]

// Inspect drawer state
const inspectOpen = ref(false)
const inspectRecord = ref<TraceDetailRecord | null>(null)
const inspectParsed = computed(() => {
  if (!inspectRecord.value) return null
  try { return JSON.parse(inspectRecord.value.body) } catch { return null }
})

// Parse SSE stream body into individual events
const inspectSSE = computed<{ events: any[]; usage: any | null; stopReason: string | null } | null>(() => {
  if (!inspectRecord.value || !isResponse(inspectRecord.value.type)) return null
  const body = inspectRecord.value.body
  if (!body.includes('data: ')) return null

  const events: any[] = []
  let usage: any = null
  let stopReason: string | null = null

  for (const line of body.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('data: ')) continue
    const data = trimmed.slice(6)
    if (data === '[DONE]') continue
    try {
      const parsed = JSON.parse(data)
      events.push(parsed)
      // Extract usage from stream events
      if (parsed.usage) usage = parsed.usage
      if (parsed.message?.usage) usage = parsed.message.usage
      // Extract stop reason
      const delta = parsed.choices?.[0]
      if (delta?.finish_reason) stopReason = delta.finish_reason
      if (parsed.delta?.stop_reason) stopReason = parsed.delta.stop_reason
      if (parsed.stop_reason) stopReason = parsed.stop_reason
      if (parsed.type === 'message_stop') stopReason = 'end_turn'
    } catch { /* skip unparseable lines */ }
  }

  return events.length > 0 ? { events, usage, stopReason } : null
})

const openInspect = (record: TraceDetailRecord) => {
  inspectRecord.value = record
  inspectOpen.value = true
}

// Diff drawer state
const diffOpen = ref(false)
const diffPairs = computed(() => {
  if (!detail.value) return []
  const recs = detail.value.records
  const pairs: { left: TraceDetailRecord; right: TraceDetailRecord; label: string }[] = []
  const reqIdx = recs.findIndex(r => r.type === 'request')
  const upReqIdx = recs.findIndex(r => r.type === 'upstream_req')
  const upRespIdx = recs.findIndex(r => r.type === 'upstream_resp')
  const respIdx = recs.findIndex(r => r.type === 'response')
  if (reqIdx >= 0 && upReqIdx >= 0) pairs.push({ left: recs[reqIdx], right: recs[upReqIdx], label: 'Request vs Upstream Request' })
  if (upRespIdx >= 0 && respIdx >= 0) pairs.push({ left: recs[upRespIdx], right: recs[respIdx], label: 'Upstream Response vs Client Response' })
  return pairs
})
const canDiff = computed(() => diffPairs.value.length > 0)

// Waterfall chart data
interface WaterfallPhase {
  key: string
  label: string
  color: string
  durationMs: number
  startMs: number
  endMs: number
}

const waterfallPhases = computed<{ phases: WaterfallPhase[]; totalMs: number } | null>(() => {
  if (!detail.value) return null
  const recs = detail.value.records
  const req = recs.find(r => r.type === 'request')
  const upReq = recs.find(r => r.type === 'upstream_req')
  const upResp = recs.find(r => r.type === 'upstream_resp')
  const resp = recs.find(r => r.type === 'response')
  if (!req || !resp) return null

  const t0 = new Date(req.time).getTime()
  const phases: WaterfallPhase[] = []

  const addPhase = (key: string, label: string, color: string, from: TraceDetailRecord, to: TraceDetailRecord) => {
    const startMs = new Date(from.time).getTime() - t0
    const endMs = new Date(to.time).getTime() - t0
    const durationMs = endMs - startMs
    if (durationMs >= 0) {
      phases.push({ key, label, color, durationMs, startMs, endMs })
    }
  }

  if (upReq) addPhase('route', t('traces.detail.waterfall.route'), '#d97706', req, upReq)
  if (upReq && upResp) addPhase('upstream', t('traces.detail.waterfall.upstream'), '#059669', upReq, upResp)
  if (upResp && resp) addPhase('response', t('traces.detail.waterfall.response'), '#7c3aed', upResp, resp)

  if (phases.length === 0) return null
  const totalMs = new Date(resp.time).getTime() - t0
  if (totalMs <= 0) return null
  return { phases, totalMs }
})

const requestTtfb = computed<number | null>(() => {
  if (!detail.value) return null
  const resp = detail.value.records.find(r => r.type === 'response')
  if (!resp || resp.client_ttfb_ms == null) return null
  return resp.client_ttfb_ms
})

const openDiff = () => { diffOpen.value = true }

// Simple line diff: returns { type: 'equal'|'add'|'remove'|'change', left?: string, right?: string }[]
const lineDiff = (left: string, right: string): { type: string; left?: string; right?: string }[] => {
  const la = left.split('\n')
  const ra = right.split('\n')
  const result: { type: string; left?: string; right?: string }[] = []
  const maxLen = Math.max(la.length, ra.length)
  for (let i = 0; i < maxLen; i++) {
    const l = i < la.length ? la[i] : undefined
    const r = i < ra.length ? ra[i] : undefined
    if (l === r) {
      result.push({ type: 'equal', left: l })
    } else {
      result.push({ type: 'change', left: l, right: r })
    }
  }
  return result
}

const diffBody = (left: TraceDetailRecord, right: TraceDetailRecord) => {
  const lb = formatBody(left.body)
  const rb = formatBody(right.body)
  return lineDiff(lb, rb)
}

const diffHeaderKeys = (left: TraceDetailRecord, right: TraceDetailRecord) => {
  const lh = left.headers || {}
  const rh = right.headers || {}
  const allKeys = [...new Set([...Object.keys(lh), ...Object.keys(rh)])].sort()
  return allKeys.map(k => ({
    key: k,
    left: lh[k],
    right: rh[k],
    type: lh[k] === rh[k] ? 'equal' : (!lh[k] ? 'add' : !rh[k] ? 'remove' : 'change')
  }))
}

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
  try { return JSON.stringify(JSON.parse(body), null, 2) } catch { return body }
}
const formatJson = (v: any): string => {
  try { return typeof v === 'string' ? v : JSON.stringify(v, null, 2) } catch { return String(v) }
}
const formatHeaders = (headers: Record<string, string>) => JSON.stringify(headers, null, 2)

// --- Structured parsing helpers ---

const isRequest = (type: string) => type === 'request' || type === 'upstream_req'
const isResponse = (type: string) => type === 'upstream_resp' || type === 'response'

const getSystem = (body: any): string | null => {
  if (!body) return null
  if (typeof body.system === 'string') return body.system
  if (Array.isArray(body.system)) return body.system.map((b: any) => b.text || '').filter(Boolean).join('\n')
  if (body.instructions) return body.instructions
  return null
}

const getMessages = (body: any): any[] => body?.messages || []
const getTools = (body: any): any[] => body?.tools || []

const getParams = (body: any): Record<string, any> => {
  if (!body) return {}
  const skip = new Set(['messages', 'system', 'tools', 'tool_choice', 'input', 'instructions',
    'stream', 'stream_options', 'content', 'metadata'])
  const params: Record<string, any> = {}
  for (const [k, v] of Object.entries(body)) {
    if (!skip.has(k) && v !== undefined && v !== null && v !== '' && v !== false) params[k] = v
  }
  return params
}

const getContent = (body: any): any[] => {
  if (!body) return []
  if (Array.isArray(body.content)) return body.content
  if (body.choices?.[0]?.message?.content) return [{ type: 'text', text: body.choices[0].message.content }]
  if (body.choices?.[0]?.message?.tool_calls) {
    return body.choices[0].message.tool_calls.map((tc: any) => ({
      type: 'tool_use', name: tc.function?.name, input: tc.function?.arguments, id: tc.id
    }))
  }
  if (Array.isArray(body.output)) {
    return body.output.flatMap((item: any) => {
      if (item.content) return item.content
      if (item.type === 'function_call') return [{ type: 'tool_use', name: item.name, input: item.arguments, id: item.call_id }]
      return [item]
    })
  }
  return []
}

const getUsage = (body: any): Record<string, number> | null => {
  if (!body?.usage) return null
  const u = body.usage
  return {
    input_tokens: u.input_tokens || u.prompt_tokens || 0,
    output_tokens: u.output_tokens || u.completion_tokens || 0,
    cache_read_tokens: u.cache_read_input_tokens || 0,
    cache_creation_tokens: u.cache_creation_input_tokens || 0,
  }
}

const getStopReason = (body: any): string | null => {
  if (!body) return null
  return body.stop_reason || body.stop_sequence || body.choices?.[0]?.finish_reason || null
}

const getRoleColor = (role: string) => {
  const map: Record<string, string> = { user: '#2563eb', assistant: '#059669', system: '#d97706', tool: '#7c3aed' }
  return map[role] || '#64748b'
}
const getRoleBg = (role: string) => {
  const map: Record<string, string> = { user: '#eff6ff', assistant: '#f0fdf4', system: '#fffbeb', tool: '#f5f3ff' }
  return map[role] || '#f8fafc'
}

const getMessageContent = (msg: any): string => {
  if (!msg.content) return ''
  if (typeof msg.content === 'string') return msg.content
  if (Array.isArray(msg.content)) {
    return msg.content.map((b: any) => {
      if (b.type === 'text') return b.text
      if (b.type === 'thinking') return `[Thinking] ${b.thinking}`
      if (b.type === 'tool_use') return `[Tool: ${b.name}]\n${JSON.stringify(b.input, null, 2)}`
      if (b.type === 'tool_result') return `[Tool Result: ${b.tool_use_id}]\n${typeof b.content === 'string' ? b.content : JSON.stringify(b.content)}`
      return JSON.stringify(b)
    }).join('\n')
  }
  return JSON.stringify(msg.content)
}

const getToolName = (tool: any): string => tool.name || tool.function?.name || ''
const getToolDesc = (tool: any): string => tool.description || tool.function?.description || ''
const getToolSchema = (tool: any): any => tool.input_schema || tool.function?.parameters || tool.parameters || null

const sessionId = computed(() => {
  return detail.value?.records.find((r: TraceDetailRecord) => r.session_id)?.session_id
})

const viewSession = () => ({
  name: 'Traces',
  query: { session_id: sessionId.value, start_time: route.query.start_time, end_time: route.query.end_time }
})

onMounted(async () => {
  loading.value = true
  try {
    detail.value = await getTraceDetail(route.params.ais_req_id as string)
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
          <el-button v-if="canDiff" size="default" :icon="Switch" @click="openDiff">{{ t('traces.detail.diff') }}</el-button>
          <router-link v-if="sessionId" :to="viewSession()" class="el-button el-button--primary">
            {{ t('traces.viewSession') }}
          </router-link>
        </div>
      </div>

      <!-- Waterfall chart -->
      <el-card v-if="waterfallPhases" shadow="never" class="mb-4">
        <template #header>
          <div class="flex items-center justify-between">
            <span class="text-sm font-semibold text-gray-700">{{ t('traces.detail.waterfall.title') }}</span>
            <span class="text-xs text-gray-400">{{ t('traces.detail.waterfall.total') }}: {{ waterfallPhases.totalMs.toLocaleString() }}ms</span>
          </div>
        </template>
        <!-- Single stacked bar -->
        <div class="flex h-8 rounded-lg overflow-hidden" style="min-width: 200px">
          <template v-for="phase in waterfallPhases.phases" :key="phase.key">
            <div
              class="flex items-center justify-center text-white text-xs font-medium transition-all duration-300"
              :style="{
                width: `${Math.max((phase.durationMs / waterfallPhases.totalMs) * 100, 1)}%`,
                backgroundColor: phase.color,
              }"
              :title="`${phase.label}: ${phase.durationMs.toLocaleString()}ms`"
            >
              <span v-if="phase.durationMs >= waterfallPhases.totalMs * 0.12" class="whitespace-nowrap px-1">
                {{ phase.label }} {{ phase.durationMs.toLocaleString() }}ms
              </span>
            </div>
          </template>
        </div>
        <!-- Legend -->
        <div class="flex flex-wrap gap-4 mt-3 text-xs text-gray-600">
          <div v-for="phase in waterfallPhases.phases" :key="phase.key" class="flex items-center gap-1.5">
            <span class="inline-block w-3 h-3 rounded-sm" :style="{ backgroundColor: phase.color }"></span>
            <span>{{ phase.label }}</span>
            <span class="text-gray-400">{{ phase.durationMs.toLocaleString() }}ms</span>
          </div>
        </div>
      </el-card>

      <div class="space-y-6">
        <div v-for="(record, index) in detail.records" :key="index" class="relative">
          <div v-if="index < detail.records.length - 1" class="absolute left-6 top-10 w-0.5 h-12 bg-gray-300"></div>

          <el-card shadow="never" class="ml-12 border-none!">
            <template #header>
              <div class="flex items-center justify-between">
                <div>
                  <span class="px-2 py-1 rounded text-xs font-medium" :style="getRecordStyle(record.type)">
                    {{ record.type }}
                  </span>
                  <span class="ml-2 text-xs text-gray-400">{{ record.time }}</span>
                </div>
                <el-button v-if="record.body" size="small" :icon="View" @click="openInspect(record)">
                  {{ t('traces.detail.inspect') }}
                </el-button>
              </div>
            </template>

            <div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm text-gray-600 mb-4">
              <div v-if="record.provider"><strong>Provider:</strong> {{ record.provider }}</div>
              <div v-if="record.model"><strong>Model:</strong> {{ record.model }}</div>
              <div v-if="record.status"><strong>Status:</strong> {{ record.status }}</div>
              <div v-if="record.latency_ms"><strong>TTFB:</strong> {{ record.latency_ms.toLocaleString() }} ms</div>
              <div v-if="record.type === 'request' && requestTtfb != null"><strong>TTFB:</strong> {{ requestTtfb.toLocaleString() }} ms</div>
            </div>

            <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <div v-if="record.headers" class="border border-slate-200 rounded-lg shadow-sm bg-white overflow-hidden">
                <button @click="toggle(index, 'headers')"
                  class="w-full flex items-center justify-between p-3 bg-slate-50 hover:bg-slate-100 text-slate-700 font-medium transition-all">
                  <span class="flex items-center gap-2">
                    <el-icon><Document /></el-icon>
                    {{ t('traces.detail.headers') }}
                  </span>
                  <el-icon><ArrowDown v-if="!isOpen(index, 'headers')" /><ArrowUp v-else /></el-icon>
                </button>
                <div v-if="isOpen(index, 'headers')" class="p-3 border-t border-slate-200">
                  <pre class="bg-slate-50 p-3 overflow-auto text-xs rounded border border-slate-200 max-h-80">{{ formatHeaders(record.headers) }}</pre>
                </div>
              </div>
              <div class="border border-slate-200 rounded-lg shadow-sm bg-white overflow-hidden">
                <button @click="toggle(index, 'body')"
                  class="w-full flex items-center justify-between p-3 bg-slate-50 hover:bg-slate-100 text-slate-700 font-medium transition-all">
                  <span class="flex items-center gap-2">
                    <el-icon><Document /></el-icon>
                    {{ t('traces.detail.body') }}
                  </span>
                  <el-icon><ArrowDown v-if="!isOpen(index, 'body')" /><ArrowUp v-else /></el-icon>
                </button>
                <div v-if="isOpen(index, 'body')" class="p-3 border-t border-slate-200">
                  <pre class="bg-slate-50 p-3 overflow-auto text-xs rounded border border-slate-200 max-h-80">{{ formatBody(record.body) }}</pre>
                </div>
              </div>
            </div>
          </el-card>
        </div>
      </div>
    </div>

    <!-- Inspect Drawer -->
    <el-drawer v-model="inspectOpen" size="60%" direction="rtl">
      <template #header>
        <div class="inspect-header">
          <span class="inspect-title">{{ t('traces.detail.inspect') }}</span>
          <span v-if="inspectRecord" class="inspect-meta">{{ inspectRecord.type }} &middot; {{ inspectRecord.time }}</span>
        </div>
      </template>
      <template v-if="inspectRecord">
        <!-- Request sections -->
        <template v-if="isRequest(inspectRecord.type) && inspectParsed">
          <!-- System Prompt -->
          <div v-if="getSystem(inspectParsed)" class="mb-5">
            <div class="section-label">System Prompt</div>
            <div class="sys-block">{{ getSystem(inspectParsed) }}</div>
          </div>
          <!-- Messages -->
          <div v-if="getMessages(inspectParsed).length" class="mb-5">
            <div class="section-label">Messages ({{ getMessages(inspectParsed).length }})</div>
            <div class="space-y-2">
              <details v-for="(msg, mi) in getMessages(inspectParsed)" :key="mi" class="msg-details">
                <summary class="msg-summary">
                  <el-tag size="small" :type="msg.role === 'user' ? 'primary' : msg.role === 'assistant' ? 'success' : msg.role === 'system' ? 'warning' : 'info'" effect="dark">
                    {{ msg.role }}
                  </el-tag>
                  <span class="text-xs text-gray-400 font-mono ml-2">
                    {{ (getMessageContent(msg).slice(0, 80) || (msg.tool_calls?.length ? `${msg.tool_calls.length} tool call(s)` : '')) }}{{ getMessageContent(msg).length > 80 ? '...' : '' }}
                  </span>
                </summary>
                <div class="msg-body">
                  <div v-if="msg.tool_calls?.length" class="space-y-2">
                    <div v-for="(tc, tci) in msg.tool_calls" :key="tci" class="tool-call-block">
                      <span class="font-semibold text-purple-700">{{ tc.function?.name }}</span>
                      <pre class="mt-1 text-gray-600 whitespace-pre-wrap">{{ tc.function?.arguments }}</pre>
                    </div>
                  </div>
                  <div v-else class="whitespace-pre-wrap break-words text-sm text-gray-700">{{ getMessageContent(msg) }}</div>
                </div>
              </details>
            </div>
          </div>
          <!-- Input (Responses API) -->
          <div v-if="inspectParsed.input && !getMessages(inspectParsed).length" class="mb-5">
            <div class="section-label">Input</div>
            <pre class="code-block">{{ formatJson(inspectParsed.input) }}</pre>
          </div>
          <!-- Tools -->
          <div v-if="getTools(inspectParsed).length" class="mb-5">
            <div class="section-label">Tools ({{ getTools(inspectParsed).length }})</div>
            <div class="space-y-2">
              <details v-for="(tool, ti) in getTools(inspectParsed)" :key="ti" class="tool-def-block">
                <summary class="px-3 py-2 cursor-pointer text-sm font-medium text-gray-700 hover:bg-gray-50 rounded-lg">
                  <el-tag size="small" type="warning" class="mr-2">{{ getToolName(tool) }}</el-tag>
                  <span class="text-gray-400 text-xs">{{ getToolDesc(tool).slice(0, 80) }}{{ getToolDesc(tool).length > 80 ? '...' : '' }}</span>
                </summary>
                <div v-if="getToolSchema(tool)" class="px-3 pb-3 border-t border-gray-100">
                  <pre class="mt-2 text-xs text-gray-600 bg-gray-50 p-2 rounded overflow-auto max-h-40">{{ formatJson(getToolSchema(tool)) }}</pre>
                </div>
              </details>
            </div>
          </div>
          <!-- Parameters -->
          <div v-if="Object.keys(getParams(inspectParsed)).length" class="mb-5">
            <div class="section-label">Parameters</div>
            <div class="flex flex-wrap gap-2">
              <el-tag v-for="(val, key) in getParams(inspectParsed)" :key="key" size="small" type="info" effect="plain">
                {{ key }}: {{ typeof val === 'object' ? JSON.stringify(val) : val }}
              </el-tag>
            </div>
          </div>
        </template>

        <!-- Response sections -->
        <template v-if="isResponse(inspectRecord.type)">
          <!-- SSE stream view -->
          <template v-if="inspectSSE">
            <div class="mb-5">
              <div class="section-label">Stream Events ({{ inspectSSE.events.length }})</div>
              <div class="space-y-1.5 max-h-[50vh] overflow-auto">
                <details v-for="(evt, ei) in inspectSSE.events" :key="ei" class="bg-gray-50 border border-gray-200 rounded">
                  <summary class="px-3 py-1.5 cursor-pointer text-xs text-gray-600 hover:bg-gray-100">
                    <span class="font-mono text-gray-400 mr-2">#{{ ei + 1 }}</span>
                    <span v-if="evt.type" class="text-blue-600 font-medium">{{ evt.type }}</span>
                    <span v-if="evt.event" class="text-blue-600 font-medium">{{ evt.event }}</span>
                    <span v-if="evt.choices?.[0]?.delta?.content" class="text-gray-500 ml-2 truncate max-w-md inline-block align-bottom">{{ evt.choices[0].delta.content }}</span>
                    <span v-if="evt.choices?.[0]?.delta?.tool_calls" class="text-purple-600 ml-2">tool_call</span>
                    <span v-if="evt.choices?.[0]?.finish_reason" class="text-green-600 ml-2">[{{ evt.choices[0].finish_reason }}]</span>
                    <span v-if="evt.delta?.stop_reason" class="text-green-600 ml-2">[{{ evt.delta.stop_reason }}]</span>
                  </summary>
                  <pre class="px-3 py-2 text-xs text-gray-600 border-t border-gray-100 overflow-auto max-h-40">{{ formatJson(evt) }}</pre>
                </details>
              </div>
            </div>
            <!-- SSE Usage -->
            <div v-if="inspectSSE.usage" class="mb-5">
              <div class="section-label">Token Usage</div>
              <div class="flex flex-wrap gap-3">
                <div class="usage-card usage-input">
                  <div class="usage-value">{{ (inspectSSE.usage.input_tokens || inspectSSE.usage.prompt_tokens || 0).toLocaleString() }}</div>
                  <div class="usage-label">Input</div>
                </div>
                <div class="usage-card usage-output">
                  <div class="usage-value">{{ (inspectSSE.usage.output_tokens || inspectSSE.usage.completion_tokens || 0).toLocaleString() }}</div>
                  <div class="usage-label">Output</div>
                </div>
                <div v-if="inspectSSE.usage.cache_read_input_tokens" class="usage-card usage-cache-read">
                  <div class="usage-value">{{ inspectSSE.usage.cache_read_input_tokens.toLocaleString() }}</div>
                  <div class="usage-label">Cache Read</div>
                </div>
                <div v-if="inspectSSE.usage.cache_creation_input_tokens" class="usage-card usage-cache-create">
                  <div class="usage-value">{{ inspectSSE.usage.cache_creation_input_tokens.toLocaleString() }}</div>
                  <div class="usage-label">Cache Create</div>
                </div>
              </div>
            </div>
            <div v-if="inspectSSE.stopReason" class="mb-5">
              <el-tag size="small" :type="inspectSSE.stopReason === 'end_turn' || inspectSSE.stopReason === 'stop' ? 'success' : 'warning'">
                {{ inspectSSE.stopReason }}
              </el-tag>
            </div>
          </template>

          <!-- Non-stream JSON response (original logic) -->
          <template v-else-if="inspectParsed">
            <!-- Error -->
            <div v-if="inspectParsed.error" class="mb-5">
              <div class="section-label">Error</div>
              <div class="error-block"><pre class="whitespace-pre-wrap">{{ formatJson(inspectParsed.error) }}</pre></div>
            </div>
            <!-- Content -->
            <div v-if="getContent(inspectParsed).length" class="mb-5">
              <div class="section-label">Response Content</div>
              <div class="space-y-2">
                <template v-for="(block, bi) in getContent(inspectParsed)" :key="bi">
                  <div v-if="block.type === 'text' && block.text" class="content-text-block">{{ block.text }}</div>
                  <div v-else-if="block.type === 'thinking'" class="thinking-block">
                    <div class="text-xs font-semibold text-purple-600 mb-1">Thinking</div>
                    <div class="text-sm text-gray-600 whitespace-pre-wrap max-h-40 overflow-auto">{{ block.thinking }}</div>
                  </div>
                  <div v-else-if="block.type === 'tool_use'" class="tool-use-block">
                    <div class="text-xs font-semibold text-purple-700 mb-1">Tool: {{ block.name }}</div>
                    <pre class="text-xs text-gray-600 whitespace-pre-wrap overflow-auto max-h-40">{{ formatJson(block.input) }}</pre>
                  </div>
                </template>
              </div>
            </div>
            <!-- Stop Reason -->
            <div v-if="getStopReason(inspectParsed)" class="mb-5">
              <el-tag size="small" :type="getStopReason(inspectParsed) === 'end_turn' || getStopReason(inspectParsed) === 'stop' ? 'success' : 'warning'">
                {{ getStopReason(inspectParsed) }}
              </el-tag>
            </div>
            <!-- Usage -->
            <div v-if="getUsage(inspectParsed)" class="mb-5">
              <div class="section-label">Token Usage</div>
              <div class="flex flex-wrap gap-3">
                <div class="usage-card usage-input">
                  <div class="usage-value">{{ getUsage(inspectParsed)!.input_tokens?.toLocaleString() }}</div>
                  <div class="usage-label">Input</div>
                </div>
                <div class="usage-card usage-output">
                  <div class="usage-value">{{ getUsage(inspectParsed)!.output_tokens?.toLocaleString() }}</div>
                  <div class="usage-label">Output</div>
                </div>
                <div v-if="getUsage(inspectParsed)!.cache_read_tokens" class="usage-card usage-cache-read">
                  <div class="usage-value">{{ getUsage(inspectParsed)!.cache_read_tokens?.toLocaleString() }}</div>
                  <div class="usage-label">Cache Read</div>
                </div>
                <div v-if="getUsage(inspectParsed)!.cache_creation_tokens" class="usage-card usage-cache-create">
                  <div class="usage-value">{{ getUsage(inspectParsed)!.cache_creation_tokens?.toLocaleString() }}</div>
                  <div class="usage-label">Cache Create</div>
                </div>
              </div>
            </div>
          </template>
        </template>
      </template>
    </el-drawer>

    <!-- Diff Dialog -->
    <el-dialog v-model="diffOpen" fullscreen :show-close="true">
      <template #header>
        <span class="text-lg font-semibold text-gray-800">{{ t('traces.detail.diff') }}</span>
      </template>
      <div v-for="(pair, pi) in diffPairs" :key="pi" class="mb-8">
        <h4 class="text-sm font-semibold text-gray-600 mb-3 pb-2 border-b border-gray-200">{{ pair.label }}</h4>
        <el-tabs>
          <el-tab-pane label="Headers">
            <div class="diff-table">
              <div class="diff-row diff-row-head">
                <div class="diff-key">Key</div>
                <div class="diff-left">{{ pair.left.type }}</div>
                <div class="diff-right">{{ pair.right.type }}</div>
              </div>
              <template v-for="h in diffHeaderKeys(pair.left, pair.right)" :key="h.key">
                <div v-if="h.type !== 'equal'" class="diff-row" :class="'diff-' + h.type">
                  <div class="diff-key font-mono text-xs">{{ h.key }}</div>
                  <div class="diff-left font-mono text-xs">{{ h.left ?? '—' }}</div>
                  <div class="diff-right font-mono text-xs">{{ h.right ?? '—' }}</div>
                </div>
              </template>
            </div>
          </el-tab-pane>
          <el-tab-pane label="Body">
            <div class="diff-body-view">
              <div class="diff-body-col diff-body-left">
                <div class="diff-col-label">{{ pair.left.type }}</div>
                <pre class="diff-pre">{{ formatBody(pair.left.body) }}</pre>
              </div>
              <div class="diff-body-col diff-body-right">
                <div class="diff-col-label">{{ pair.right.type }}</div>
                <pre class="diff-pre">{{ formatBody(pair.right.body) }}</pre>
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
/* Main page styles */
</style>

<style>
/* Inspect drawer header */
.el-drawer__header {
  margin-bottom: 0;
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
}
.inspect-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
}
.inspect-title {
  font-size: 16px;
  font-weight: 600;
  color: #1e293b;
}
.inspect-meta {
  font-size: 12px;
  color: #94a3b8;
}

/* Drawer content styles */
.el-drawer__body .section-label {
  font-size: 0.75rem;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.5rem;
}
.el-drawer__body .sys-block {
  background: #fffbeb;
  border: 1px solid #fde68a;
  border-radius: 0.5rem;
  padding: 0.75rem;
  font-size: 0.875rem;
  color: #374151;
  white-space: pre-wrap;
  max-height: 15rem;
  overflow: auto;
}
.el-drawer__body .msg-details {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
}
.el-drawer__body .msg-summary {
  padding: 0.5rem 0.75rem;
  cursor: pointer;
  display: flex;
  align-items: center;
}
.el-drawer__body .msg-summary:hover {
  background: #f9fafb;
  border-radius: 0.5rem;
}
.el-drawer__body .msg-body {
  padding: 0.75rem;
  border-top: 1px solid #f3f4f6;
}
.el-drawer__body .tool-call-block {
  background: #fff;
  border-radius: 0.25rem;
  padding: 0.5rem;
  font-size: 0.75rem;
  border: 1px solid #e9d5ff;
}
.el-drawer__body .tool-def-block {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
}
.el-drawer__body .code-block {
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
  padding: 0.75rem;
  font-size: 0.875rem;
  color: #374151;
  white-space: pre-wrap;
  max-height: 15rem;
  overflow: auto;
}
.el-drawer__body .error-block {
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 0.5rem;
  padding: 0.75rem;
  font-size: 0.875rem;
  color: #b91c1c;
}
.el-drawer__body .content-text-block {
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
  padding: 0.75rem;
  font-size: 0.875rem;
  color: #374151;
  white-space: pre-wrap;
  max-height: 20rem;
  overflow: auto;
}
.el-drawer__body .thinking-block {
  background: #faf5ff;
  border: 1px solid #e9d5ff;
  border-radius: 0.5rem;
  padding: 0.75rem;
}
.el-drawer__body .tool-use-block {
  background: #faf5ff;
  border: 1px solid #e9d5ff;
  border-radius: 0.5rem;
  padding: 0.75rem;
}
.el-drawer__body .usage-card {
  border-radius: 0.5rem;
  padding: 0.75rem 1rem;
  text-align: center;
  min-width: 7rem;
}
.el-drawer__body .usage-value {
  font-size: 1.125rem;
  font-weight: 700;
}
.el-drawer__body .usage-label {
  font-size: 0.75rem;
  margin-top: 0.125rem;
}
.el-drawer__body .usage-input {
  background: #eff6ff;
  border: 1px solid #bfdbfe;
}
.el-drawer__body .usage-input .usage-value { color: #1d4ed8; }
.el-drawer__body .usage-input .usage-label { color: #3b82f6; }
.el-drawer__body .usage-output {
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
}
.el-drawer__body .usage-output .usage-value { color: #15803d; }
.el-drawer__body .usage-output .usage-label { color: #22c55e; }
.el-drawer__body .usage-cache-read {
  background: #ecfeff;
  border: 1px solid #a5f3fc;
}
.el-drawer__body .usage-cache-read .usage-value { color: #0e7490; }
.el-drawer__body .usage-cache-read .usage-label { color: #06b6d4; }
.el-drawer__body .usage-cache-create {
  background: #fffbeb;
  border: 1px solid #fde68a;
}
.el-drawer__body .usage-cache-create .usage-value { color: #b45309; }
.el-drawer__body .usage-cache-create .usage-label { color: #f59e0b; }

/* Diff dialog styles */
.diff-table {
  width: 100%;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
  overflow: hidden;
  font-size: 0.8125rem;
}
.diff-row {
  display: grid;
  grid-template-columns: 200px 1fr 1fr;
  border-bottom: 1px solid #f3f4f6;
}
.diff-row:last-child { border-bottom: none; }
.diff-row-head {
  background: #f9fafb;
  font-weight: 600;
  color: #64748b;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.diff-key, .diff-left, .diff-right {
  padding: 6px 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.diff-left { border-right: 1px solid #f3f4f6; }
.diff-add .diff-right { background: #f0fdf4; color: #166534; }
.diff-remove .diff-left { background: #fef2f2; color: #991b1b; }
.diff-change .diff-left { background: #fef2f2; color: #991b1b; }
.diff-change .diff-right { background: #f0fdf4; color: #166534; }
.diff-body-view {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  height: calc(100vh - 260px);
}
.diff-body-col {
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
  overflow: auto;
  display: flex;
  flex-direction: column;
}
.diff-col-label {
  padding: 8px 12px;
  font-size: 0.75rem;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  background: #f9fafb;
  border-bottom: 1px solid #e5e7eb;
  flex-shrink: 0;
}
.diff-pre {
  padding: 12px;
  font-size: 0.75rem;
  line-height: 1.5;
  margin: 0;
  flex: 1;
}
</style>
