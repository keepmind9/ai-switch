import type { AxiosInstance, AxiosRequestConfig } from "axios"
import axios from "axios"
import { get, merge } from "lodash-es"

function createInstance() {
  const instance = axios.create()
  instance.interceptors.request.use(
    config => config,
    error => Promise.reject(error)
  )
  instance.interceptors.response.use(
    (response) => response,
    (error) => {
      const status = get(error, "response.status")
      const message = get(error, "response.data.message")
      switch (status) {
        case 400: error.message = message || "Bad request"; break
        case 403: error.message = message || "Forbidden"; break
        case 404: error.message = "Not found"; break
        case 500: error.message = "Internal server error"; break
        case 502: error.message = "Bad gateway"; break
        case 503: error.message = "Service unavailable"; break
        default: error.message = message || `Error ${status}`
      }
      ElMessage.error(error.message)
      return Promise.reject(error)
    }
  )
  return instance
}

function createRequest(instance: AxiosInstance) {
  return <T>(config: AxiosRequestConfig): Promise<T> => {
    const defaultConfig: AxiosRequestConfig = {
      baseURL: import.meta.env.VITE_BASE_URL,
      headers: { "Content-Type": "application/json" },
      data: {},
      timeout: 5000,
      withCredentials: false
    }
    const mergeConfig = merge(defaultConfig, config)
    return instance(mergeConfig)
  }
}

const instance = createInstance()
export const request = createRequest(instance)
