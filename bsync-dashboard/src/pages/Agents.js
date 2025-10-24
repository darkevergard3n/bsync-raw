// Save this as src/pages/Agents.js
import React, { useState, useEffect, useMemo } from 'react';
import {
  Table,
  Card,
  Button,
  Tag,
  Space,
  Input,
  Select,
  Modal,
  message,
  Descriptions,
  Popconfirm,
  Row,
  Col,
  Typography,
  Dropdown,
  ConfigProvider,
} from 'antd';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  SyncOutlined,
  DesktopOutlined,
  ReloadOutlined,
  ExclamationCircleOutlined,
  WindowsOutlined,
  AppleOutlined,
  DeleteOutlined,
  DatabaseOutlined,
  DisconnectOutlined,
  ClockCircleOutlined,
  EditOutlined,
  MoreOutlined,
} from '@ant-design/icons';
import { useWebSocket } from '../contexts/WebSocketContext';
import { agentAPI } from '../services/api';
// Using built-in Date methods instead of moment

const { Search } = Input;
const { Option } = Select;
const { Title, Text } = Typography;

function Agents() {
  const { subscribe, unsubscribe } = useWebSocket();
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [filterStatus, setFilterStatus] = useState('all');
  const [filterApproval, setFilterApproval] = useState('all');
  const [isPageActive, setIsPageActive] = useState(true); // Track if page is active
  const [selectedAgent, setSelectedAgent] = useState(null);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [stats, setStats] = useState({
    total: 0,
    online: 0,
    offline: 0,
    pending: 0,
  });

  const fetchAgents = async () => {
    console.log('[Agents] Fetching agents...');
    setLoading(true);
    try {
      const response = await agentAPI.list();
      
      // Use the hybrid data directly (already processed in api.js)
      const agentArray = response.data.data || [];
      
      console.log('[Agents] Loaded', agentArray.length, 'agents');
      
      setAgents(agentArray);
      calculateStats(agentArray);
    } catch (error) {
      console.error('[Agents] Failed to fetch agents:', error);
      console.error('[Agents] Error details:', {
        message: error.message,
        response: error.response?.data,
        status: error.response?.status,
        statusText: error.response?.statusText
      });
      
      message.error('Failed to fetch agents: ' + (error.response?.data?.error || error.message));
      
      // Set empty agents to avoid undefined errors
      setAgents([]);
      calculateStats([]);
    } finally {
      setLoading(false);
    }
  };

  const refreshSingleAgent = async (agentId) => {
    // console.log('[Agents] Refreshing single agent:', agentId); // Reduced logging
    
    // Find current agent to get hostname for notification
    const currentAgent = agents.find(a => a.id === agentId);
    const displayName = currentAgent?.hostname || agentId;
    
    try {
      // Fetch latest data
      const response = await agentAPI.list();
      const agentArray = response.data.data || [];
      
      // Update the specific agent in the list
      setAgents(prevAgents => {
        const updatedAgents = prevAgents.map(agent => {
          if (agent.id === agentId) {
            const refreshedAgent = agentArray.find(a => a.id === agentId);
            return refreshedAgent || agent;
          }
          return agent;
        });
        calculateStats(updatedAgents);
        return updatedAgents;
      });
      
      message.success(`Agent ${displayName} refreshed successfully`);
    } catch (error) {
      console.error('[Agents] Failed to refresh agent:', agentId, error);
      message.error(`Failed to refresh agent ${displayName}: ` + (error.response?.data?.error || error.message));
    }
  };

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    console.log('[Agents] Component mounted');
    fetchAgents();
  }, []);

  // Track page visibility to optimize performance
  useEffect(() => {
    const handleVisibilityChange = () => {
      const isVisible = !document.hidden;
      console.log('[Agents] Page visibility changed:', isVisible ? 'visible' : 'hidden');
      setIsPageActive(isVisible);
    };

    // Add visibility change listener
    document.addEventListener('visibilitychange', handleVisibilityChange);

    // Cleanup
    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      setIsPageActive(false);
    };
  }, []);

  // WebSocket subscription - only active when page is visible
  useEffect(() => {
    const handleAgentUpdate = (data) => {
      // Only process updates if page is active/visible
      if (!isPageActive) {
        console.log('[Agents] Ignoring update - page not active');
        return;
      }

      if (data.type === 'agent_update') {
        console.log('[Agents] Processing agent update for:', data.agent?.id);
        setAgents((prev) =>
          prev.map((agent) =>
            agent.id === data.agent.id ? data.agent : agent
          )
        );
      } else if (data.type === 'agent_new') {
        console.log('[Agents] Processing new agent:', data.agent?.id);
        setAgents((prev) => [data.agent, ...prev]);
        message.info('New agent registration received');
      }
    };

    // Only subscribe if page is active and WebSocket functions are available
    if (subscribe && unsubscribe && isPageActive) {
      console.log('[Agents] Starting WebSocket subscription');
      subscribe('agent_updates', handleAgentUpdate);

      return () => {
        console.log('[Agents] Stopping WebSocket subscription');
        unsubscribe('agent_updates', handleAgentUpdate);
      };
    } else {
      console.log('[Agents] WebSocket subscription skipped - page inactive or functions unavailable');
    }
  }, [subscribe, unsubscribe, isPageActive]);

  const calculateStats = (agentList) => {
    // Ensure agentList is an array and handle null/undefined cases
    if (!Array.isArray(agentList) || agentList.length === 0) {
      setStats({ total: 0, online: 0, offline: 0, pending: 0 });
      return;
    }

    const stats = agentList.reduce(
      (acc, agent) => {
        acc.total++;
        // Check if agent should be considered offline based on heartbeat
        const isOfflineByHeartbeat = isAgentOfflineByHeartbeat(agent.last_heartbeat);
        const effectiveStatus = isOfflineByHeartbeat ? 'offline' : agent.status;
        
        if (effectiveStatus === 'online') acc.online++;
        else if (effectiveStatus === 'offline') acc.offline++;
        if (agent.approval_status === 'pending') acc.pending++;
        return acc;
      },
      { total: 0, online: 0, offline: 0, pending: 0 }
    );
    setStats(stats);
  };

  const handleApprove = async (agentId) => {
    try {
      await agentAPI.approve(agentId, 'Approved via UI');
      message.success('Agent approved successfully');
      fetchAgents();
    } catch (error) {
      console.error('Approve error:', error);
      message.error('Failed to approve agent: ' + (error.response?.data?.error || error.message));
    }
  };

  const handleReject = async (agentId) => {
    try {
      await agentAPI.reject(agentId, 'Rejected via UI');
      message.success('Agent rejected');
      fetchAgents();
    } catch (error) {
      console.error('Reject error:', error);
      message.error('Failed to reject agent: ' + (error.response?.data?.error || error.message));
    }
  };


  const handleDelete = async (agentId) => {
    try {
      await agentAPI.delete(agentId);
      message.success(`Agent ${agentId} deleted successfully`);
      fetchAgents(); // Refresh the agent list
    } catch (error) {
      console.error('Delete agent error:', error);
      message.error(`Failed to delete agent: ${error.response?.data?.error || error.message}`);
    }
  };

  const getStatusTag = (status, lastHeartbeat, connected) => {
    // Use connected field if available, otherwise fall back to status logic
    let effectiveStatus = 'offline';
    if (connected !== undefined) {
      effectiveStatus = connected ? 'online' : 'offline';
    } else {
      // Check if agent should be considered offline based on heartbeat
      const isOfflineByHeartbeat = isAgentOfflineByHeartbeat(lastHeartbeat);
      effectiveStatus = isOfflineByHeartbeat ? 'offline' : status;
    }
    
    const config = {
      online: { color: 'green', icon: <CheckCircleOutlined /> },
      offline: { color: 'red', icon: <CloseCircleOutlined /> },
      error: { color: 'orange', icon: <ExclamationCircleOutlined /> },
      updating: { color: 'blue', icon: <SyncOutlined spin /> },
    };

    const { color, icon } = config[effectiveStatus] || {};
    return (
      <Tag 
        color={color} 
        icon={icon}
        style={effectiveStatus === 'offline' ? { 
          backgroundColor: '#ff4d4f', 
          color: 'white',
          border: '1px solid #ff4d4f'
        } : {}}
      >
        {effectiveStatus.toUpperCase()}
      </Tag>
    );
  };

  // Helper function to check if agent is offline based on heartbeat
  const isAgentOfflineByHeartbeat = (lastHeartbeat) => {
    if (!lastHeartbeat) return true;
    const now = new Date();
    const heartbeatTime = new Date(lastHeartbeat);
    const diffMs = now - heartbeatTime;
    const diffMins = Math.floor(diffMs / 60000);
    return diffMins > 2; // More than 2 minutes = offline
  };


  // Linux penguin-like icon component
  const LinuxIcon = ({ color = '#fcc624' }) => (
    <span style={{ 
      color, 
      fontSize: '14px', 
      fontWeight: 'bold',
      fontFamily: 'monospace',
      display: 'inline-block',
      transform: 'rotate(-5deg)'
    }}>
      🐧
    </span>
  );

  const getOSIcon = (os) => {
    const osLower = (os || '').toLowerCase();
    
    if (osLower.includes('windows')) {
      return <WindowsOutlined style={{ color: '#0078d4', fontSize: '16px' }} />;
    } else if (osLower.includes('macos') || osLower.includes('darwin')) {
      return <AppleOutlined style={{ color: '#000000', fontSize: '16px' }} />;
    } else if (osLower.includes('ubuntu')) {
      return <LinuxIcon color="#e95420" />;
    } else if (osLower.includes('linux')) {
      return <LinuxIcon color="#fcc624" />;
    } else {
      return <DesktopOutlined style={{ color: '#666666', fontSize: '16px' }} />;
    }
  };

  // Optimize filtering using useMemo to prevent unnecessary re-calculations
  const filteredAgents = useMemo(() => {
    console.log('[Agents] Recalculating filtered agents', { 
      agentsCount: agents.length, 
      searchText, 
      filterStatus, 
      filterApproval 
    });

    return agents.filter((agent) => {
      // Handle both backend format (syncthing_device_id, ip_address) and frontend format (device_id)
      const deviceId = agent.syncthing_device_id || agent.device_id || '';
      const ipAddress = agent.ip_address || '';
      const hostname = agent.hostname || '';
      
      const matchesSearch =
        hostname.toLowerCase().includes(searchText.toLowerCase()) ||
        deviceId.toLowerCase().includes(searchText.toLowerCase()) ||
        ipAddress.includes(searchText);

      const matchesStatus =
        filterStatus === 'all' || agent.status === filterStatus;

      const matchesApproval =
        filterApproval === 'all' || agent.approval_status === filterApproval;

      return matchesSearch && matchesStatus && matchesApproval;
    });
  }, [agents, searchText, filterStatus, filterApproval]);

  const columns = [
    {
      title: 'Hostname',
      dataIndex: 'hostname',
      key: 'hostname',
      width: 180,
      ellipsis: true,
      render: (text, record) => (
        <Space>
          <DesktopOutlined style={{ color: '#6b7280' }} />
          <span style={{ color: '#1f2937', fontWeight: 500 }}>
            {text}
          </span>
        </Space>
      ),
    },
    {
      title: 'IP Address',
      dataIndex: 'ip_address', 
      key: 'ip_address',
      width: 140,
      ellipsis: true,
      render: (text) => {
        const ipAddress = text || 'N/A';
        return (
          <span 
            style={{ 
              fontFamily: 'monospace', 
              fontSize: '13px',
              whiteSpace: 'nowrap',
              display: 'inline-block',
              maxWidth: '120px',
              overflow: 'hidden',
              textOverflow: 'ellipsis'
            }}
            title={ipAddress}
          >
            {ipAddress}
          </span>
        );
      },
    },
    {
      title: 'Operating System',
      dataIndex: 'os',
      key: 'os',
      width: 200,
      ellipsis: true,
      render: (os, record) => {
        const osName = (os || 'Unknown').charAt(0).toUpperCase() + (os || 'Unknown').slice(1);
        const arch = record.architecture || '';
        const fullOSName = arch ? `${osName} ${arch}` : osName;
        return (
          <Space style={{ whiteSpace: 'nowrap' }}>
            {getOSIcon(record.os)}
            <span style={{ whiteSpace: 'nowrap' }} title={fullOSName}>
              {fullOSName}
            </span>
          </Space>
        );
      },
    },
    // {
    //   title: 'Version',
    //   dataIndex: 'version',
    //   key: 'version',
    // },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status, record) => {
        const tag = getStatusTag(status, record.last_heartbeat, record.connected);
        return (
          <div style={{ 
            display: 'flex', 
            alignItems: 'center',
            whiteSpace: 'nowrap'
          }}>
            {tag}
          </div>
        );
      },
    },
    {
      title: 'Approval',
      dataIndex: 'approval_status',
      key: 'approval_status',
      width: 110,
      render: (status) => {
        const config = {
          pending: { color: '#f59e0b', bg: '#fef3c7' },
          approved: { color: '#10b981', bg: '#d1fae5' },
          rejected: { color: '#ef4444', bg: '#fee2e2' },
        };
        
        const style = config[status] || {};
        return (
          <span style={{
            color: style.color,
            backgroundColor: style.bg,
            padding: '4px 12px',
            borderRadius: '12px',
            fontSize: '12px',
            fontWeight: 500,
            textTransform: 'capitalize',
            whiteSpace: 'nowrap',
            display: 'inline-block'
          }}>
            {status || 'Unknown'}
          </span>
        );
      },
    },
    {
      title: 'Last Heartbeat',
      dataIndex: 'last_heartbeat',
      key: 'last_heartbeat',
      width: 150,
      ellipsis: true,
      render: (date) => {
        if (!date) return 'Never';
        const now = new Date();
        const heartbeat = new Date(date);
        const diffMs = now - heartbeat;
        const diffMins = Math.floor(diffMs / 60000);
        
        if (diffMins < 1) return 'Just now';
        if (diffMins < 60) return `${diffMins} minutes ago`;
        const diffHours = Math.floor(diffMins / 60);
        if (diffHours < 24) return `${diffHours} hours ago`;
        const diffDays = Math.floor(diffHours / 24);
        return `${diffDays} days ago`;
      },
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 200,
      render: (_, record) => {
        const menuItems = [
          {
            key: 'refresh',
            icon: <ReloadOutlined />,
            label: 'Refresh',
            onClick: () => refreshSingleAgent(record.id),
            disabled: record.approval_status === 'pending'
          },
          {
            key: 'edit',
            icon: <EditOutlined />,
            label: 'Edit',
            onClick: () => {
              setSelectedAgent(record);
              setDetailModalVisible(true);
            }
          },
          // Add approve/reject options for pending agents
          ...(record.approval_status === 'pending' ? [
            { type: 'divider' },
            {
              key: 'approve',
              icon: <CheckCircleOutlined />,
              label: 'Approve',
              onClick: () => {
                Modal.confirm({
                  title: 'Approve this agent?',
                  content: 'Are you sure you want to approve this agent?',
                  okText: 'Yes',
                  cancelText: 'No',
                  onOk: () => handleApprove(record.id),
                });
              },
              style: { color: '#10b981' }
            },
            {
              key: 'reject',
              icon: <CloseCircleOutlined />,
              label: 'Reject',
              onClick: () => {
                Modal.confirm({
                  title: 'Reject this agent?',
                  content: 'Are you sure you want to reject this agent?',
                  okText: 'Yes',
                  cancelText: 'No',
                  onOk: () => handleReject(record.id),
                });
              },
              danger: true
            }
          ] : []),
          { type: 'divider' },
          {
            key: 'delete',
            icon: <DeleteOutlined />,
            label: 'Delete',
            onClick: () => handleDelete(record.id),
            danger: true,
            disabled: record.approval_status === 'pending'
          }
        ];

        return (
          <Space size="middle">
            {/* Quick action icons */}
            <Button
              type="text"
              size="small" 
              icon={<ReloadOutlined />}
              onClick={() => refreshSingleAgent(record.id)}
              title="Refresh"
              style={{ color: '#6b7280' }}
            />
            <Button
              type="text"
              size="small"
              icon={<EditOutlined />}
              onClick={() => {
                setSelectedAgent(record);
                setDetailModalVisible(true);
              }}
              title="Edit"
              style={{ color: '#6b7280' }}
            />
            
            <Dropdown
              menu={{ items: menuItems }}
              trigger={['click']}
              placement="bottomRight"
            >
              <Button
                type="text"
                size="small"
                icon={<MoreOutlined />}
                style={{ color: '#6b7280' }}
              />
            </Dropdown>
          </Space>
        );
      },
    },
  ];

  return (
    <>
      <style>
        {`
          .custom-button:hover {
            border-color: #10b981 !important;
            color: inherit !important;
          }
          
          .custom-button:focus {
            border-color: #10b981 !important;
            color: inherit !important;
          }
        `}
      </style>
      <ConfigProvider
        theme={{
          token: {
            colorPrimary: '#10b981',
          },
        }}
      >
      {/* Page Header */}
      <div style={{ marginBottom: 24 }}>
        <Title level={2} style={{ margin: 0, color: '#1f2937', fontWeight: 600 }}>
          Agents Management
        </Title>
        <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280' }}>
          Monitor and manage synchronization agents
        </Text>
      </div>

      {/* Stats Cards */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} md={6}>
          <Card 
            style={{ 
              borderRadius: '12px', 
              border: '1px solid #e5e7eb',
              background: '#ffffff'
            }}
            styles={{ body: { padding: '20px' } }}
          >
            <div style={{ position: 'relative' }}>
              <div style={{ 
                position: 'absolute',
                top: '8px',
                right: '8px'
              }}>
                <DatabaseOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Total Agents
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {stats.total}
                </div>
                {/* <div style={{ display: 'flex', alignItems: 'center', marginTop: '8px' }}>
                  <span style={{ 
                    color: '#10b981', 
                    backgroundColor: '#d1fae5', 
                    padding: '2px 8px', 
                    borderRadius: '12px', 
                    fontSize: '12px',
                    fontWeight: 500
                  }}>
                    +1 from last hour
                  </span>
                </div> */}
              </div>
            </div>
          </Card>
        </Col>
        
        <Col xs={24} sm={12} md={6}>
          <Card 
            style={{ 
              borderRadius: '12px', 
              border: '1px solid #e5e7eb',
              background: '#ffffff'
            }}
            styles={{ body: { padding: '20px' } }}
          >
            <div style={{ position: 'relative' }}>
              <div style={{ 
                position: 'absolute',
                top: '8px',
                right: '8px'
              }}>
                <CheckCircleOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Online
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {stats.online}
                </div>
                {/* <div style={{ display: 'flex', alignItems: 'center', marginTop: '8px' }}>
                  <span style={{ 
                    color: '#10b981', 
                    backgroundColor: '#d1fae5', 
                    padding: '2px 8px', 
                    borderRadius: '12px', 
                    fontSize: '12px',
                    fontWeight: 500
                  }}>
                    +2 from last hour
                  </span>
                </div> */}
              </div>
            </div>
          </Card>
        </Col>
        
        <Col xs={24} sm={12} md={6}>
          <Card 
            style={{ 
              borderRadius: '12px', 
              border: '1px solid #e5e7eb',
              background: '#ffffff'
            }}
            styles={{ body: { padding: '20px' } }}
          >
            <div style={{ position: 'relative' }}>
              <div style={{ 
                position: 'absolute',
                top: '8px',
                right: '8px'
              }}>
                <CloseCircleOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Offline
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {stats.offline}
                </div>
                {/* <div style={{ display: 'flex', alignItems: 'center', marginTop: '8px' }}>
                  <span style={{ 
                    color: '#ef4444', 
                    backgroundColor: '#fee2e2', 
                    padding: '2px 8px', 
                    borderRadius: '12px', 
                    fontSize: '12px',
                    fontWeight: 500
                  }}>
                    -1 from last hour
                  </span>
                </div> */}
              </div>
            </div>
          </Card>
        </Col>
        
        <Col xs={24} sm={12} md={6}>
          <Card 
            style={{ 
              borderRadius: '12px', 
              border: '1px solid #e5e7eb',
              background: '#ffffff'
            }}
            styles={{ body: { padding: '20px' } }}
          >
            <div style={{ position: 'relative' }}>
              <div style={{ 
                position: 'absolute',
                top: '8px',
                right: '8px'
              }}>
                <ClockCircleOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Pending Approval
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {stats.pending}
                </div>
                {/* <div style={{ display: 'flex', alignItems: 'center', marginTop: '8px' }}>
                  <span style={{ 
                    color: '#6b7280', 
                    backgroundColor: '#f3f4f6', 
                    padding: '2px 8px', 
                    borderRadius: '12px', 
                    fontSize: '12px',
                    fontWeight: 500
                  }}>
                    0 from last hour
                  </span>
                </div> */}
              </div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* Table Container Card */}
      <Card 
        style={{ 
          borderRadius: '12px', 
          border: '1px solid #e5e7eb',
          background: '#ffffff'
        }}
        styles={{ body: { padding: '24px' } }}
      >
        {/* Table Header */}
        <div style={{ 
          display: 'flex', 
          justifyContent: 'space-between', 
          alignItems: 'center',
          marginBottom: '24px',
          flexWrap: 'wrap',
          gap: '16px'
        }}>
          <div>
            <Title level={4} style={{ margin: 0, color: '#1f2937', fontWeight: 600 }}>
              Registered Agents
            </Title>
          </div>
          
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
            <Search
              placeholder="Search agents, IP addresses..."
              allowClear
              onSearch={setSearchText}
              style={{ width: 280 }}
              size="middle"
              className="custom-search"
            />
            <Select
              value={filterStatus}
              onChange={setFilterStatus}
              style={{ width: 140 }}
              size="middle"
              placeholder="All Status"
              className="custom-select"
            >
              <Option value="all">All Status</Option>
              <Option value="online">Online</Option>
              <Option value="offline">Offline</Option>
              <Option value="error">Error</Option>
            </Select>
            <Select
              value={filterApproval}
              onChange={setFilterApproval}
              style={{ width: 140 }}
              size="middle"
              placeholder="All"
              className="custom-select"
            >
              <Option value="all">All</Option>
              <Option value="pending">Pending</Option>
              <Option value="approved">Approved</Option>
              <Option value="rejected">Rejected</Option>
            </Select>
            <Button
              icon={<ReloadOutlined />}
              onClick={fetchAgents}
              loading={loading}
              type="default"
              className="custom-button"
              style={{ 
                borderRadius: '8px',
                display: 'flex',
                alignItems: 'center',
                gap: '4px'
              }}
            >
              Refresh All
            </Button>
          </div>
        </div>

        {/* Table */}
        <Table
          columns={columns}
          dataSource={filteredAgents}
          rowKey="id"
          loading={loading}
          pagination={false}
          className="custom-table"
        />
        {/* Custom Pagination */}
        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '16px 0 0 0',
          borderTop: '1px solid #f3f4f6',
          marginTop: '16px'
        }}>
          <span style={{ color: '#6b7280', fontSize: '14px' }}>
            Showing 1 to {Math.min(5, filteredAgents.length)} of {filteredAgents.length} entries
          </span>
          <div style={{ display: 'flex', alignItems: 'center', gap: '24px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ color: '#6b7280', fontSize: '14px' }}>Rows per page</span>
              <select 
                className="pagination-select"
                style={{
                  border: '1px solid #d1d5db',
                  borderRadius: '8px',
                  padding: '4px 8px',
                  fontSize: '14px',
                  minWidth: '70px',
                  outline: 'none',
                  transition: 'all 0.2s ease'
                }}
                onMouseEnter={(e) => {
                  e.target.style.borderColor = '#10b981';
                  e.target.style.boxShadow = '0 0 0 2px rgba(16, 185, 129, 0.2)';
                }}
                onMouseLeave={(e) => {
                  if (document.activeElement !== e.target) {
                    e.target.style.borderColor = '#d1d5db';
                    e.target.style.boxShadow = 'none';
                  }
                }}
                onFocus={(e) => {
                  e.target.style.borderColor = '#10b981';
                  e.target.style.boxShadow = '0 0 0 2px rgba(16, 185, 129, 0.2)';
                }}
                onBlur={(e) => {
                  e.target.style.borderColor = '#d1d5db';
                  e.target.style.boxShadow = 'none';
                }}
                defaultValue="5"
              >
                <option value="5">5</option>
                <option value="10">10</option>
                <option value="20">20</option>
                <option value="50">50</option>
              </select>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
              <span style={{ color: '#6b7280', fontSize: '14px' }}>Page 1 of 1</span>
              <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <button style={{
                  border: '1px solid #d1d5db',
                  background: 'white',
                  padding: '6px 12px',
                  borderRadius: '6px',
                  color: '#374151',
                  cursor: 'pointer',
                  fontSize: '16px'
                }}>
                  ‹
                </button>
                <button style={{
                  border: '1px solid #00be62',
                  background: '#00be62',
                  padding: '6px 12px',
                  borderRadius: '6px',
                  color: 'white',
                  cursor: 'pointer',
                  fontSize: '14px',
                  minWidth: '32px'
                }}>
                  1
                </button>
                <button style={{
                  border: '1px solid #d1d5db',
                  background: 'white',
                  padding: '6px 12px',
                  borderRadius: '6px',
                  color: '#374151',
                  cursor: 'pointer',
                  fontSize: '16px'
                }}>
                  ›
                </button>
              </div>
            </div>
          </div>
        </div>
      </Card>

      <Modal
        title="SyncTool Integrated Agent Details"
open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={null}
        width={800}
      >
        {selectedAgent && (
          <Descriptions bordered column={2}>
            <Descriptions.Item label="Agent ID" span={2}>
              {selectedAgent.id || 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="Device ID" span={2}>
              <code style={{ fontSize: '12px' }}>{selectedAgent.device_id || 'N/A'}</code>
            </Descriptions.Item>
            <Descriptions.Item label="Hostname">
              {selectedAgent.hostname || 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="IP Address">
              {selectedAgent.ip_address || 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="Operating System">
              {selectedAgent.os || 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="Agent Type">
              SyncTool Integrated Agent
            </Descriptions.Item>
            <Descriptions.Item label="Version">
              {selectedAgent.version || 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="Data Directory">
              {selectedAgent.data_dir || 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="Last Seen">
              {selectedAgent.last_heartbeat ? new Date(selectedAgent.last_heartbeat).toLocaleString() : 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="Connected Since">
              {selectedAgent.created_at ? new Date(selectedAgent.created_at).toLocaleString() : 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="Connection Status" span={2}>
              {getStatusTag(selectedAgent.status, selectedAgent.last_heartbeat, selectedAgent.connected)}
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </ConfigProvider>
    </>
  );
}

export default Agents;
