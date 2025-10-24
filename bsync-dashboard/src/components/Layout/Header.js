import React from 'react';
import { Layout, Button, Avatar, Space, Badge, Tooltip } from 'antd';
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  BellOutlined,
  UserOutlined,
  LogoutOutlined,
  WifiOutlined,
  DisconnectOutlined,
  LoadingOutlined,
} from '@ant-design/icons';
import { useAuth } from '../../contexts/AuthContext';
import { useWebSocket } from '../../contexts/WebSocketContext';

const { Header: AntHeader } = Layout;

function Header({ collapsed, onToggle }) {
  const { user, logout } = useAuth();
  const { connected, connectionState, isReconnecting, reconnectAttempts } = useWebSocket();
  
  const getConnectionIcon = () => {
    if (isReconnecting || connectionState === 'connecting') {
      return <LoadingOutlined spin />;
    }
    if (connected && connectionState === 'connected') {
      return <WifiOutlined style={{ color: '#52c41a' }} />;
    }
    return <DisconnectOutlined style={{ color: '#ff4d4f' }} />;
  };
  
  const getConnectionTooltip = () => {
    if (isReconnecting) {
      return `Reconnecting... (attempt ${reconnectAttempts})`;
    }
    if (connected) {
      return 'Real-time updates connected';
    }
    if (connectionState === 'error') {
      return 'Connection failed - refresh page to retry';
    }
    return 'Real-time updates disconnected';
  };

  return (
    <AntHeader style={{ background: '#fff', padding: 0, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
      <Button
        type="text"
        icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
        onClick={onToggle}
        style={{ fontSize: '16px', width: 64, height: 64 }}
      />
      
      <Space style={{ marginRight: 24 }}>
        <Tooltip title={getConnectionTooltip()}>
          <Button type="text" icon={getConnectionIcon()} />
        </Tooltip>
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
