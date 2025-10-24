#!/bin/bash

# Create all dashboard files
echo "Creating all dashboard files..."

# Create services/api.js
cat > src/services/api.js << 'EOF'
const mockAgents = [
  {
    id: '1',
    device_id: 'DESKTOP-01-AB3DEF',
    hostname: 'desktop-01',
    ip_address: '192.168.1.100',
    os: 'Windows',
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
    hostname: 'server-02',
    ip_address: '192.168.1.101',
    os: 'Linux',
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
    os: 'macOS',
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
    type: 'sendreceive',
    rescan_interval: 3600,
    max_file_size: 104857600,
    ignore_patterns: ['*.tmp', '.DS_Store'],
    agents: [mockAgents[0], mockAgents[2]],
  },
  {
    id: '2',
    name: 'Backup',
    path: '/backup/data',
    type: 'receiveonly',
    rescan_interval: 7200,
    max_file_size: 0,
    ignore_patterns: [],
    agents: [mockAgents[0]],
  },
];

export const agentAPI = {
  list: () => Promise.resolve({ data: mockAgents }),
  get: (id) => Promise.resolve({ data: mockAgents.find(a => a.id === id) }),
  approve: (id) => Promise.resolve({ success: true }),
  reject: (id) => Promise.resolve({ success: true }),
  delete: (id) => Promise.resolve({ success: true }),
  restart: (id) => Promise.resolve({ success: true }),
  browseFolders: (id, params) => Promise.resolve({
    data: {
      path: '/',
      name: 'root',
      is_directory: true,
      children: [
        { path: '/home', name: 'home', is_directory: true },
        { path: '/var', name: 'var', is_directory: true },
        { path: '/opt', name: 'opt', is_directory: true },
      ],
    },
  }),
};

export const folderAPI = {
  list: () => Promise.resolve({ data: mockFolders }),
  get: (id) => Promise.resolve({ data: mockFolders.find(f => f.id === id) }),
  create: (data) => Promise.resolve({ data: { ...data, id: Date.now().toString() } }),
  update: (id, data) => Promise.resolve({ data: { ...data, id } }),
  delete: (id) => Promise.resolve({ success: true }),
  assignAgents: (id, agentIds) => Promise.resolve({ success: true }),
};

export const authAPI = {
  login: (credentials) => Promise.resolve({ 
    data: { 
      token: 'mock-jwt-token',
      user: { id: '1', username: 'admin', role: 'admin' }
    } 
  }),
  logout: () => Promise.resolve({ success: true }),
};
EOF

# Create contexts/AuthContext.js
cat > src/contexts/AuthContext.js << 'EOF'
import React, { createContext, useState, useContext } from 'react';

const AuthContext = createContext(null);

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return context;
};

export const AuthProvider = ({ children }) => {
  const [user, setUser] = useState({ username: 'admin', role: 'admin' });
  const [isAuthenticated, setIsAuthenticated] = useState(true);
  const [darkMode, setDarkMode] = useState(false);

  const login = async (credentials) => {
    setUser({ username: credentials.username, role: 'admin' });
    setIsAuthenticated(true);
    return true;
  };

  const logout = () => {
    setUser(null);
    setIsAuthenticated(false);
  };

  return (
    <AuthContext.Provider value={{
      user,
      isAuthenticated,
      darkMode,
      setDarkMode,
      login,
      logout,
    }}>
      {children}
    </AuthContext.Provider>
  );
};
EOF

# Create contexts/WebSocketContext.js
cat > src/contexts/WebSocketContext.js << 'EOF'
import React, { createContext, useContext, useEffect, useState } from 'react';

const WebSocketContext = createContext(null);

export const useWebSocket = () => {
  const context = useContext(WebSocketContext);
  if (!context) {
    throw new Error('useWebSocket must be used within WebSocketProvider');
  }
  return context;
};

export const WebSocketProvider = ({ children }) => {
  const [connected, setConnected] = useState(false);
  const [subscribers, setSubscribers] = useState({});

  const subscribe = (event, handler) => {
    setSubscribers(prev => ({
      ...prev,
      [event]: [...(prev[event] || []), handler],
    }));
  };

  const unsubscribe = (event, handler) => {
    setSubscribers(prev => ({
      ...prev,
      [event]: (prev[event] || []).filter(h => h !== handler),
    }));
  };

  useEffect(() => {
    setConnected(true);
    return () => setConnected(false);
  }, []);

  return (
    <WebSocketContext.Provider value={{
      connected,
      subscribe,
      unsubscribe,
    }}>
      {children}
    </WebSocketContext.Provider>
  );
};
EOF

# Create store/index.js
cat > src/store/index.js << 'EOF'
import { configureStore } from '@reduxjs/toolkit';

export const store = configureStore({
  reducer: {
    // Add reducers here
  },
});
EOF

# Create components/Layout/Header.js
cat > src/components/Layout/Header.js << 'EOF'
import React from 'react';
import { Layout, Button, Avatar, Space, Badge } from 'antd';
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  BellOutlined,
  UserOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import { useAuth } from '../../contexts/AuthContext';

const { Header: AntHeader } = Layout;

function Header({ collapsed, onToggle }) {
  const { user, logout } = useAuth();

  return (
    <AntHeader style={{ background: '#fff', padding: 0, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
      <Button
        type="text"
        icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
        onClick={onToggle}
        style={{ fontSize: '16px', width: 64, height: 64 }}
      />
      
      <Space style={{ marginRight: 24 }}>
        <Badge count={5}>
          <Button type="text" icon={<BellOutlined />} />
        </Badge>
        <Avatar icon={<UserOutlined />} />
        <span>{user?.username}</span>
        <Button type="text" icon={<LogoutOutlined />} onClick={logout}>
          Logout
        </Button>
      </Space>
    </AntHeader>
  );
}

export default Header;
EOF

# Create components/Layout/Sidebar.js
cat > src/components/Layout/Sidebar.js << 'EOF'
import React from 'react';
import { Layout, Menu } from 'antd';
import { useNavigate, useLocation } from 'react-router-dom';
import {
  DashboardOutlined,
  DesktopOutlined,
  FolderOutlined,
  BarChartOutlined,
  UserOutlined,
  SettingOutlined,
} from '@ant-design/icons';

const { Sider } = Layout;

function Sidebar({ collapsed, onCollapse }) {
  const navigate = useNavigate();
  const location = useLocation();

  const menuItems = [
    { key: '/dashboard', icon: <DashboardOutlined />, label: 'Dashboard' },
    { key: '/agents', icon: <DesktopOutlined />, label: 'Agents' },
    { key: '/folders', icon: <FolderOutlined />, label: 'Folders' },
    { key: '/reports', icon: <BarChartOutlined />, label: 'Reports' },
    { key: '/users', icon: <UserOutlined />, label: 'Users' },
    { key: '/settings', icon: <SettingOutlined />, label: 'Settings' },
  ];

  return (
    <Sider
      collapsible
      collapsed={collapsed}
      onCollapse={onCollapse}
      style={{ position: 'fixed', left: 0, top: 0, bottom: 0, height: '100vh' }}
    >
      <div style={{ 
        height: 64, 
        display: 'flex', 
        alignItems: 'center', 
        justifyContent: 'center',
        color: 'white',
        fontSize: collapsed ? '16px' : '20px',
        fontWeight: 'bold'
      }}>
        {collapsed ? 'SM' : 'Sync Manager'}
      </div>
      <Menu
        theme="dark"
        selectedKeys={[location.pathname]}
        mode="inline"
        items={menuItems}
        onClick={({ key }) => navigate(key)}
      />
    </Sider>
  );
}

export default Sidebar;
EOF

# Create components/PrivateRoute/index.js
cat > src/components/PrivateRoute/index.js << 'EOF'
import React from 'react';
import { Navigate, Outlet } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';

function PrivateRoute() {
  const { isAuthenticated } = useAuth();
  
  return isAuthenticated ? <Outlet /> : <Navigate to="/login" />;
}

export default PrivateRoute;
EOF

# Create pages/Login.js
cat > src/pages/Login.js << 'EOF'
import React from 'react';
import { Form, Input, Button, Card } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

function Login() {
  const navigate = useNavigate();
  const { login } = useAuth();

  const onFinish = async (values) => {
    await login(values);
    navigate('/dashboard');
  };

  return (
    <div style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#f0f2f5' }}>
      <Card title="Syncthing Management Login" style={{ width: 400 }}>
        <Form onFinish={onFinish}>
          <Form.Item name="username" rules={[{ required: true, message: 'Please input your username!' }]}>
            <Input prefix={<UserOutlined />} placeholder="Username" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: 'Please input your password!' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="Password" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" style={{ width: '100%' }}>
              Log in
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}

export default Login;
EOF

# Create pages/Dashboard.js
cat > src/pages/Dashboard.js << 'EOF'
import React from 'react';
import { Card, Col, Row, Statistic } from 'antd';
import {
  DesktopOutlined,
  FolderOutlined,
  SyncOutlined,
  CheckCircleOutlined,
} from '@ant-design/icons';

function Dashboard() {
  return (
    <div>
      <h1>Dashboard</h1>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic
              title="Total Agents"
              value={3}
              prefix={<DesktopOutlined />}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Active Folders"
              value={2}
              prefix={<FolderOutlined />}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Syncing"
              value={1}
              prefix={<SyncOutlined spin />}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="Online Agents"
              value={2}
              valueStyle={{ color: '#3f8600' }}
              prefix={<CheckCircleOutlined />}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
}

export default Dashboard;
EOF

# Create placeholder pages
cat > src/pages/Reports.js << 'EOF'
import React from 'react';
import { Card } from 'antd';

function Reports() {
  return (
    <Card title="Reports">
      <p>Reports functionality coming soon...</p>
    </Card>
  );
}

export default Reports;
EOF

cat > src/pages/Users.js << 'EOF'
import React from 'react';
import { Card } from 'antd';

function Users() {
  return (
    <Card title="User Management">
      <p>User management functionality coming soon...</p>
    </Card>
  );
}

export default Users;
EOF

cat > src/pages/Settings.js << 'EOF'
import React from 'react';
import { Card } from 'antd';

function Settings() {
  return (
    <Card title="Settings">
      <p>Settings functionality coming soon...</p>
    </Card>
  );
}

export default Settings;
EOF

echo "Basic files created. Now creating Agents and Folders pages..."

# Create the Agents page - copy from artifact dashboard-agents-page
# Create the Folders page - copy from artifact folder-management-page
# These are too long for this script, need to be created separately

echo "All files created successfully!"
echo "Now you need to manually create:"
echo "1. src/pages/Agents.js (copy from artifact dashboard-agents-page)"
echo "2. src/pages/Folders.js (copy from artifact folder-management-page)"
EOF

chmod +x create-all-files.sh
