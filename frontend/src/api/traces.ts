import client from './client'

export interface TraceItem {
  ais_req_id: string
  time: string
  session_id: string
  client_protocol: string
  model: string
  stream: boolean
  provider: string
  status: number
  latency_ms: number
  input_tokens: number
  output_tokens: number
}

export interface TraceListResponse {
  data: {
    items: TraceItem[]
    total: number
    page: number
    page_size: number
  }
}

export interface TraceDetailRecord {
  type: string
  time: string
  session_id?: string
  client_protocol?: string
  model?: string
  stream?: boolean
  upstream_protocol?: string
  provider?: string
  url?: string
  status?: number
  latency_ms?: number
  input_tokens?: number
  output_tokens?: number
  body: string
}

export interface TraceDetail {
  ais_req_id: string
  records: TraceDetailRecord[]
}

export interface TraceDetailResponse {
  data: TraceDetail
}

export const getTraceDates = async (): Promise<string[]> => {
  const { data } = await client.get<{ data: string[] }>('/admin/traces/dates')
  return data.data
}

export const getTraces = async (params: {
  date?: string
  model?: string
  provider?: string
  status?: number
  session_id?: string
  page?: number
  page_size?: number
}): Promise<TraceListResponse> => {
  const { data } = await client.get<TraceListResponse>('/admin/traces', { params })
  return data
}

export const getTraceDetail = async (ais_req_id: string, date?: string): Promise<TraceDetail> => {
  const { data } = await client.get<TraceDetailResponse>(`/admin/traces/${ais_req_id}`, { params: { date } })
  return data.data
}
