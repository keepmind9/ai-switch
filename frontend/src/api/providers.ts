import client from './client'

export interface Provider {
  key: string
  name: string
  base_url: string
  path: string
  api_key: string
  format: string
  logo_url: string
  sponsor: boolean
  think_tag: string
  default_model: string
  models: string[]
}

export interface ModelInfo {
  id: string
  name: string
}

export const listProviders = () => client.get<{ data: Provider[] }>('/admin/providers')
export const createProvider = (data: Partial<Provider> & { key: string; name: string; base_url: string; api_key: string }) => client.post('/admin/providers', data)
export const updateProvider = (key: string, data: Partial<Provider>) => client.put(`/admin/providers/${key}`, data)
export const deleteProvider = (key: string) => client.delete(`/admin/providers/${key}`)
export const revealAPIKey = (key: string) => client.get(`/admin/apikeys/provider/${key}?reveal=true`)
export const fetchModels = (data: { base_url: string; api_key: string; format: string }) => client.post<{ models: ModelInfo[] }>('/admin/providers/fetch-models', data)
