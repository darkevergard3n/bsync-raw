import React, { useState, useEffect } from 'react';
import {
  Card,
  Table,
  Tag,
  Space,
  Typography,
  Tooltip,
  Button,
  Select,
  Input,
  Alert,
  Badge,
  Row,
  Col,
  Statistic,
  Empty,
  message,
  Spin,
} from 'antd';
import {
  FileOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  SearchOutlined,
  FilterOutlined,
  ReloadOutlined,
  WarningOutlined,
  FileTextOutlined,
  FileImageOutlined,
  FileZipOutlined,
  FilePdfOutlined,
  FileExcelOutlined,
  FileWordOutlined,
  FilePptOutlined,
  DownloadOutlined,
  ClockCircleOutlined,
  SwapOutlined,
  ArrowRightOutlined,
  DatabaseOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { useWebSocket } from '../contexts/WebSocketContext';
import axios from 'axios';
import moment from 'moment';

const { Title, Text } = Typography;
const { Option } = Select;

const FileTransferLogsSafe = () => {
  const {
    fileTransferLogs: realtimeLogs,
    connected,
    formatFileSize,
  } = useWebSocket();

  const [filterStatus, setFilterStatus] = useState('all');
  const [filterJob, setFilterJob] = useState('all');
  const [searchText, setSearchText] = useState('');
  const [loading, setLoading] = useState(false);
  const [dbLogs, setDbLogs] = useState([]);
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 20, // Increase page size to see more logs
    total: 0,
  });
  const [dataSource, setDataSource] = useState('database');
  const [uniqueJobs, setUniqueJobs] = useState([]);

  // Fetch logs from database - SAFE VERSION without redirect
  const fetchLogsFromDatabase = async (page = 1, pageSize = 10) => {
    setLoading(true);
    try {
      const params = {
        limit: pageSize,
        offset: (page - 1) * pageSize,
      };

      // Add filters
      if (filterStatus !== 'all') {
        params.status = filterStatus;
      }
      if (filterJob !== 'all') {
        params.job_name = filterJob;
      }
      if (searchText) {
        params.file_name = searchText;
      }

      const token = localStorage.getItem('synctool-token'); // Use correct token name
      if (!token) {
        console.warn('[FileTransferLogsSafe] No token found, skipping log fetch');
        setDbLogs([]);
        setPagination({ current: 1, pageSize: 10, total: 0 });
        return; // SAFE: Don't redirect, just return empty
      }

      const response = await axios.get('/api/v1/file-transfer-logs', { 
        params,
        headers: {
          'Authorization': `Bearer ${token}`,
        }
      });
      
      const logs = response.data.logs.map(log => {
        // Debug: log the first record to see API response format
        if (response.data.logs.indexOf(log) === 0) {
          console.log('[FileTransferLogsSafe] API Response Sample:', log);
        }
        // Calculate duration if not provided
        let duration = log.transfer_duration;
        if (!duration && log.started_at && log.completed_at) {
          const startTime = new Date(log.started_at);
          const endTime = new Date(log.completed_at);
          duration = (endTime - startTime) / 1000; // Convert to seconds
        }
        
        return {
          id: log.id,
          fileName: log.file_name || log.file,
          filePath: log.file_path,
          fileSize: log.file_size || 0,
          status: log.status,
          error: log.error_message,
          jobId: log.job_id,
          jobName: log.job_name || log.job,
          sourceAgentId: log.source_agent_id,
          sourceAgentName: log.source_agent_name,
          targetAgentId: log.target_agent_id,
          targetAgentName: log.target_agent_name,
          sourceTarget: log.source_target, // New field from API
          timestamp: log.started_at || log.time,
          completedAt: log.completed_at,
          transferDuration: log.transfer_duration || duration || log.duration, // Try multiple duration fields
          direction: log.direction,
          action: log.action,
        };
      });

      setDbLogs(logs);
      setPagination({
        current: page,
        pageSize: pageSize,
        total: response.data.total,
      });

      // Extract unique job names
      const jobNames = [...new Set(logs.map(log => log.jobName).filter(Boolean))];
      setUniqueJobs(jobNames);

    } catch (error) {
      console.error('Failed to fetch logs from database:', error);
      if (error.response?.status === 401) {
        console.warn('[FileTransferLogsSafe] Authentication failed, clearing logs');
        setDbLogs([]);
        setPagination({ current: 1, pageSize: 10, total: 0 });
        // SAFE: Don't redirect or show error message, just log silently
      } else {
        message.error(`Failed to load file transfer logs: ${error.response?.data?.error || error.message}`);
      }
    } finally {
      setLoading(false);
    }
  };

  // Initial data load
  useEffect(() => {
    if (dataSource === 'database') {
      fetchLogsFromDatabase(1, 20);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Run once on mount

  // Fetch logs when filters change
  useEffect(() => {
    if (dataSource === 'database') {
      fetchLogsFromDatabase(pagination.current, pagination.pageSize);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterStatus, filterJob, searchText, dataSource]);

  // Get logs based on data source
  const logs = dataSource === 'realtime' ? realtimeLogs : dbLogs;

  // Get unique job names for filter (realtime mode)
  useEffect(() => {
    if (dataSource === 'realtime') {
      const jobNames = [...new Set(realtimeLogs.map(log => log.jobName).filter(Boolean))];
      setUniqueJobs(jobNames);
    }
  }, [realtimeLogs, dataSource]);

  // Filter logs (for realtime mode only, database mode filters on server)
  const filteredLogs = dataSource === 'realtime' ? logs.filter(log => {
    const matchesSearch = log.fileName?.toLowerCase().includes(searchText.toLowerCase()) ||
                         log.jobName?.toLowerCase().includes(searchText.toLowerCase()) ||
                         log.sourceAgentName?.toLowerCase().includes(searchText.toLowerCase()) ||
                         log.targetAgentName?.toLowerCase().includes(searchText.toLowerCase());
    
    const matchesStatus = filterStatus === 'all' || log.status === filterStatus;
    const matchesJob = filterJob === 'all' || log.jobName === filterJob;
    
    return matchesSearch && matchesStatus && matchesJob;
  }) : logs;

  // Get file transfer stats
  const getTransferStats = () => {
    const logsToCount = dataSource === 'realtime' ? filteredLogs : logs;
    const total = logsToCount.length;
    const successful = logsToCount.filter(log => log.status === 'success').length;
    const failed = logsToCount.filter(log => log.status === 'failed').length;
    const transferring = logsToCount.filter(log => log.status === 'transferring').length;
    const totalSize = logsToCount
      .filter(log => log.status === 'success')
      .reduce((sum, log) => sum + (log.fileSize || 0), 0);
    
    return { total, successful, failed, transferring, totalSize };
  };

  const stats = getTransferStats();

  const getFileIcon = (fileName) => {
    const ext = fileName?.split('.').pop()?.toLowerCase();
    const iconMap = {
      pdf: <FilePdfOutlined style={{ color: '#ff4d4f' }} />,
      jpg: <FileImageOutlined style={{ color: '#52c41a' }} />,
      jpeg: <FileImageOutlined style={{ color: '#52c41a' }} />,
      png: <FileImageOutlined style={{ color: '#52c41a' }} />,
      gif: <FileImageOutlined style={{ color: '#52c41a' }} />,
      xlsx: <FileExcelOutlined style={{ color: '#52c41a' }} />,
      xls: <FileExcelOutlined style={{ color: '#52c41a' }} />,
      docx: <FileWordOutlined style={{ color: '#1890ff' }} />,
      doc: <FileWordOutlined style={{ color: '#1890ff' }} />,
      pptx: <FilePptOutlined style={{ color: '#fa8c16' }} />,
      ppt: <FilePptOutlined style={{ color: '#fa8c16' }} />,
      zip: <FileZipOutlined style={{ color: '#722ed1' }} />,
      rar: <FileZipOutlined style={{ color: '#722ed1' }} />,
      txt: <FileTextOutlined />,
      json: <FileTextOutlined />,
      ini: <FileTextOutlined />,
    };
    
    return iconMap[ext] || <FileOutlined />;
  };

  const getStatusColor = (status) => {
    const colors = {
      success: 'success',
      failed: 'error',
      transferring: 'processing',
      pending: 'default',
    };
    return colors[status] || 'default';
  };

  const getStatusIcon = (status) => {
    const icons = {
      success: <CheckCircleOutlined />,
      failed: <ExclamationCircleOutlined />,
      transferring: <ClockCircleOutlined />,
      pending: <ClockCircleOutlined />,
    };
    return icons[status] || <ClockCircleOutlined />;
  };

  const formatDuration = (seconds) => {
    if (!seconds || seconds === 0) return '-';
    
    // Handle very small durations (milliseconds)
    if (seconds < 1) {
      const milliseconds = Math.round(seconds * 1000);
      return `${milliseconds}ms`;
    }
    
    if (seconds < 60) {
      return `${seconds.toFixed(2)}s`;
    } else if (seconds < 3600) {
      const mins = Math.floor(seconds / 60);
      const secs = Math.round(seconds % 60);
      return `${mins}m ${secs}s`;
    } else {
      const hours = Math.floor(seconds / 3600);
      const mins = Math.floor((seconds % 3600) / 60);
      return `${hours}h ${mins}m`;
    }
  };

  const columns = [
    {
      title: 'File',
      dataIndex: 'fileName',
      key: 'fileName',
      render: (fileName, record) => (
        <Space>
          {getFileIcon(fileName)}
          <Text strong={record.status === 'failed'} type={record.status === 'failed' ? 'danger' : undefined}>
            {fileName}
          </Text>
        </Space>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status, record) => (
        <Tag
          icon={getStatusIcon(status)}
          color={getStatusColor(status)}
        >
          {status.toUpperCase()}
          {record.error && (
            <Tooltip title={record.error}>
              <WarningOutlined style={{ marginLeft: 4 }} />
            </Tooltip>
          )}
        </Tag>
      ),
    },
    {
      title: 'Job',
      dataIndex: 'jobName',
      key: 'jobName',
      render: (jobName) => <Tag color="blue">{jobName}</Tag>,
    },
    {
      title: 'Source → Target',
      key: 'agents',
      render: (_, record) => {
        // Parse the source→target from API
        if (record.sourceTarget) {
          // Check for two-way sync (contains ↔)
          if (record.sourceTarget.includes('↔')) {
            const [source, target] = record.sourceTarget.split(' ↔ ');
            return (
              <Space size="small">
                <Tag color="cyan">{source}</Tag>
                <SwapOutlined />
                <Tag color="green">{target}</Tag>
              </Space>
            );
          }
          // Check for one-way sync (contains →)
          else if (record.sourceTarget.includes('→')) {
            const [source, target] = record.sourceTarget.split(' → ');
            return (
              <Space size="small">
                <Tag color="cyan">{source}</Tag>
                <ArrowRightOutlined />
                <Tag color="green">{target}</Tag>
              </Space>
            );
          }
          // Single agent name
          else {
            return (
              <Space size="small">
                <Tag color="cyan">{record.sourceTarget}</Tag>
              </Space>
            );
          }
        }
        
        // Fallback: Build from individual agent names (for compatibility)
        let sourceAgent = record.sourceAgentName;
        let targetAgent = record.targetAgentName;
        
        return (
          <Space size="small">
            <Tag color="cyan">{sourceAgent}</Tag>
            {targetAgent && (
              <>
                <ArrowRightOutlined />
                <Tag color="green">{targetAgent}</Tag>
              </>
            )}
          </Space>
        );
      },
    },
    {
      title: 'Action',
      dataIndex: 'action',
      key: 'action',
      render: (action) => {
        if (!action) return <Text type="secondary">-</Text>;
        const color = action === 'update' ? 'blue' : action === 'delete' ? 'red' : 'default';
        return <Tag color={color}>{action.toUpperCase()}</Tag>;
      },
    },
    {
      title: 'Size',
      dataIndex: 'fileSize',
      key: 'fileSize',
      render: (fileSize) => (
        <Text type="secondary">
          {formatFileSize(fileSize || 0)}
        </Text>
      ),
    },
    {
      title: 'Duration',
      dataIndex: 'transferDuration',
      key: 'transferDuration',
      render: (duration) => (
        <Text type="secondary">
          {formatDuration(duration)}
        </Text>
      ),
    },
    {
      title: 'Time',
      dataIndex: 'timestamp',
      key: 'timestamp',
      render: (timestamp, record) => (
        <Tooltip title={`Started: ${moment(timestamp).format('YYYY-MM-DD HH:mm:ss')}${
          record.completedAt ? `\nCompleted: ${moment(record.completedAt).format('YYYY-MM-DD HH:mm:ss')}` : ''
        }`}>
          <Text type="secondary">
            {moment(timestamp).format('HH:mm:ss')}
          </Text>
        </Tooltip>
      ),
    },
  ];

  // Handle table pagination change
  const handleTableChange = (newPagination) => {
    if (dataSource === 'database') {
      fetchLogsFromDatabase(newPagination.current, newPagination.pageSize);
    }
  };

  // Count failed files for alert
  const failedCount = filteredLogs.filter(log => log.status === 'failed').length;

  return (
    <div>
      {/* Failed Files Alert */}
      {failedCount > 0 && (
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
          message={`${failedCount} file transfer${failedCount > 1 ? 's' : ''} failed`}
          description="Check the error details in the logs below"
          action={
            <Button
              size="small"
              danger
              onClick={() => setFilterStatus('failed')}
            >
              Show Failed Only
            </Button>
          }
        />
      )}

      {/* Transfer Statistics */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={6}>
          <Card size="small">
            <Statistic
              title="Total Transfers"
              value={dataSource === 'database' ? pagination.total : stats.total}
              prefix={<FileOutlined />}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Statistic
              title="Successful"
              value={stats.successful}
              prefix={<CheckCircleOutlined />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Statistic
              title="Failed"
              value={stats.failed}
              prefix={<ExclamationCircleOutlined />}
              valueStyle={{ color: '#ff4d4f' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card size="small">
            <Statistic
              title="Data Transferred"
              value={formatFileSize(stats.totalSize)}
              prefix={<FileOutlined />}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
      </Row>

      {/* Filters and Controls */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <Space wrap>
          {/* Data Source Toggle */}
          <Button.Group>
            <Button
              type={dataSource === 'realtime' ? 'primary' : 'default'}
              onClick={() => setDataSource('realtime')}
              icon={<SyncOutlined />}
            >
              Real-time
            </Button>
            <Button
              type={dataSource === 'database' ? 'primary' : 'default'}
              onClick={() => setDataSource('database')}
              icon={<DatabaseOutlined />}
            >
              Historical
            </Button>
          </Button.Group>

          <Input
            placeholder="Search files, jobs, or agents..."
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
            <Option value="success">Success</Option>
            <Option value="failed">Failed</Option>
            <Option value="transferring">Transferring</Option>
          </Select>
          
          <Select
            value={filterJob}
            onChange={setFilterJob}
            style={{ width: 150 }}
            suffixIcon={<FilterOutlined />}
            placeholder="Select job"
          >
            <Option value="all">All Jobs</Option>
            {uniqueJobs.map(jobName => (
              <Option key={jobName} value={jobName}>{jobName}</Option>
            ))}
          </Select>

          {dataSource === 'database' && (
            <Button
              icon={<ReloadOutlined />}
              onClick={() => fetchLogsFromDatabase(1, pagination.pageSize)}
              loading={loading}
            >
              Refresh
            </Button>
          )}
        </Space>
        
        <div style={{ marginTop: 8 }}>
          <Badge 
            status={dataSource === 'realtime' ? (connected ? 'success' : 'error') : 'default'} 
            text={dataSource === 'realtime' ? `Real-time: ${connected ? 'Connected' : 'Disconnected'}` : 'Historical Data'} 
          />
          {dataSource === 'realtime' && (
            <Text type="secondary" style={{ marginLeft: 16 }}>
              Showing last {filteredLogs.length} transfers
            </Text>
          )}
        </div>
      </Card>

      {/* File Transfer Logs Table */}
      <Card
        title={
          <Space>
            <FileOutlined />
            <Text>File Transfer Logs</Text>
            <Badge count={dataSource === 'database' ? pagination.total : filteredLogs.length} showZero />
          </Space>
        }
      >
        <Spin spinning={loading}>
          <Table
            dataSource={filteredLogs}
            columns={columns}
            rowKey={(record) => record.id}
            pagination={dataSource === 'database' ? {
              ...pagination,
              showSizeChanger: false,
              showQuickJumper: false,
              showTotal: (total, range) => `${range[0]}-${range[1]} of ${total} transfers`,
            } : {
              pageSize: 10,
              showSizeChanger: false,
              showQuickJumper: false,
              showTotal: (total, range) => `${range[0]}-${range[1]} of ${total} transfers`,
            }}
            onChange={handleTableChange}
            rowClassName={(record) => record.status === 'failed' ? 'failed-transfer-row' : ''}
            locale={{
              emptyText: (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={loading ? 'Loading...' : 'No file transfer logs found'}
                />
              ),
            }}
          />
        </Spin>
      </Card>

      <style jsx>{`
        .failed-transfer-row {
          background-color: #fff1f0 !important;
          border-left: 3px solid #ff4d4f;
        }
      `}</style>
    </div>
  );
};

export default FileTransferLogsSafe;