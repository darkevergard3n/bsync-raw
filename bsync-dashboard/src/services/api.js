import axios from 'axios';

// Create axios instance for SyncTool server
const api = axios.create({
  baseURL: `https://192.168.50.157:443`, // SyncTool server
  timeout: 60000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor - add auth token automatically
api.interceptors.request.use(
  (config) => {
    console.log('[API] Making SyncTool request:', config.method?.toUpperCase(), config.url);
    console.log('[API] Request config:', {
      baseURL: config.baseURL,
      url: config.url,
      method: config.method,
      data: config.data
    });

    // Add authorization token from localStorage if available
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }

    return config;
  },
  (error) => {
    console.error('[API] Request interceptor error:', error);
    return Promise.reject(error);
  }
);

// Response interceptor - handle auth errors
api.interceptors.response.use(
  (response) => {
    console.log('[API] SyncTool response received:', response.status, response.config.url);
    console.log('[API] Response data:', response.data);
    return response;
  },
  (error) => {
    console.error('[API] SyncTool response error:', {
      status: error.response?.status,
      statusText: error.response?.statusText,
      url: error.config?.url,
      method: error.config?.method,
      data: error.response?.data,
      message: error.message
    });

    // Handle 401 - Unauthorized (token expired or invalid)
    if (error.response?.status === 401) {
      console.log('[API] Unauthorized - redirecting to login');
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/login';
    }

    return Promise.reject(error);
  }
);

// For development - use mock data when SyncTool server is not available
const USE_MOCK_DATA = false; // SyncTool server is running on port 8090

console.log('[API] Initializing API service');
console.log('[API] Base URL:', api.defaults.baseURL);
console.log('[API] Using mock data:', USE_MOCK_DATA);
console.log('[API] API service ready at:', new Date().toISOString());

// Mock data
const mockAgents = [
  {
    id: '1',
    device_id: 'DESKTOP-01-AB3DEF',
    hostname: 'desktop-01',
    ip_address: '192.168.1.100',
    os: 'Windows 11',
    architecture: 'x64',
    version: '1.0.0',
    status: 'online',
    approval_status: 'approved',
    last_heartbeat: new Date().toISOString(),
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
  {
    id: '2',
    device_id: 'SERVER-02-CD5GHI',
    hostname: 'test-laptop-01',
    ip_address: '192.168.1.101',
    os: 'Ubuntu 22.04',
    architecture: 'x64',
    version: '1.0.0',
    status: 'online',
    approval_status: 'pending',
    last_heartbeat: new Date().toISOString(),
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
  {
    id: '3',
    device_id: 'LAPTOP-03-JK7LMN',
    hostname: 'laptop-03',
    ip_address: '192.168.1.102',
    os: 'macOS Sonoma',
    architecture: 'arm64',
    version: '1.0.0',
    status: 'offline',
    approval_status: 'approved',
    last_heartbeat: new Date(Date.now() - 3600000).toISOString(),
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
];

const mockFolders = [
  {
    id: '1',
    name: 'Documents',
    path: '/home/user/Documents',
    sync_type: 'sendreceive',
    rescan_interval: 3600,
    max_file_size: 104857600,
    ignore_patterns: ['*.tmp', '.DS_Store'],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
  {
    id: '2',
    name: 'Backup',
    path: '/backup/data',
    sync_type: 'receiveonly',
    rescan_interval: 7200,
    max_file_size: 0,
    ignore_patterns: [],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
];

// Mock API wrapper
const mockAPI = {
  get: (url) => {
    console.log(`[MOCK] GET ${url}`);
    
    if (url.includes('/agents')) {
      return Promise.resolve({ 
        data: { 
          data: mockAgents,
          total: mockAgents.length,
          limit: 20,
          offset: 0 
        } 
      });
    }
    
    if (url.includes('/folders')) {
      return Promise.resolve({ 
        data: { 
          data: mockFolders,
          total: mockFolders.length,
          limit: 20,
          offset: 0
        } 
      });
    }
    
    if (url.includes('/metrics')) {
      return Promise.resolve({
        data: {
          total_agents: mockAgents.length,
          online_agents: mockAgents.filter(a => a.status === 'online').length,
          offline_agents: mockAgents.filter(a => a.status === 'offline').length,
          total_folders: mockFolders.length,
          syncing_folders: 1,
          idle_folders: mockFolders.length - 1,
          error_folders: 0,
          data_transferred_24h: 1024 * 1024 * 256, // 256MB
          sync_operations_24h: 42,
          avg_sync_time: 12.5,
        }
      });
    }
    
    return Promise.reject(new Error('Mock endpoint not found'));
  },
  
  post: (url, data) => {
    console.log(`[MOCK] POST ${url}`, data);
    
    if (url.includes('/auth/login')) {
      return Promise.resolve({
        data: {
          token: 'mock-jwt-token-' + Date.now(),
          expires_at: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
          user: {
            id: '1',
            username: data.username || 'admin',
            email: 'admin@synctool.local',
            role: 'admin'
          }
        }
      });
    }

    if (url.includes('/approve')) {
      const agentId = url.split('/')[3]; // Extract agent ID from URL
      const agent = mockAgents.find(a => a.id === agentId);
      if (agent) {
        agent.approval_status = 'approved';
        console.log(`[MOCK] Agent ${agentId} approved`);
      }
      return Promise.resolve({ data: { success: true, message: 'Agent approved successfully' } });
    }

    if (url.includes('/reject')) {
      const agentId = url.split('/')[3]; // Extract agent ID from URL
      const agent = mockAgents.find(a => a.id === agentId);
      if (agent) {
        agent.approval_status = 'rejected';
        console.log(`[MOCK] Agent ${agentId} rejected`);
      }
      return Promise.resolve({ data: { success: true, message: 'Agent rejected successfully' } });
    }
    
    return Promise.resolve({ data: { success: true } });
  },
  
  put: (url, data) => {
    console.log(`[MOCK] PUT ${url}`, data);
    return Promise.resolve({ data: { success: true } });
  },
  
  delete: (url) => {
    console.log(`[MOCK] DELETE ${url}`);
    return Promise.resolve({ data: { success: true } });
  },
  
  defaults: {
    headers: {
      common: {}
    }
  }
};

// Export the API instance (use mock in development)
const apiInstance = USE_MOCK_DATA ? mockAPI : api;

export default apiInstance;

// Named exports for SyncTool API
export const agentAPI = {
  list: async () => {
    console.log('[AgentAPI] Fetching agents using hybrid strategy...');
    
    try {
      // Step 1: Get database agents (persistent data)
      console.log('[AgentAPI] Fetching from database...');
      const dbResponse = await apiInstance.get('/api/integrated-agents');
      const dbAgents = dbResponse.data.data || [];
      console.log('[AgentAPI] Database agents:', dbAgents.length);
      
      // Step 2: Get live agents (real-time status)
      console.log('[AgentAPI] Fetching live status...');
      const liveResponse = await apiInstance.get('/api/agents');
      const liveAgents = liveResponse.data || {};
      console.log('[AgentAPI] Live agents:', Object.keys(liveAgents).length);
      
      // Step 3: Merge all agents from both sources
      const allAgentIds = new Set([
        ...dbAgents.map(agent => agent.agent_id),
        ...Object.keys(liveAgents)
      ]);

      const hybridAgents = Array.from(allAgentIds).map(agentId => {
        const dbAgent = dbAgents.find(agent => agent.agent_id === agentId);
        const liveAgent = liveAgents[agentId];
        
        if (dbAgent && liveAgent) {
          // Agent exists in both sources - use hybrid data
          // Choose the most recent heartbeat between DB and live data
          const dbHeartbeat = new Date(dbAgent.last_heartbeat);
          const liveLastSeen = new Date(liveAgent.last_seen);
          const mostRecentHeartbeat = dbHeartbeat > liveLastSeen ? dbAgent.last_heartbeat : liveAgent.last_seen;
          
          return {
            ...dbAgent,
            id: dbAgent.agent_id,
            status: liveAgent.connected ? 'online' : 'offline',
            connected: liveAgent.connected,
            last_heartbeat: mostRecentHeartbeat,
            ip_address: liveAgent.remote_addr?.split(':')[0] || dbAgent.ip_address,
            os: liveAgent.os !== 'Unknown' ? liveAgent.os : dbAgent.os,
            architecture: liveAgent.architecture !== 'Unknown' ? liveAgent.architecture : dbAgent.architecture,
            data_dir: liveAgent.data_dir || dbAgent.data_dir,
            is_live: true,
            version: '1.0.0',
            approval_status: dbAgent.approval_status || 'approved',
            created_at: dbAgent.created_at || liveAgent.last_seen,
            updated_at: new Date().toISOString(),
            is_fallback: false
          };
        } else if (dbAgent) {
          // Agent only in database - mark as offline
          console.log(`[AgentAPI] Agent ${agentId} found only in database - marking as offline`);
          return {
            ...dbAgent,
            id: dbAgent.agent_id,
            status: 'offline',
            connected: false,
            last_heartbeat: dbAgent.last_heartbeat,
            ip_address: dbAgent.ip_address,
            os: dbAgent.os || 'Unknown',
            architecture: dbAgent.architecture || 'Unknown',
            data_dir: dbAgent.data_dir,
            is_live: false,
            version: '1.0.0',
            approval_status: dbAgent.approval_status || 'approved',
            created_at: dbAgent.created_at,
            updated_at: new Date().toISOString(),
            is_fallback: true
          };
        } else if (liveAgent) {
          // Agent only in live data - create basic record
          console.log(`[AgentAPI] Agent ${agentId} found only in live data - creating basic record`);
          return {
            id: agentId,
            agent_id: agentId,
            hostname: liveAgent.hostname || agentId,
            device_id: liveAgent.device_id || '',
            status: liveAgent.connected ? 'online' : 'offline',
            connected: liveAgent.connected,
            last_heartbeat: liveAgent.last_seen,
            ip_address: liveAgent.remote_addr?.split(':')[0] || '',
            os: liveAgent.os || 'Unknown',
            architecture: liveAgent.architecture || 'Unknown',
            data_dir: liveAgent.data_dir || '',
            is_live: true,
            version: '1.0.0',
            approval_status: 'pending', // New agents default to pending
            created_at: liveAgent.last_seen,
            updated_at: new Date().toISOString(),
            is_fallback: false
          };
        }
      }).filter(Boolean);

      console.log('[AgentAPI] Final hybrid agents:', hybridAgents.length);
      return { 
        data: {
          data: hybridAgents,
          total: hybridAgents.length
        }
      };
    } catch (error) {
      console.error('[AgentAPI] Error fetching agents:', error);
      return { data: { data: [], total: 0 } };
    }
  },
  get: (id) => {
    console.log('[AgentAPI] Fetching agent details for ID:', id);
    return apiInstance.get(`/api/agents/${id}`);
  },
  approve: (id, reason) => {
    console.log('[AgentAPI] Approving agent:', id, 'with reason:', reason);
    return apiInstance.post(`/api/agents/${id}/approve`, { reason });
  },
  reject: (id, reason) => {
    console.log('[AgentAPI] Rejecting agent:', id, 'with reason:', reason);
    return apiInstance.post(`/api/agents/${id}/reject`, { reason });
  },
  restart: (id) => {
    console.log('[AgentAPI] Restarting agent:', id);
    return apiInstance.post(`/api/agents/${id}/restart`);
  },
  delete: (id) => {
    console.log('[AgentAPI] Deleting agent:', id);
    return apiInstance.post(`/api/agents/${id}/delete`);
  },
  browseFolders: (id, params) => {
    console.log('[AgentAPI] Browsing folders for agent:', id, 'params:', params);
    return apiInstance.get(`/api/agents/${id}/browse`, { params });
  },
};

// License Management API
export const licenseAPI = {
  // License operations
  list: () => apiInstance.get('/api/v1/licenses'),
  get: (id) => apiInstance.get(`/api/v1/licenses/${id}`),
  create: (data) => apiInstance.post('/api/v1/licenses', data),
  update: (id, data) => apiInstance.put(`/api/v1/licenses/${id}`, data),
  delete: (id) => apiInstance.delete(`/api/v1/licenses/${id}`),
};

// Agent-License Mapping API
export const agentLicenseAPI = {
  // Get all agent-license mappings
  list: () => apiInstance.get('/api/v1/agent-licenses'),
  
  // Get mapping for a specific agent
  getByAgent: (agentId) => apiInstance.get(`/api/v1/agent-licenses/${agentId}`),
  
  // Create or update agent-license mapping
  assign: (data) => apiInstance.post('/api/v1/agent-licenses', data),
  
  // Remove license from agent
  unassign: (agentId) => apiInstance.delete(`/api/v1/agent-licenses/${agentId}`)
};

// Unlicensed Agents API
export const unlicensedAgentsAPI = {
  list: () => apiInstance.get('/api/v1/agents/unlicensed')
};

export const folderAPI = {
  list: () => apiInstance.get('/folders'),
  get: (id) => apiInstance.get(`/folders/${id}`),
  create: (data) => apiInstance.post('/folders', data),
  update: (id, data) => apiInstance.put(`/folders/${id}`, data),
  delete: (id) => apiInstance.delete(`/folders/${id}`),
  assign: (id, agentId) => apiInstance.post(`/folders/${id}/assign`, { agent_id: agentId }),
};

export const reportsAPI = {
  // Legacy API - keep for now during migration
  getFileTransferLogs: (params) => {
    console.log('[ReportsAPI] Fetching file transfer logs with params:', params);
    return api.get('/api/v1/file-transfer-logs', { params });
  },
  
  // New separated APIs
  getTransferStats: (filters = {}) => {
    console.log('[ReportsAPI] Fetching transfer stats with filters:', filters);
    return api.get('/api/v1/reports/transfer-stats', { params: filters });
  },
  
  getFilterOptions: (dateRange = {}) => {
    console.log('[ReportsAPI] Fetching filter options with date range:', dateRange);
    return api.get('/api/v1/reports/filter-options', { params: dateRange });
  },
  
  getJobsList: () => {
    console.log('[ReportsAPI] Fetching jobs list');
    return api.get('/api/v1/reports/jobs');
  },
};

export const authAPI = {
  login: (credentials) => apiInstance.post('/auth/login', credentials),
  logout: () => apiInstance.post('/auth/logout'),
  profile: () => apiInstance.get('/auth/profile'),
  changePassword: (data) => apiInstance.post('/auth/change-password', data),
};

// Generic fetchAPI for backward compatibility
export const fetchAPI = async (endpoint, options = {}) => {
  try {
    const response = await apiInstance.get(endpoint, options);
    return response.data;
  } catch (error) {
    console.error(`[fetchAPI] Error fetching ${endpoint}:`, error);
    throw error;
  }
};

// Jobs API
export const jobsAPI = {
  list: async () => {
    const response = await apiInstance.get('/api/jobs');
    return response.data;
  },
  updateName: (jobId, newName) => {
    return apiInstance.put(`/api/jobs/${jobId}/name`, { name: newName });
  }
};

// Folder Stats API
export const folderStatsAPI = {
  get: (agentId, folderId) => {
    return apiInstance.get(`/api/folder-stats?agent_id=${agentId}&folder_id=${folderId}`);
  }
};