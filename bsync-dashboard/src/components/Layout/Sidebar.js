import React from 'react';
import { Layout, Menu } from 'antd';
import { useNavigate, useLocation } from 'react-router-dom';
import {
  DashboardOutlined,
  DesktopOutlined,
  SyncOutlined,
  BarChartOutlined,
  FolderOutlined,
  KeyOutlined,
  UserOutlined,
} from '@ant-design/icons';

const { Sider } = Layout;

function Sidebar({ collapsed, onCollapse }) {
  const navigate = useNavigate();
  const location = useLocation();

  const menuItems = [
    { key: '/dashboard', icon: <DashboardOutlined />, label: 'Dashboard' },
    { key: '/agents', icon: <DesktopOutlined />, label: 'Agents' },
    { key: '/jobs', icon: <SyncOutlined />, label: 'Sync Jobs' },
    // { key: '/license-management', icon: <KeyOutlined />, label: 'License Management' },
    { key: '/folder-stats', icon: <FolderOutlined />, label: 'Folder Statistics' },
    { key: '/users', icon: <UserOutlined />, label: 'User Management' },
    { key: '/reports', icon: <BarChartOutlined />, label: 'Reports' },
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
