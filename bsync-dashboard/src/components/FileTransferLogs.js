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
  Spin,
  message,
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
} from '@ant-design/icons';
import { useWebSocket } from '../contexts/WebSocketContext';
import axios from 'axios';

const { Title, Text } = Typography;
const { Option } = Select;

const FileTransferLogs = () => {
  const {
    fileTransferLogs: realTimeLogs,
    connected,
    formatFileSize,
    clearFileTransferLogs,
  } = useWebSocket();

  const [filterStatus, setFilterStatus] = useState('all');
  const [filterJob, setFilterJob] = useState('all');
  const [searchText, setSearchText] = useState('');
  const [viewMode, setViewMode] = useState('table'); // 'table' or 'list'
  const [historicalLogs, setHistoricalLogs] = useState([]);
  const [loading, setLoading] = useState(true);

  // Combined logs: historical + real-time (avoid duplicates)
  const fileTransferLogs = React.useMemo(() => {
    const combinedLogs = [...historicalLogs];
    
    // Add real-time logs that aren't already in historical logs
    realTimeLogs.forEach(realTimeLog => {
      const exists = historicalLogs.some(histLog => 
        histLog.id === realTimeLog.id || 
        (histLog.file === realTimeLog.fileName && 
         histLog.time === realTimeLog.timestamp)
      );
      if (!exists) {
        // Convert real-time log format to match new API format
        combinedLogs.push({
          id: realTimeLog.id,
          file: realTimeLog.fileName,
          file_size: realTimeLog.fileSize,
          status: realTimeLog.status,
          job: realTimeLog.jobName,
          source_target: realTimeLog.agentName,
          action: realTimeLog.action || '',
          duration: realTimeLog.duration || 0,
          time: realTimeLog.timestamp,
          error_message: realTimeLog.error || '',
        });
      }
    });

    return combinedLogs.sort((a, b) => new Date(b.time) - new Date(a.time));
  }, [historicalLogs, realTimeLogs]);

  // Load historical data from API
  const loadHistoricalData = async () => {
    try {
      setLoading(true);
      const token = localStorage.getItem('token');
      const response = await axios.get('/api/v1/file-transfer-logs', {
        headers: {
          'Authorization': `Bearer ${token}`,
        },
        params: {
          limit: 100, // Load recent 100 logs
        }
      });
      
      if (response.data && response.data.logs) {
        setHistoricalLogs(response.data.logs);
      }
    } catch (error) {
      console.error('Failed to load historical data:', error);
      message.error('Failed to load historical file transfer logs');
    } finally {
      setLoading(false);
    }
  };

  // Load data on component mount
  useEffect(() => {
    loadHistoricalData();
  }, []);

  // Get unique job names for filter
  const uniqueJobs = [...new Set(fileTransferLogs.map(log => log.job || log.jobName).filter(Boolean))];
  
  // Get file transfer stats
  const getTransferStats = () => {
    const total = fileTransferLogs.length;
    const successful = fileTransferLogs.filter(log => log.status === 'success').length;
    const failed = fileTransferLogs.filter(log => log.status === 'failed').length;
    const totalSize = fileTransferLogs
      .filter(log => log.status === 'success')
      .reduce((sum, log) => sum + (log.file_size || log.fileSize || 0), 0);
    
    return { total, successful, failed, totalSize };
  };

  const stats = getTransferStats();

  // Filter logs based on search, status, and job
  const filteredLogs = fileTransferLogs.filter(log => {
    const fileName = log.file || log.fileName;
    const jobName = log.job || log.jobName;
    const sourceTarget = log.source_target || log.agentName;
    
    const matchesSearch = fileName?.toLowerCase().includes(searchText.toLowerCase()) ||
                         jobName?.toLowerCase().includes(searchText.toLowerCase()) ||
                         sourceTarget?.toLowerCase().includes(searchText.toLowerCase());
    
    const matchesStatus = filterStatus === 'all' || log.status === filterStatus;
    const matchesJob = filterJob === 'all' || jobName === filterJob;
    
    return matchesSearch && matchesStatus && matchesJob;
  });

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
    return status === 'success' ? 'success' : 'error';
  };

  const getStatusIcon = (status) => {
    return status === 'success' ? <CheckCircleOutlined /> : <ExclamationCircleOutlined />;
  };

  const columns = [
    {
      title: 'File',
      dataIndex: 'file',
      key: 'file',
      render: (fileName, record) => {
        const displayName = fileName || record.fileName;
        const fileSize = record.file_size || record.fileSize || 0;
        return (
          <Space>
            {getFileIcon(displayName)}
            <div>
              <Text strong={record.status === 'failed'} type={record.status === 'failed' ? 'danger' : undefined}>
                {displayName}
              </Text>
              <div>
                <Text type="secondary" style={{ fontSize: '12px' }}>
                  {formatFileSize(fileSize)}
                </Text>
              </div>
            </div>
          </Space>
        );
      },
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status, record) => {
        const errorMsg = record.error_message || record.error;
        return (
          <Tag
            icon={getStatusIcon(status)}
            color={getStatusColor(status)}
          >
            {status.toUpperCase()}
            {errorMsg && (
              <Tooltip title={errorMsg}>
                <WarningOutlined style={{ marginLeft: 4 }} />
              </Tooltip>
            )}
          </Tag>
        );
      },
    },
    {
      title: 'Job',
      dataIndex: 'job',
      key: 'job', 
      render: (jobName, record) => {
        const displayName = jobName || record.jobName;
        return <Tag color="blue">{displayName}</Tag>;
      },
    },
    {
      title: 'Source → Target',
      dataIndex: 'source_target',
      key: 'source_target',
      render: (sourceTarget, record) => {
        const displayName = sourceTarget || record.agentName;
        return <Tag>{displayName}</Tag>;
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
      dataIndex: 'file_size',
      key: 'file_size',
      render: (size, record) => {
        const displaySize = size || record.fileSize || 0;
        return (
          <Text type="secondary">
            {displaySize ? formatFileSize(displaySize) : '-'}
          </Text>
        );
      },
    },
    {
      title: 'Duration',
      dataIndex: 'duration',
      key: 'duration',
      render: (duration) => {
        if (!duration || duration === 0) return <Text type="secondary">-</Text>;
        return (
          <Text type="secondary">
            {duration.toFixed(2)}s
          </Text>
        );
      },
    },
    {
      title: 'Time',
      dataIndex: 'time',
      key: 'time',
      render: (timestamp, record) => {
        const displayTime = timestamp || record.timestamp;
        return (
          <Text type="secondary">
            {displayTime ? new Date(displayTime).toLocaleTimeString() : '-'}
          </Text>
        );
      },
    },
  ];

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
              value={stats.total}
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
        <Row justify="space-between" align="middle">
          <Col>
            <Space wrap>
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
              </Select>
              
              <Select
                value={filterJob}
                onChange={setFilterJob}
                style={{ width: 150 }}
                suffixIcon={<FilterOutlined />}
              >
                <Option value="all">All Jobs</Option>
                {uniqueJobs.map(jobName => (
                  <Option key={jobName} value={jobName}>{jobName}</Option>
                ))}
              </Select>

              <Button.Group>
                <Button
                  type={viewMode === 'table' ? 'primary' : 'default'}
                  onClick={() => setViewMode('table')}
                >
                  Table
                </Button>
                <Button
                  type={viewMode === 'list' ? 'primary' : 'default'}
                  onClick={() => setViewMode('list')}
                >
                  List
                </Button>
              </Button.Group>
            </Space>
          </Col>
          
          <Col>
            <Space>
              <Badge status={connected ? 'success' : 'error'} text={`Real-time: ${connected ? 'On' : 'Off'}`} />
              <Button
                icon={<ReloadOutlined />}
                onClick={loadHistoricalData}
                loading={loading}
              >
                Refresh
              </Button>
              <Button
                icon={<ClearOutlined />}
                onClick={clearFileTransferLogs}
                disabled={fileTransferLogs.length === 0}
              >
                Clear Real-time
              </Button>
            </Space>
          </Col>
        </Row>
      </Card>

      {/* File Transfer Logs */}
      <Card
        title={
          <Space>
            <FileOutlined />
            <Text>File Transfer Logs</Text>
            <Badge count={filteredLogs.length} showZero />
          </Space>
        }
        loading={loading}
      >
        {viewMode === 'table' ? (
          <Table
            dataSource={filteredLogs}
            columns={columns}
            rowKey={(record) => record.id}
            pagination={{ 
              pageSize: 20,
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total, range) => `${range[0]}-${range[1]} of ${total} transfers`
            }}
            rowClassName={(record) => record.status === 'failed' ? 'failed-transfer-row' : ''}
            locale={{
              emptyText: (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description="No file transfer logs found"
                />
              ),
            }}
          />
        ) : (
          <List
            dataSource={filteredLogs}
            pagination={{ 
              pageSize: 15,
              showSizeChanger: true 
            }}
            renderItem={(item) => {
              const fileName = item.file || item.fileName;
              const jobName = item.job || item.jobName;
              const sourceTarget = item.source_target || item.agentName;
              const fileSize = item.file_size || item.fileSize || 0;
              const timestamp = item.time || item.timestamp;
              const errorMsg = item.error_message || item.error;
              const action = item.action;
              const duration = item.duration;
              
              return (
                <List.Item
                  className={item.status === 'failed' ? 'failed-transfer-item' : ''}
                >
                  <List.Item.Meta
                    avatar={
                      <Avatar 
                        icon={getFileIcon(fileName)}
                        style={{ 
                          backgroundColor: item.status === 'failed' ? '#ff4d4f' : '#52c41a',
                          color: 'white'
                        }}
                      />
                    }
                    title={
                      <Space>
                        <Text 
                          strong={item.status === 'failed'} 
                          type={item.status === 'failed' ? 'danger' : undefined}
                        >
                          {fileName}
                        </Text>
                        <Tag 
                          icon={getStatusIcon(item.status)}
                          color={getStatusColor(item.status)}
                          size="small"
                        >
                          {item.status.toUpperCase()}
                        </Tag>
                      </Space>
                    }
                    description={
                      <Space direction="vertical" size="small">
                        <Space size="large">
                          <Text type="secondary">Job: {jobName}</Text>
                          <Text type="secondary">Source → Target: {sourceTarget}</Text>
                          <Text type="secondary">Action: {action || '-'}</Text>
                          <Text type="secondary">Duration: {duration ? `${duration.toFixed(2)}s` : '-'}</Text>
                          <Text type="secondary">Size: {formatFileSize(fileSize)}</Text>
                          <Text type="secondary">Time: {timestamp ? new Date(timestamp).toLocaleTimeString() : '-'}</Text>
                        </Space>
                        {errorMsg && (
                          <Alert
                            type="error"
                            message={errorMsg}
                            size="small"
                            showIcon
                          />
                        )}
                      </Space>
                    }
                  />
                </List.Item>
              );
            }}
            locale={{
              emptyText: (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description="No file transfer logs found"
                />
              ),
            }}
          />
        )}
      </Card>

      <style jsx>{`
        .failed-transfer-row {
          background-color: #fff1f0 !important;
          border-left: 3px solid #ff4d4f;
        }
        
        .failed-transfer-item {
          background-color: #fff1f0;
          border-left: 3px solid #ff4d4f;
          margin-bottom: 8px;
          border-radius: 4px;
        }
      `}</style>
    </div>
  );
};

export default FileTransferLogs;