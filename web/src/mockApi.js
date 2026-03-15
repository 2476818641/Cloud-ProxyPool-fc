const DEMO_TOKEN = 'demo-jwt-token';
const CONFIG_KEY = 'cloud_proxy_demo_config';
const NODES_KEY = 'cloud_proxy_demo_nodes';

function nowIso() {
  return new Date().toISOString();
}

function makeResponse(data) {
  return Promise.resolve({ data });
}

function readStorage(key, fallback) {
  try {
    const raw = localStorage.getItem(key);
    return raw ? JSON.parse(raw) : fallback;
  } catch {
    return fallback;
  }
}

function writeStorage(key, value) {
  localStorage.setItem(key, JSON.stringify(value));
}

function defaultConfig() {
  return {
    listenAddr: '0.0.0.0:10800',
    socksAddr: '0.0.0.0:10801',
    user: 'demo',
    password: 'demo12345',
    dashboardEnabled: true,
    users: [{ username: 'demo', password: 'demo12345' }]
  };
}

function defaultNodes() {
  return [
    makeNode('https://cloud-proxy-pool.cn-shanghai.fc.aliyuncs.com', 'cn-shanghai', 'Healthy', 23, 68, true),
    makeNode('https://cloud-proxy-pool.cn-shenzhen.fc.aliyuncs.com', 'cn-shenzhen', 'Healthy', 4, 92, true),
    makeNode('https://cloud-proxy-pool.ap-southeast-1.fc.aliyuncs.com', 'ap-southeast-1', 'Healthy', 1, 131, true),
    makeNode('https://cloud-proxy-pool.us-west-1.fc.aliyuncs.com', 'us-west-1', 'Melting (Fused)', 7, 219, false)
  ];
}

function makeNode(url, region, status, failureCount, latency, enabled) {
  return {
    url,
    region,
    failure_count: failureCount,
    last_fail: status === 'Healthy' ? '' : '14:36:12',
    status,
    latency,
    enabled
  };
}

function ensureConfig() {
  const config = readStorage(CONFIG_KEY, null) || defaultConfig();
  writeStorage(CONFIG_KEY, config);
  return config;
}

function ensureNodes() {
  const nodes = readStorage(NODES_KEY, null) || defaultNodes();
  writeStorage(NODES_KEY, nodes);
  return nodes;
}

function generateHistory() {
  const now = Date.now();
  return Array.from({ length: 24 }, (_, index) => {
    const factor = index + 1;
    return {
      time: new Date(now - (23 - index) * 5000).toLocaleTimeString('zh-CN', { hour12: false }),
      qps: Number((4 + Math.sin(factor / 2) * 1.8 + factor * 0.08).toFixed(2)),
      avgLatency: 75 + (factor % 4) * 8,
      p95Latency: 120 + (factor % 5) * 13,
      p99Latency: 180 + (factor % 6) * 15
    };
  });
}

function generateLogs() {
  const now = Date.now();
  return Array.from({ length: 48 }, (_, index) => {
    const status = index % 11 === 0 ? 502 : index % 7 === 0 ? 302 : 200;
    const level = status >= 400 ? 'error' : status >= 300 ? 'warn' : 'info';
    const minutesAgo = 47 - index;
    return {
      time: new Date(now - minutesAgo * 60 * 1000).toLocaleTimeString('zh-CN', { hour12: false }),
      level,
      method: index % 3 === 0 ? 'POST' : 'GET',
      url: index % 4 === 0 ? 'https://api.openai.com/v1/models' : 'https://example.com/health',
      status,
      duration: 60 + (index % 9) * 17,
      error: status >= 400 ? 'Function invoke failed: upstream timeout' : ''
    };
  });
}

function filterLogs(logs, params = {}) {
  let result = [...logs];
  if (params.status) {
    result = result.filter((entry) => {
      if (params.status === 'success') return entry.status >= 200 && entry.status < 400;
      if (params.status === 'error') return entry.status >= 400;
      if (params.status === 'warn') return entry.level === 'warn' || entry.status >= 400;
      return true;
    });
  }

  if (params.timeRange) {
    const ranges = { '1h': 60, '6h': 360, '24h': 1440, '7d': 10080 };
    const limitMinutes = ranges[params.timeRange];
    if (limitMinutes) {
      result = result.slice(-Math.min(result.length, Math.max(1, Math.floor(limitMinutes / 15))));
    }
  }

  return result;
}

export function createDemoApis() {
  ensureConfig();
  ensureNodes();
  const history = generateHistory();
  const logs = generateLogs();

  return {
    authAPI: {
      login: async (username, password) => {
        const hashedDemoPassword = await hashDemoPassword('demo12345');
        if (username !== 'demo' || (password !== 'demo12345' && password !== hashedDemoPassword)) {
          const error = new Error('Invalid credentials');
          error.response = { data: { error: 'Demo 账号: demo / demo12345' } };
          throw error;
        }
        return { token: DEMO_TOKEN };
      },
      logout: () => {
        localStorage.removeItem('jwt_token');
      },
      getToken: () => localStorage.getItem('jwt_token'),
      isAuthenticated: () => !!localStorage.getItem('jwt_token')
    },
    statsAPI: {
      getStats: () => {
        const nodes = ensureNodes();
        return makeResponse({
          total: 12482,
          success: 12197,
          failed: 285,
          nodes
        });
      },
      getHistory: () => makeResponse(history)
    },
    nodesAPI: {
      getNodes: () => makeResponse(ensureNodes()),
      updateNode: (url, enabled) => {
        const nodes = ensureNodes().map((node) => node.url === url ? { ...node, enabled } : node);
        writeStorage(NODES_KEY, nodes);
        return makeResponse({ status: 'ok' });
      },
      testLatency: (url) => {
        const nodes = ensureNodes().map((node) => {
          if (node.url !== url) return node;
          return { ...node, latency: 50 + Math.floor(Math.random() * 180) };
        });
        writeStorage(NODES_KEY, nodes);
        return makeResponse({ latency: 50 + Math.floor(Math.random() * 180), success: true });
      }
    },
    configAPI: {
      getConfig: () => makeResponse(ensureConfig()),
      updateConfig: (data) => {
        writeStorage(CONFIG_KEY, data);
        return makeResponse({ status: 'ok' });
      }
    },
    logsAPI: {
      getLogs: (params) => makeResponse(filterLogs(logs, params))
    }
  };
}

async function hashDemoPassword(password) {
  const encoder = new TextEncoder();
  const data = encoder.encode(password);
  const hashBuffer = await crypto.subtle.digest('SHA-256', data);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hashHex = hashArray.map((value) => value.toString(16).padStart(2, '0')).join('');
  return btoa(hashHex);
}

