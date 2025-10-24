import React, { useState } from 'react';
import {
  Card,
  Table,
  Tag,
  Progress,
  Space,
  Typography,
  Tooltip,
  Badge,
  Button,
  Select,
  Input,
  Row,
  Col,
  Statistic,
  Empty,
} from 'antd';
import {
  SyncOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  ClockCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
  FilterOutlined,
} from '@ant-design/icons';
import { useWebSocket } from '../contexts/WebSocketContext';

const { Text } = Typography;
const { Option } = Select;

const JobStatusMonitor = () => {
  const {
    jobStatuses,
    jobHistory,
    getJobStats,
    connected,
    formatFileSize,
  } = useWebSocket();

  const [activeTab, setActiveTab] = useState('current');
  const [filterStatus, setFilterStatus] = useState('all');
  const [searchText, setSearchText] = useState('');

  const currentJobs = Array.from(jobStatuses.values());
  const jobStats = getJobStats();

  // Filter jobs based on search and status
  const filterJobs = (jobs, includeCompleted = false) => {
    return jobs.filter(job => {
      const matchesSearch = job.job_name?.toLowerCase().includes(searchText.toLowerCase()) ||
                           job.agent_name?.toLowerCase().includes(searchText.toLowerCase());
      
      const matchesStatus = filterStatus === 'all' || job.status === filterStatus;
      
      return matchesSearch && matchesStatus;
    });
  };

  const getStatusColor = (status) => {
    const colors = {
      running: 'processing',
      success: 'success',
      failed: 'error',
      idle: 'default',
    };
    return colors[status] || 'default';
  };

  const getStatusIcon = (status) => {
    const icons = {
      running: <SyncOutlined spin />,
      success: <CheckCircleOutlined />,
      failed: <ExclamationCircleOutlined />,
      idle: <ClockCircleOutlined />,
    };
    return icons[status] || <ClockCircleOutlined />;
  };

  const formatDuration = (startTime, endTime) => {
    if (!startTime) return '-';
    const start = new Date(startTime);
    const end = endTime ? new Date(endTime) : new Date();
    const diffMs = end - start;
    const diffSecs = Math.floor(diffMs / 1000);
    const diffMins = Math.floor(diffSecs / 60);
    const diffHours = Math.floor(diffMins / 60);
    
    if (diffHours > 0) {
      return `${diffHours}h ${diffMins % 60}m`;
    } else if (diffMins > 0) {
      return `${diffMins}m ${diffSecs % 60}s`;
    } else {
      return `${diffSecs}s`;
    }
  };

  const currentJobsColumns = [
    {
      title: 'Job Name',
      dataIndex: 'job_name',
      key: 'job_name',
      render: (name, record) => (
        <Space>
          <Badge status={getStatusColor(record.status)} />
          <Text strong>{name}</Text>
        </Space>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status, record) => (
        <Tag icon={getStatusIcon(status)} color={getStatusColor(status)}>
          {status.toUpperCase()}
          {record.error && (
            <Tooltip title={record.error}>
              <ExclamationCircleOutlined style={{ marginLeft: 4, color: '#ff4d4f' }} />
            </Tooltip>
          )}
        </Tag>
      ),
    },
    {
      title: 'Progress',
      dataIndex: 'progress',
      key: 'progress',
      render: (progress, record) => (
        <Space direction="vertical" size="small" style={{ width: '100%' }}>
          <Progress
            percent={Math.round(progress || 0)}
            size="small"
            status={record.status === 'failed' ? 'exception' : 'normal'}
          />
          {record.bytes_total && (
            <Text type="secondary" style={{ fontSize: '12px' }}>
              {formatFileSize(record.bytes_synced || 0)} / {formatFileSize(record.bytes_total)}
            </Text>
          )}
        </Space>
      ),
    },
    {
      title: 'Source Agent',
      dataIndex: 'source_agent',
      key: 'source_agent',
      render: (_, record) => (
        <Tooltip title={`Source: ${record.source_agent_name || record.agent_name}`}>
          <Tag color="blue">{record.source_agent_name || record.agent_name}</Tag>
        </Tooltip>
      ),
    },
    {
      title: 'Target Agent',
      dataIndex: 'target_agent',
      key: 'target_agent',
      render: (_, record) => (
        record.target_agent_name ? (
          <Tooltip title={`Target: ${record.target_agent_name}`}>
            <Tag color="green">{record.target_agent_name}</Tag>
          </Tooltip>
        ) : <Text type="secondary">-</Text>
      ),
    },
    {
      title: 'Files',
      key: 'files',
      render: (_, record) => (
        record.files_total ? (
          <Text>
            {record.files_synced || 0} / {record.files_total}
          </Text>
        ) : '-'
      ),
    },
    {
      title: 'Duration',
      key: 'duration',
      render: (_, record) => (
        <Text>{formatDuration(record.start_time, record.end_time)}</Text>
      ),
    },
    {
      title: 'Last Update',
      key: 'lastUpdate',
      render: (_, record) => (
        <Text type="secondary">
          {record.lastUpdate ? record.lastUpdate.toLocaleTimeString() : '-'}
        </Text>
      ),
    },
  ];

  const historyColumns = [
    {
      title: 'Job Name',
      dataIndex: 'job_name',
      key: 'job_name',
      render: (name) => <Text strong>{name}</Text>,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status, record) => (
        <Tag icon={getStatusIcon(status)} color={getStatusColor(status)}>
          {status.toUpperCase()}
          {record.error && (
            <Tooltip title={record.error}>
              <ExclamationCircleOutlined style={{ marginLeft: 4 }} />
            </Tooltip>
          )}
        </Tag>
      ),
    },
    {
      title: 'Source Agent',
      dataIndex: 'source_agent',
      key: 'source_agent',
      render: (_, record) => (
        <Tag color="blue">{record.source_agent_name || record.agent_name}</Tag>
      ),
    },
    {
      title: 'Target Agent',
      dataIndex: 'target_agent',
      key: 'target_agent',
      render: (_, record) => (
        record.target_agent_name ? (
          <Tag color="green">{record.target_agent_name}</Tag>
        ) : <Text type="secondary">-</Text>
      ),
    },
    {
      title: 'Data Transferred',
      key: 'data',
      render: (_, record) => (
        record.bytes_total ? formatFileSize(record.bytes_total) : '-'
      ),
    },
    {
      title: 'Files',
      dataIndex: 'files_total',
      key: 'files_total',
      render: (files) => files || '-',
    },
    {
      title: 'Duration',
      key: 'duration',
      render: (_, record) => formatDuration(record.start_time, record.end_time),
    },
    {
      title: 'Completed At',
      dataIndex: 'completedAt',
      key: 'completedAt',
      render: (date) => (
        <Text type="secondary">
          {date ? new Date(date).toLocaleString() : '-'}
        </Text>
      ),
    },
  ];

  return (
    <div>
      {/* Connection Status */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Space align="center">
          <Badge status={connected ? 'success' : 'error'} />
          <Text>
            Real-time Updates: {connected ? 'Connected' : 'Disconnected'}
          </Text>
          {!connected && (
            <Button
              size="small"
              icon={<ReloadOutlined />}
              onClick={() => window.location.reload()}
            >
              Reconnect
            </Button>
          )}
        </Space>
      </Card>

      {/* Job Statistics */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={6}>
          <Card size="small">
            <Statistic
              title="Total Jobs"
              value={jobStats.total}
              prefix={<SyncOutlined />}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Statistic
              title="Running"
              value={jobStats.running}
              prefix={<SyncOutlined spin />}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Statistic
              title="Successful"
              value={jobStats.success}
              prefix={<CheckCircleOutlined />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Statistic
              title="Failed"
              value={jobStats.failed}
              prefix={<ExclamationCircleOutlined />}
              valueStyle={{ color: '#ff4d4f' }}
            />
          </Card>
        </Col>
      </Row>

      {/* Filters */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Space>
          <Button.Group>
            <Button
              type={activeTab === 'current' ? 'primary' : 'default'}
              onClick={() => setActiveTab('current')}
            >
              Current Jobs ({currentJobs.length})
            </Button>
            <Button
              type={activeTab === 'history' ? 'primary' : 'default'}
              onClick={() => setActiveTab('history')}
            >
              Job History ({jobHistory.length})
            </Button>
          </Button.Group>
          
          <Input
            placeholder="Search jobs..."
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 200 }}
            allowClear
          />
          
          <Select
            value={filterStatus}
            onChange={setFilterStatus}
            style={{ width: 120 }}
            suffixIcon={<FilterOutlined />}
          >
            <Option value="all">All Status</Option>
            <Option value="running">Running</Option>
            <Option value="success">Success</Option>
            <Option value="failed">Failed</Option>
            <Option value="idle">Idle</Option>
          </Select>
        </Space>
      </Card>

      {/* Job Tables */}
      <Card
        title={
          <Space>
            {activeTab === 'current' ? (
              <>
                <SyncOutlined />
                <Text>Current Jobs</Text>
              </>
            ) : (
              <>
                <ClockCircleOutlined />
                <Text>Job History</Text>
              </>
            )}
          </Space>
        }
      >
        {activeTab === 'current' ? (
          <Table
            dataSource={filterJobs(currentJobs)}
            columns={currentJobsColumns}
            rowKey={(record) => record.job_id || record.id}
            pagination={{ pageSize: 10 }}
            locale={{
              emptyText: (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description="No current jobs found"
                />
              ),
            }}
          />
        ) : (
          <Table
            dataSource={filterJobs(jobHistory, true)}
            columns={historyColumns}
            rowKey={(record) => record.id}
            pagination={{ pageSize: 15 }}
            locale={{
              emptyText: (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description="No job history found"
                />
              ),
            }}
          />
        )}
      </Card>
    </div>
  );
};

export default JobStatusMonitor;