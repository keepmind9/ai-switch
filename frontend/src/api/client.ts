import axios from 'axios'

const client = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

client.interceptors.response.use(
  (resp) => resp,
  (err) => {
    const msg = err.response?.data?.error || err.message
    console.error('API error:', msg)
    return Promise.reject(err)
  },
)

export default client
