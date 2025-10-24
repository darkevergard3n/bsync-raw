import React, { useState } from 'react';
import { Layout as AntLayout, Menu, Button, Avatar, Dropdown, Space, Typography, Badge, message } from 'antd';
import { useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { useWebSocket } from '../../contexts/WebSocketContext';
import api from '../../services/api';
import {
  DashboardOutlined,
  DatabaseOutlined,
  FolderOutlined,
  UserOutlined,
  FileTextOutlined,
  SettingOutlined,
  LogoutOutlined,
  BellOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  SunOutlined,
  MoonOutlined,
  SyncOutlined,
  KeyOutlined,
} from '@ant-design/icons';

const { Header, Sider, Content } = AntLayout;
const { Title } = Typography;

const Layout = ({ children, onThemeToggle, isDarkMode }) => {
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout } = useAuth();
  const { notifications, onlineAgents, totalAgents } = useWebSocket();

  // Get initials from fullname
  const getInitials = (fullname) => {
    if (!fullname) return 'U';
    const names = fullname.trim().split(' ');
    if (names.length === 1) {
      return names[0].substring(0, 2).toUpperCase();
    }
    return (names[0].charAt(0) + names[names.length - 1].charAt(0)).toUpperCase();
  };

  // Logout function
  const handleLogout = async () => {
    try {
      await api.post('/api/v1/auth/logout');
      message.success('Logged out successfully');
    } catch (error) {
      console.error('Logout failed:', error);
      message.error('Failed to logout');
    } finally {
      // Always call logout from context to clear state
      await logout();
    }
  };

  // Menu items based on user role
  const getMenuItems = () => {
    const baseItems = [
      {
        key: '/dashboard',
        icon: <DashboardOutlined />,
        label: 'Dashboard',
      },
      {
        key: '/agents',
        icon: <DatabaseOutlined />,
        label: 'Agents',
      },
      {
        key: '/jobs',
        icon: <SyncOutlined />,
        label: 'Sync Jobs',
      },
      // {
      //   key: '/license-management',
      //   icon: <KeyOutlined />,
      //   label: 'License Management',
      // },
      {
        key: '/folder-stats',
        icon: <FolderOutlined />,
        label: 'Folder Statistics',
      },
      {
        key: '/users',
        icon: <UserOutlined />,
        label: 'User Management',
      },
      {
        key: '/reports',
        icon: <FileTextOutlined />,
        label: 'Reports',
      },
      {
        key: '/settings',
        icon: <SettingOutlined />,
        label: 'Settings',
      },
    ];

    return baseItems;
  };

  const handleMenuClick = ({ key }) => {
    navigate(key);
  };

  const handleUserMenuClick = ({ key }) => {
    if (key === 'settings') {
      navigate('/settings');
    } else if (key === 'profile') {
      // TODO: Navigate to profile page
      message.info('Profile page coming soon');
    }
  };

  const userMenuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: 'Profile',
    },
    {
      key: 'settings',
      icon: <SettingOutlined />,
      label: 'Settings',
    },
    {
      type: 'divider',
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: 'Logout',
      onClick: handleLogout,
    },
  ];

  return (
    <AntLayout style={{ minHeight: '100vh' }}>
      <Sider 
        trigger={null} 
        collapsible 
        collapsed={collapsed}
        width={240}
        collapsedWidth={80}
        style={{
          background: '#001529',
        }}
      >
        <div
          style={{
            height: 80,
            margin: '20px 16px',
            background: '#001529',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            borderRadius: '8px',
            padding: '12px',
            transition: 'all 0.2s',
          }}
        >
          <img 
            src={collapsed ? "/bsync-logo-collapsed.png" : "/bsync-logo-utama.png"} 
            alt="Primasys Logo" 
            style={{
              height: '100%',
              width: 'auto',
              objectFit: 'contain',
              maxWidth: '100%'
            }}
          />
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={getMenuItems()}
          onClick={handleMenuClick}
          style={{
            background: '#2F3349',
            border: 'none',
          }}
          className="custom-sidebar-menu"
        />
      </Sider>
      <AntLayout className="site-layout">
        <Header
          style={{
            padding: '0 24px',
            background: isDarkMode ? '#141414' : '#fff',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: `1px solid ${isDarkMode ? '#303030' : '#f0f0f0'}`,
          }}
        >
          <Space>
            <Button
              type="text"
              icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              onClick={() => setCollapsed(!collapsed)}
              style={{
                fontSize: '16px',
                width: 64,
                height: 64,
              }}
            />
            {/* <div style={{ marginLeft: 16 }}>
              <Space>
                <Badge count={onlineAgents} showZero>
                  <DesktopOutlined style={{ fontSize: '16px' }} />
                </Badge>
                <span style={{ fontSize: '14px', color: '#666' }}>
                  {onlineAgents}/{totalAgents} agents online
                </span>
              </Space>
            </div> */}
          </Space>

          <Space size="middle">
            <Button
              type="text"
              icon={isDarkMode ? <SunOutlined /> : <MoonOutlined />}
              onClick={onThemeToggle}
              style={{ fontSize: '16px' }}
            />
            
            <Badge count={notifications.length} size="small">
              <Button
                type="text"
                icon={<BellOutlined />}
                style={{ fontSize: '16px' }}
              />
            </Badge>

            <Dropdown
              menu={{ items: userMenuItems, onClick: handleUserMenuClick }}
              placement="bottomRight"
              arrow
            >
              <Space style={{ cursor: 'pointer' }}>
                <Avatar
                  size="small"
                  style={{ backgroundColor: '#1890ff' }}
                >
                  {user?.fullname ? getInitials(user.fullname) : 'U'}
                </Avatar>
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
                  <span style={{ fontSize: '14px', fontWeight: 500, lineHeight: '20px' }}>
                    {user?.fullname || 'User'}
                  </span>
                  <span style={{
                    fontSize: '12px',
                    fontWeight: 400,
                    lineHeight: '16px',
                    color: '#6b7280',
                    textTransform: 'capitalize'
                  }}>
                    {user?.role_label || user?.role || 'User'}
                  </span>
                </div>
              </Space>
            </Dropdown>
          </Space>
        </Header>
        <Content
          style={{
            margin: '24px',
            minHeight: 'calc(100vh - 112px)',
          }}
        >
          {children}
        </Content>
      </AntLayout>
    </AntLayout>
  );
};

export default Layout;