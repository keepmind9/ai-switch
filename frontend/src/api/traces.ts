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

export interface TraceListResponse {
  items: TraceItem[]
  has_prev: boolean
  has_next: boolean
  prev_cursor: string
  next_cursor: string
}

export interface TraceDetail {
  ais_req_id: string
  records: TraceDetailRecord[]
}

export const getTraces = async (params: {
  start_time?: string
  end_time?: string
  model?: string
  provider?: string
  status?: number
  session_id?: string
  cursor?: string
  page_size?: number
}): Promise<TraceListResponse> => {
  const { data } = await client.get<TraceListResponse>('/admin/traces', { params })
  return data
}

export const getTraceDetail = async (ais_req_id: string): Promise<TraceDetail> => {
  const { data } = await client.get<TraceDetail>(`/admin/traces/${ais_req_id}`)
  return data
}
