import client from './client'

export interface Settings {
  host: string
  port: number
  allowed_ips: string[]
  log_retention_days: number
}

export const getSettings = () => client.get<Settings>('/admin/settings')

export const updateSettings = (data: Partial<Settings>) => client.put<Settings>('/admin/settings', data)

export const restartServer = () => client.post<{ url: string }>('/admin/restart')

export const stopServer = () => client.post('/admin/stop')
