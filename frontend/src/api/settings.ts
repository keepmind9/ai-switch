import client from './client'

export interface Settings {
  host: string
  port: number
  allowed_ips: string[]
  log_retention_days: number
  proxy_url: string
}

export interface ConfigBackup {
  name: string
  path: string
  size: number
  mod_time: string
}

export const getSettings = () => client.get<Settings>('/admin/settings')

export const updateSettings = (data: Partial<Settings>) => client.put<Settings>('/admin/settings', data)

export const restartServer = () => client.post<{ url: string }>('/admin/restart')

export const stopServer = () => client.post('/admin/stop')

export const listConfigBackups = () => client.get<ConfigBackup[]>('/admin/config/backups')

export const restoreConfigBackup = (name: string) => client.post<{ name: string }>('/admin/config/backups/restore', { name })

export const cleanConfigBackups = (keep: number) => client.post<{ keep: number }>('/admin/config/backups/clean', { keep })
