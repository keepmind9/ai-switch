import client from './client'

export interface UsageRecord {
  provider: string
  model: string
  date: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
}

export interface Preset {
  key: string
  name: string
  base_url: string
  format: string
  logo_url: string
}

export const queryStats = (params?: { provider?: string; model?: string; start_date?: string; end_date?: string }) =>
  client.get<{ data: UsageRecord[] }>('/stats', { params })

export const listPresets = () => client.get<{ data: Preset[] }>('/admin/presets')
export const getAdminStatus = () => client.get('/admin/status')
