import client from './client'

// `default_model` is NOT returned by the backend (ProviderConfig has no such
// field); it is request-only and used to initialize the auto-created Route.
export interface Provider {
  key: string
  name: string
  base_url: string
  path: string
  api_key: string
  fallback_keys: string[]
  format: string
  logo_url: string
  think_tag: string
  models: string[]
  enable_proxy: boolean
}

export interface CreateProviderRequest {
  key: string
  name: string
  base_url: string
  api_key: string
  path?: string
  format?: string
  logo_url?: string
  think_tag?: string
  fallback_keys?: string[]
  models?: string[]
  default_model?: string
  enable_proxy?: boolean
}

export interface ModelInfo {
  id: string
  name: string
}

export const listProviders = () => client.get<Provider[]>('/admin/providers')
export const createProvider = (data: CreateProviderRequest) => client.post<{ key: string; name: string; auto_route_created: boolean; warnings?: string[] }>('/admin/providers', data)
export const updateProvider = (key: string, data: Partial<Provider>) => client.put(`/admin/providers/${key}`, data)
export const deleteProvider = (key: string) => client.delete(`/admin/providers/${key}`)
export const revealAPIKey = (key: string) => client.get<{ api_key: string }>(`/admin/apikeys/provider/${key}?reveal=true`)
export const fetchModels = (data: { base_url: string; api_key: string; format: string }) => client.post<ModelInfo[]>('/admin/providers/fetch-models', data)
