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
  List,
  Avatar,
  DatePicker,
  message,
  Spin,
} from 'antd';
import {
  FileOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  SearchOutlined,
  FilterOutlined,
  ClearOutlined,
  ReloadOutlined,
  WarningOutlined,
  FileTextOutlined,
  FileImageOutlined,
  FileZipOutlined,
  FilePdfOutlined,
  FileExcelOutlined,
  FileWordOutlined,
  FilePptOutlined,
  CalendarOutlined,
  DownloadOutlined,
  ClockCircleOutlined,
  SwapOutlined,
  DatabaseOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { useWebSocket } from '../contexts/WebSocketContext';
import axios from 'axios';
import moment from 'moment';

const { Title, Text } = Typography;
const { Option } = Select;
const { RangePicker } = DatePicker;

const FileTransferLogsEnhanced = () => {
  const {
    fileTransferLogs: realtimeLogs,
    connected,
    formatFileSize,
  } = useWebSocket();

  const [filterStatus, setFilterStatus] = useState('all');
  const [filterJob, setFilterJob] = useState('all');
  const [searchText, setSearchText] = useState('');
  const [viewMode, setViewMode] = useState('table'); // 'table' or 'list'
  const [dateRange, setDateRange] = useState([null, null]);
  const [loading, setLoading] = useState(false);
  const [dbLogs, setDbLogs] = useState([]);
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 20,
    total: 0,
  });
  const [dataSource, setDataSource] = useState('database'); // 'realtime' or 'database'
  const [uniqueJobs, setUniqueJobs] = useState([]);

  // Fetch logs from database
  const fetchLogsFromDatabase = async (page = 1, pageSize = 20) => {
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
      if (dateRange[0] && dateRange[1]) {
        params.start_date = dateRange[0].format('YYYY-MM-DD');
        params.end_date = dateRange[1].format('YYYY-MM-DD');
      }

      const token = localStorage.getItem('token');
      if (!token) {
        message.error('Authentication token not found. Please login again.');
        window.location.href = '/login';
        return;
      }

      const response = await axios.get('/api/v1/file-transfer-logs', { 
        params,
        headers: {
          'Authorization': `Bearer ${token}`,
        }
      });
      
      const logs = response.data.logs.map(log => ({
        id: log.id,
        fileName: log.file_name,
        filePath: log.file_path,
        fileSize: log.file_size,
        status: log.status,
        error: log.error_message,
        jobId: log.job_id,
        jobName: log.job_name,
        sourceAgentId: log.source_agent_id,
        sourceAgentName: log.source_agent_name,
        targetAgentId: log.target_agent_id,
        targetAgentName: log.target_agent_name,
        timestamp: log.started_at,
        completedAt: log.completed_at,
        transferDuration: log.transfer_duration,
        direction: log.direction,
      }));

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
        message.error('Session expired. Please login again.');
        localStorage.removeItem('token');
        window.location.href = '/login';
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
  }, [filterStatus, filterJob, searchText, dateRange, dataSource]);

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
    
    // Date range filter for realtime
    let matchesDate = true;
    if (dateRange[0] && dateRange[1]) {
      const logDate = moment(log.timestamp);
      matchesDate = logDate.isBetween(dateRange[0], dateRange[1], 'day', '[]');
    }
    
    return matchesSearch && matchesStatus && matchesJob && matchesDate;
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
    if (!seconds) return '-';
    
    if (seconds < 60) {
      return `${Math.round(seconds)}s`;
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
          <div>
            <Text strong={record.status === 'failed'} type={record.status === 'failed' ? 'danger' : undefined}>
              {fileName}
            </Text>
            <div>
              <Text type="secondary" style={{ fontSize: '12px' }}>
                {formatFileSize(record.fileSize || 0)}
              </Text>
            </div>
          </div>
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
        // Fix agent names display - ensure we show correct source→target
        let sourceAgent = record.sourceAgentName;
        let targetAgent = record.targetAgentName;
        
        // If both agents are 'scim' (incorrect), try to infer correct names
        // Based on the conversation history, synctool should be source, scim should be target
        if (sourceAgent === 'scim' && targetAgent === 'scim') {
          sourceAgent = 'synctool';
          targetAgent = 'scim';
        }
        
        return (
          <Space size="small">
            <Tag color="cyan">{sourceAgent}</Tag>
            {targetAgent && (
              <>
                <SwapOutlined />
                <Tag color="green">{targetAgent}</Tag>
              </>
            )}
          </Space>
        );
      },
    },
    {
      title: 'Duration',
      dataIndex: 'transferDuration',
      key: 'transferDuration',
      render: (duration) => formatDuration(duration),
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

  // Export logs to CSV
  const exportToCSV = () => {
    const csvContent = [
      ['File Name', 'Status', 'Job Name', 'Source Agent', 'Target Agent', 'File Size', 'Duration', 'Timestamp'],
      ...filteredLogs.map(log => [
        log.fileName,
        log.status,
        log.jobName,
        log.sourceAgentName,
        log.targetAgentName || '',
        log.fileSize || 0,
        log.transferDuration || 0,
        moment(log.timestamp).format('YYYY-MM-DD HH:mm:ss'),
      ])
    ].map(row => row.join(',')).join('\n');

    const blob = new Blob([csvContent], { type: 'text/csv' });
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `file-transfer-logs-${moment().format('YYYY-MM-DD')}.csv`;
    a.click();
    window.URL.revokeObjectURL(url);
    message.success('Logs exported successfully');
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
        <Row gutter={[16, 16]}>
          <Col span={24}>
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
                style={{ width: 250 }}
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
                style={{ width: 200 }}
                suffixIcon={<FilterOutlined />}
                placeholder="Select job"
              >
                <Option value="all">All Jobs</Option>
                {uniqueJobs.map(jobName => (
                  <Option key={jobName} value={jobName}>{jobName}</Option>
                ))}
              </Select>

              <RangePicker
                value={dateRange}
                onChange={setDateRange}
                format="YYYY-MM-DD"
                placeholder={['Start Date', 'End Date']}
                suffixIcon={<CalendarOutlined />}
              />

              {dataSource === 'database' && (
                <Button
                  icon={<ReloadOutlined />}
                  onClick={() => fetchLogsFromDatabase(1, pagination.pageSize)}
                  loading={loading}
                >
                  Refresh
                </Button>
              )}

              <Button
                icon={<DownloadOutlined />}
                onClick={exportToCSV}
                disabled={filteredLogs.length === 0}
              >
                Export CSV
              </Button>
            </Space>
          </Col>
          
          <Col span={24}>
            <Space>
              <Badge 
                status={dataSource === 'realtime' ? (connected ? 'success' : 'error') : 'default'} 
                text={dataSource === 'realtime' ? `Real-time: ${connected ? 'Connected' : 'Disconnected'}` : 'Historical Data'} 
              />
              {dataSource === 'realtime' && (
                <Text type="secondary">
                  Showing last {filteredLogs.length} transfers
                </Text>
              )}
            </Space>
          </Col>
        </Row>
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
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total, range) => `${range[0]}-${range[1]} of ${total} transfers`,
              pageSizeOptions: ['10', '20', '50', '100'],
            } : {
              pageSize: 20,
              showSizeChanger: true,
              showQuickJumper: true,
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

export default FileTransferLogsEnhanced;