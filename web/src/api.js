import axios from 'axios';
import { createDemoApis } from './mockApi';

export const isDemoMode = import.meta.env.VITE_DEMO_MODE === 'true';

const api = axios.create({
  baseURL: '/api',
  timeout: 30000
});

api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('jwt_token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('jwt_token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

const realAuthAPI = {
  login: async (username, password) => {
    const encoder = new TextEncoder();
    const data = encoder.encode(password);
    const hashBuffer = await crypto.subtle.digest('SHA-256', data);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    const hashHex = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
    const hashBase64 = btoa(hashHex);
    const response = await api.post('/login', { username, password: hashBase64 });
    return response.data;
  },
  logout: () => {
    localStorage.removeItem('jwt_token');
  },
  getToken: () => localStorage.getItem('jwt_token'),
  isAuthenticated: () => !!localStorage.getItem('jwt_token')
};

const realStatsAPI = {
  getStats: () => api.get('/stats'),
  getHistory: () => api.get('/history')
};

const realNodesAPI = {
  getNodes: () => api.get('/nodes'),
  updateNode: (url, enabled) => api.put('/nodes', { url, enabled }),
  testLatency: (url) => api.post('/nodes/test', { url })
};

const realConfigAPI = {
  getConfig: () => api.get('/config'),
  updateConfig: (data) => api.put('/config', data)
};

const realLogsAPI = {
  getLogs: (params) => api.get('/logs', { params })
};

const demoApis = isDemoMode ? createDemoApis() : null;

export const authAPI = demoApis?.authAPI || realAuthAPI;
export const statsAPI = demoApis?.statsAPI || realStatsAPI;
export const nodesAPI = demoApis?.nodesAPI || realNodesAPI;
export const configAPI = demoApis?.configAPI || realConfigAPI;
export const logsAPI = demoApis?.logsAPI || realLogsAPI;

export default api;
