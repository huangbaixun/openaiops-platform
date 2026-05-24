import axios from 'axios'

const client = axios.create({
  baseURL: import.meta.env.VITE_API_BASE ?? '/api',
  timeout: 10000,
})

client.interceptors.request.use((cfg) => {
  const key = localStorage.getItem('apiKey')
  if (key) cfg.headers.Authorization = `Bearer ${key}`
  return cfg
})

export default client
