import React, { useState, useEffect } from 'react';
import {
  Card,
  Row,
  Col,
  Statistic,
  Typography,
  Space,
  Tag,
  Progress,
  Spin,
  Alert,
} from 'antd';
import {
  DesktopOutlined,
  FolderOutlined,
  SyncOutlined,
  CheckCircleOutlined,
} from '@ant-design/icons';
import api from '../services/api';

const { Title } = Typography;

const DashboardSimple = () => {
  const [loading, setLoading] = useState(true);
  const [metrics, setMetrics] = useState({
    total_agents: 0,
    online_agents: 0,
    offline_agents: 0,
    total_folders: 0,
  });
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchDashboardData();
  }, []);

  const fetchDashboardData = async () => {
    try {
      setLoading(true);
      setError(null);
      
      // Simple metrics calculation
      const agentsResponse = await api.get('/agents');
      const agentsData = agentsResponse.data.data || [];
      
      const foldersResponse = await api.get('/folders');
      const foldersData = foldersResponse.data.data || [];
      
      setMetrics({
        total_agents: agentsData.length,
        online_agents: agentsData.filter(a => a.status === 'online').length,
        offline_agents: agentsData.filter(a => a.status === 'offline').length,
        total_folders: foldersData.length,
      });
      
    } catch (err) {
      console.error('Dashboard error:', err);
      setError(err.message);
      // Set fallback data
      setMetrics({
        total_agents: 0,
        online_agents: 0,
        offline_agents: 0,
        total_folders: 0,
      });
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '50px' }}>
        <Spin size="large" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert
        message="Error Loading Dashboard"
        description={error}
        type="error"
        showIcon
      />
    );
  }

  return (
    <div>
      <Title level={2}>Dashboard</Title>
      
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="Total Agents"
              value={metrics.total_agents}
              prefix={<DesktopOutlined />}
            />
          </Card>
        </Col>
        
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="Online Agents"
              value={metrics.online_agents}
              valueStyle={{ color: '#52c41a' }}
              prefix={<CheckCircleOutlined />}
            />
          </Card>
        </Col>
        
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="Offline Agents"
              value={metrics.offline_agents}
              valueStyle={{ color: '#ff4d4f' }}
              prefix={<DesktopOutlined />}
            />
          </Card>
        </Col>
        
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="Total Jobs"
              value={metrics.total_folders}
              prefix={<FolderOutlined />}
            />
          </Card>
        </Col>
      </Row>
      
      <Card style={{ marginTop: 24 }}>
        <Title level={4}>System Status</Title>
        <Space direction="vertical" style={{ width: '100%' }}>
          <div>
            <span>Server Status: </span>
            <Tag color="green">Online</Tag>
          </div>
          <div>
            <span>API Status: </span>
            <Tag color="green">Connected</Tag>
          </div>
        </Space>
      </Card>
    </div>
  );
};

export default DashboardSimple;