// 后端 API 封装:统一 JSON 请求、401 自动跳登录页
async function request(method, url, body) {
  const opts = { method, credentials: 'same-origin', headers: {} }
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(url, opts)
  if (res.status === 401 && !url.includes('/api/login') && !url.includes('/api/auth')) {
    window.location.href = '/login'
    throw new Error('未登录')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error(data.error || `请求失败(${res.status})`)
  }
  return data
}

const enc = encodeURIComponent

function qs(params) {
  const usp = new URLSearchParams()
  for (const [k, v] of Object.entries(params || {})) {
    if (v !== undefined && v !== null && v !== '') usp.set(k, v)
  }
  const s = usp.toString()
  return s ? `?${s}` : ''
}

export const api = {
  authCheck: () => request('GET', '/api/auth'),
  login: password => request('POST', '/api/login', { password }),
  logout: () => request('POST', '/api/logout'),

  listProjects: () => request('GET', '/api/projects'),
  getProject: name => request('GET', `/api/projects/${enc(name)}`),
  createProject: cfg => request('POST', '/api/projects', cfg),
  updateProject: (name, cfg) => request('PUT', `/api/projects/${enc(name)}`, cfg),
  deleteProject: name => request('DELETE', `/api/projects/${enc(name)}`),
  deploy: name => request('POST', `/api/projects/${enc(name)}/deploy`),
  switchBranch: (name, branch) => request('POST', `/api/projects/${enc(name)}/branch`, { branch }),

  projectLogs: (name, params) => request('GET', `/api/projects/${enc(name)}/logs${qs(params)}`),
  systemLogs: params => request('GET', `/api/logs${qs(params)}`)
}
