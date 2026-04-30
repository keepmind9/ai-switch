import axios from 'axios'

const client = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

client.interceptors.response.use(
  (resp) => {
    const body = resp.data
    // Admin API responses have unified envelope
    if (body && typeof body === 'object' && 'code' in body) {
      if (body.code !== 0) {
        return Promise.reject(new Error(body.msg || 'Unknown error'))
      }
      resp.data = body.data
    }
    return resp
  },
  (err) => {
    const body = err.response?.data
    if (body && typeof body === 'object' && 'msg' in body) {
      err.message = body.msg
    }
    return Promise.reject(err)
  },
)

export default client
