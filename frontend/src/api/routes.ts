import client from './client'

export interface Route {
  key: string
  provider: string
  default_model: string
  scene_map: Record<string, string>
  model_map: Record<string, string>
  long_context_threshold: number
}

export const listRoutes = () => client.get<{ data: Route[] }>('/admin/routes')
export const createRoute = (data: Partial<Route> & { key: string; provider: string }) => client.post<{ data: { key: string }; warnings?: string[] }>('/admin/routes', data)
export const updateRoute = (key: string, data: Partial<Route>) => client.put<{ warnings?: string[] }>(`/admin/routes/${key}`, data)
export const deleteRoute = (key: string) => client.delete(`/admin/routes/${key}`)
export const generateKey = () => client.post<{ data: { key: string } }>('/admin/routes/generate-key')
export const revealRouteKey = (key: string) => client.get(`/admin/apikeys/route/${key}?reveal=true`)
