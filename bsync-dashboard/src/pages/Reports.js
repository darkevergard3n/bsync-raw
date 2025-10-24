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
  Row,
  Col,
  message,
  ConfigProvider,
} from 'antd';
import { useNavigate } from 'react-router-dom';
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
  ClockCircleOutlined,
  SwapOutlined,
  ArrowRightOutlined,
  DatabaseOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import moment from 'moment';
import { reportsAPI } from '../services/api';

const { Title, Text } = Typography;
const { Option } = Select;

const Reports = () => {
  const navigate = useNavigate();
  

  const [filterStatus, setFilterStatus] = useState([]);
  const [filterJob, setFilterJob] = useState([]);
  const [filterAction, setFilterAction] = useState([]);
  const [searchText, setSearchText] = useState('');
  const [debouncedSearchText, setDebouncedSearchText] = useState('');
  
  // Debounce search text
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchText(searchText);
    }, 500);
    return () => clearTimeout(timer);
  }, [searchText]);
  const [loading, setLoading] = useState(false);
  const [dbLogs, setDbLogs] = useState([]);
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 5, // Increase page size to see more logs
    total: 0,
    completed: 0,
  });
  const [uniqueJobs, setUniqueJobs] = useState([]);
  const [uniqueActions, setUniqueActions] = useState([]);
  const [uniqueStatuses, setUniqueStatuses] = useState([]);
  const [totalStats, setTotalStats] = useState({
    total: 0,
    successful: 0,
    totalDuration: 0,
    totalSize: 0
  });

  // Fetch transfer statistics using new dedicated API
  const fetchTransferStats = async () => {
    try {
      const filters = {};

      // Apply same filters for consistent stats
      if (filterStatus.length > 0) {
        filters.status = filterStatus.join(',');
      }
      if (filterJob.length > 0) {
        filters.job_name = filterJob.join(',');
      }
      if (filterAction.length > 0) {
        filters.action = filterAction.join(',');
      }
      if (searchText) {
        filters.search = searchText;
      }

      console.log('[Reports] Fetching transfer stats with filters:', filters);

      const response = await reportsAPI.getTransferStats(filters);
      
      const statsData = response.data?.data || {};
      setTotalStats({
        total: statsData.total_transfers || 0,
        successful: statsData.completed_transfers || 0,
        totalDuration: statsData.total_duration || 0,
        totalSize: statsData.total_data_size || 0,
        successRate: statsData.success_rate || 0,
        avgDuration: statsData.average_duration || 0,
      });
      
      console.log('[Reports] Successfully updated transfer stats:', statsData);
    } catch (error) {
      console.error('[Reports] Failed to fetch transfer stats:', error);
      message.error('Failed to load transfer statistics');
    }
  };

  // Fetch filter options using new dedicated API
  const fetchFilterOptions = async () => {
    try {
      console.log('[Reports] Fetching filter options...');

      const response = await reportsAPI.getFilterOptions();
      
      if (response.data && response.data.data) {
        const optionsData = response.data.data;
        console.log('[Reports] Filter options received:', optionsData);
        
        // Extract statuses
        const statuses = (optionsData.statuses || []).map(item => item.value).sort();
        setUniqueStatuses(statuses);
        
        // Extract jobs
        const jobs = (optionsData.jobs || []).map(item => item.value).sort();
        setUniqueJobs(jobs);
        
        // Extract actions
        const actions = (optionsData.actions || []).map(item => item.value).sort();
        setUniqueActions(actions);
        
        console.log('[Reports] Successfully loaded filter options:', {
          statuses: statuses.length,
          jobs: jobs.length, 
          actions: actions.length
        });
        
      } else {
        console.warn('[Reports] Invalid filter options response:', response.data);
        setUniqueJobs([]);
        setUniqueActions([]);
        setUniqueStatuses([]);
      }
    } catch (error) {
      console.error('[Reports] Failed to fetch filter options:', error);
      message.error('Failed to load filter options');
      setUniqueJobs([]);
      setUniqueActions([]);
      setUniqueStatuses([]);
    }
  };

  // Fetch logs from database - EXACT COPY from FileTransferLogsSafe
  const fetchLogsFromDatabase = async (page = 1, pageSize = 10) => {
    setLoading(true);
    try {
      const params = {
        limit: pageSize,
        offset: (page - 1) * pageSize,
      };

      // Add filters - convert arrays to comma-separated strings for API
      if (filterStatus.length > 0) {
        params.status = filterStatus.join(',');
      }
      if (filterJob.length > 0) {
        params.job_name = filterJob.join(',');
      }
      if (filterAction.length > 0) {
        params.action = filterAction.join(',');
      }
      if (searchText) {
        params.file_name = searchText;
      }

      console.log('[Reports] API call params:', params);
      console.log('[Reports] Filter states:', { filterStatus, filterJob, filterAction, searchText });
      console.log('[Reports] Params object keys:', Object.keys(params));
      console.log('[Reports] Params object values:', Object.values(params));
      console.log('[Reports] Number of params being sent:', Object.keys(params).length);

      const response = await reportsAPI.getFileTransferLogs(params);
      console.log('[Reports] API response received, data count:', response.data?.logs?.length || 0);
      
      // Handle empty or invalid response
      const rawLogs = response.data?.logs || [];
      if (!Array.isArray(rawLogs)) {
        console.warn('[Reports] Invalid logs data received:', response.data);
        setDbLogs([]);
        setPagination(prev => ({ ...prev, current: page, total: 0 }));
        return;
      }
      
      const logs = rawLogs.map(log => {
        // Debug: log the first record to see API response format
        if (rawLogs.indexOf(log) === 0) {
          console.log('[Reports] API Response Sample:', log);
        }
        // Calculate duration if not provided
        let duration = log.transfer_duration;
        if (!duration && log.started_at && log.completed_at) {
          const startTime = new Date(log.started_at);
          const endTime = new Date(log.completed_at);
          duration = (endTime - startTime) / 1000; // Convert to seconds
        }
        
        // Debug the first log entry to verify new API fields
        if (rawLogs.indexOf(log) === 0) {
          console.log('[Reports] API Response Sample (with JOIN):', {
            source_agent_name: log.source_agent_name,
            destination_agent_name: log.destination_agent_name,
            source_target: log.source_target,
            sync_mode: log.sync_mode
          });
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
          sourceAgentName: log.source_agent_name,
          targetAgentName: log.destination_agent_name,
          sourceTarget: log.source_target, // Now provided directly by API
          timestamp: log.started_at || log.time,
          completedAt: log.completed_at,
          transferDuration: log.transfer_duration || duration || log.duration,
          direction: log.direction,
          action: log.action,
        };
      });

      setDbLogs(logs);
      setPagination(prev => ({
        ...prev,
        current: page,
        total: response.data?.total || logs.length,
        completed: response.data?.total_completed || logs.filter(log => log.status === 'completed').length,
      }));

      console.log(`[Reports] Loaded ${logs.length} database logs`);
      
      // Also fetch transfer stats when data changes
      fetchTransferStats();
    } catch (error) {
      console.error('[Reports] Failed to fetch logs from database:', error);
      setDbLogs([]);
      setPagination({ current: 1, pageSize: 10, total: 0 });
      
      if (error.response?.status !== 401) {
        message.error(`Failed to load file transfer logs: ${error.response?.data?.error || error.message}`);
      }
    } finally {
      setLoading(false);
    }
  };

  // Fetch job details for source/destination mapping
  // Load filter options on component mount (only once)
  useEffect(() => {
    console.log('[Reports] Component mounted, loading initial data');
    fetchFilterOptions();
  }, []);

  // Load logs and stats when filters change (reset to page 1)
  useEffect(() => {
    console.log('[Reports] Loading data - filters or mount');
    // Reset to page 1 when filters change
    if (pagination.current !== 1) {
      setPagination(prev => ({ ...prev, current: 1 }));
      fetchLogsFromDatabase(1, pagination.pageSize);
    } else {
      fetchLogsFromDatabase(pagination.current, pagination.pageSize);
    }
    // Also refresh stats when filters change
    fetchTransferStats();
  }, [filterStatus, filterJob, filterAction, debouncedSearchText]);

  // Use totalStats from dedicated API instead of calculating from current page logs
  const stats = totalStats;
  
  // Format functions for display
  const formatFileSize = (bytes) => {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  // Get file type icon - EXACT COPY from FileTransferLogsSafe
  const getFileTypeIcon = (fileName) => {
    if (!fileName) return <FileOutlined />;
    
    const ext = fileName.split('.').pop()?.toLowerCase();
    switch (ext) {
      case 'txt': case 'md': case 'log': return <FileTextOutlined />;
      case 'jpg': case 'jpeg': case 'png': case 'gif': case 'svg': return <FileImageOutlined />;
      case 'zip': case 'rar': case '7z': case 'tar': case 'gz': return <FileZipOutlined />;
      case 'pdf': return <FilePdfOutlined />;
      case 'xls': case 'xlsx': case 'csv': return <FileExcelOutlined />;
      case 'doc': case 'docx': return <FileWordOutlined />;
      case 'ppt': case 'pptx': return <FilePptOutlined />;
      default: return <FileOutlined />;
    }
  };

  // Helper functions for rendering - copied from Dashboard
  const getFileIcon = (fileName) => {
    return getFileTypeIcon(fileName);
  };

  const getStatusIcon = (status) => {
    switch(status) {
      case 'completed': return <CheckCircleOutlined />;
      case 'failed': return <ExclamationCircleOutlined />;
      case 'transferring': return <SyncOutlined spin />;
      default: return <CheckCircleOutlined />;
    }
  };

  const getStatusColor = (status) => {
    switch(status) {
      case 'completed': return 'success';
      case 'failed': return 'error';
      case 'transferring': return 'processing';
      case 'downloading': return 'processing';
      case 'started': return 'processing';
      default: return 'default';
    }
  };

  // Helper function to get status display info for filters
  const getStatusDisplayInfo = (status) => {
    switch(status) {
      case 'completed': 
        return { icon: <CheckCircleOutlined style={{ color: '#52c41a' }} />, label: 'Completed' };
      case 'failed': 
        return { icon: <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />, label: 'Failed' };
      case 'transferring': 
        return { icon: <SyncOutlined style={{ color: '#1890ff' }} />, label: 'Transferring' };
      case 'downloading': 
        return { icon: <SyncOutlined style={{ color: '#1890ff' }} />, label: 'Downloading' };
      case 'started': 
        return { icon: <ClockCircleOutlined style={{ color: '#faad14' }} />, label: 'Started' };
      default: 
        return { icon: <CheckCircleOutlined style={{ color: '#d9d9d9' }} />, label: status.charAt(0).toUpperCase() + status.slice(1) };
    }
  };

  const formatDuration = (duration) => {
    if (!duration || duration === 0) return '-';
    
    if (duration < 1) {
      return `${(duration * 1000).toFixed(1)}ms`;
    } else if (duration < 60) {
      return `${duration.toFixed(1)}s`;
    } else {
      return `${(duration / 60).toFixed(1)}m`;
    }
  };

  // TABLE COLUMNS with sort indicators
  const columns = [
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          File
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'fileName',
      key: 'fileName',
      width: 280,
      ellipsis: true,
      render: (fileName, record) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {getFileIcon(fileName)}
          <Text strong={record.status === 'failed'} type={record.status === 'failed' ? 'danger' : undefined}>
            {fileName}
          </Text>
        </div>
      ),
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Status
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'status',
      key: 'status',
      width: 130,
      align: 'center',
      render: (status, record) => {
        if (status === 'completed') {
          return (
            <div style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '6px',
              backgroundColor: '#dcfce7',
              color: '#166534',
              padding: '4px 12px',
              borderRadius: '16px',
              fontSize: '12px',
              fontWeight: '500'
            }}>
              <CheckCircleOutlined style={{ fontSize: '14px' }} />
              Completed
            </div>
          );
        }
        
        return (
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
        );
      },
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Job
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'jobName',
      key: 'jobName',
      width: 180,
      align: 'center',
      render: (jobName) => (
        <div 
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            backgroundColor: '#f3f4f6',
            color: '#374151',
            padding: '6px 24px',
            borderRadius: '16px',
            fontSize: '12px',
            fontWeight: '500',
            cursor: 'pointer'
          }}
          onClick={() => navigate('/jobs')}
        >
          {jobName}
        </div>
      ),
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Source → Target
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      key: 'agents',
      width: 220,
      align: 'center',
      render: (_, record) => {
        // Parse the source→target from our enhanced data
        if (record.sourceTarget) {
          // Check for two-way sync (contains ↔)
          if (record.sourceTarget.includes('↔')) {
            const [source, target] = record.sourceTarget.split(' ↔ ');
            return (
              <div style={{ color: '#6b7280', fontSize: '13px' }}>
                <span>{source}</span>
                <SwapOutlined style={{ margin: '0 4px', color: '#9ca3af' }} />
                <span>{target}</span>
              </div>
            );
          }
          // Check for one-way sync (contains →)
          else if (record.sourceTarget.includes('→')) {
            const [source, target] = record.sourceTarget.split(' → ');
            return (
              <div style={{ color: '#6b7280', fontSize: '13px' }}>
                <span>{source}</span>
                <ArrowRightOutlined style={{ margin: '0 4px', color: '#9ca3af' }} />
                <span>{target}</span>
              </div>
            );
          }
          // Single agent name (fallback)
          else {
            return (
              <span style={{ color: '#6b7280', fontSize: '13px' }}>
                {record.sourceTarget}
              </span>
            );
          }
        }
        
        // Fallback: No source/target info available
        return (
          <Text type="secondary">-</Text>
        );
      },
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Action
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'action',
      key: 'action',
      width: 110,
      align: 'center',
      render: (action) => {
        if (!action) return <Text type="secondary">-</Text>;
        
        if (action === 'update') {
          return (
            <div style={{
              display: 'inline-flex',
              alignItems: 'center',
              backgroundColor: '#dcfce7',
              color: '#166534',
              padding: '4px 12px',
              borderRadius: '16px',
              fontSize: '12px',
              fontWeight: '500'
            }}>
              Update
            </div>
          );
        }
        
        if (action === 'delete') {
          return (
            <div style={{
              display: 'inline-flex',
              alignItems: 'center',
              backgroundColor: '#fee2e2',
              color: '#dc2626',
              padding: '4px 12px',
              borderRadius: '16px',
              fontSize: '12px',
              fontWeight: '500'
            }}>
              Delete
            </div>
          );
        }
        
        // For other actions (metadata, etc.), use default Tag
        const color = action === 'metadata' ? 'orange' : 'default';
        return <Tag color={color}>{action.toUpperCase()}</Tag>;
      },
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Size
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'fileSize',
      key: 'fileSize',
      width: 110,
      align: 'center',
      render: (fileSize) => (
        <Text type="secondary">
          {formatFileSize(fileSize || 0)}
        </Text>
      ),
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Duration
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'transferDuration',
      key: 'transferDuration',
      width: 110,
      align: 'center',
      render: (duration) => (
        <Text type="secondary">
          {formatDuration(duration)}
        </Text>
      ),
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Timestamp
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 170,
      align: 'center',
      render: (timestamp, record) => (
        <Tooltip title={`Started: ${moment(timestamp).format('YYYY-MM-DD HH:mm:ss')}${
          record.completedAt ? `\nCompleted: ${moment(record.completedAt).format('YYYY-MM-DD HH:mm:ss')}` : ''
        }`}>
          <Text type="secondary">
            {moment(timestamp).format('YYYY-MM-DD HH:mm:ss')}
          </Text>
        </Tooltip>
      ),
    },
  ];

  // Handle table pagination change
  const handleTableChange = (paginationInfo) => {
    console.log('[Reports] Table change:', paginationInfo);
    const newPagination = {
      current: paginationInfo.current,
      pageSize: paginationInfo.pageSize,
      total: pagination.total,
    };
    setPagination(newPagination);
    fetchLogsFromDatabase(paginationInfo.current, paginationInfo.pageSize);
  };

  // Manual refresh
  const refreshData = () => {
    fetchLogsFromDatabase(pagination.current, pagination.pageSize);
    fetchTransferStats(); // Also refresh stats
    fetchFilterOptions(); // Also refresh filter options
  };

  return (
    <>
      <style>
        {`
          .custom-table .ant-table-thead > tr > th {
            background-color: #fafafa !important;
            border-bottom: 1px solid #e5e7eb !important;
            font-weight: 600 !important;
            color: #374151 !important;
            font-size: 13px !important;
            padding: 16px 20px !important;
          }
          
          .custom-table .ant-table-tbody > tr:hover > td {
            background-color: #f9fafb !important;
          }
          
          .custom-table .ant-table-tbody > tr > td {
            border-bottom: 1px solid #f3f4f6 !important;
            padding: 16px 20px !important;
            font-size: 14px !important;
          }
          
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
        <div style={{ padding: '24px' }}>
          {/* Page Header */}
          <div style={{ marginBottom: 24 }}>
            <Title level={2} style={{ margin: 0, color: '#1f2937', fontWeight: 600 }}>
              File Transfer Reports
            </Title>
            <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280' }}>
              Monitor and analyze file transfer operations
            </Text>
          </div>

      {/* Transfer Statistics */}
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
                <FileOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Total Transfers
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {(stats.total || 0).toLocaleString()}
                </div>
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
                  Completed
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {(stats.successful || 0).toLocaleString()}
                </div>
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
                  Total Duration
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {formatDuration(stats.totalDuration || 0)}
                </div>
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
                <DatabaseOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Data Transferred
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {formatFileSize(stats.totalSize || 0)}
                </div>
              </div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* Table Header with Search and Filters */}
      <div style={{ 
        display: 'flex', 
        justifyContent: 'space-between', 
        alignItems: 'center',
        marginBottom: 16 
      }}>
        <Title level={3} style={{ margin: 0, color: '#1f2937', fontWeight: 600, fontSize: '20px' }}>
          File Transfer Logs
        </Title>
        
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Input
            placeholder="Search files, agents..."
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 280, fontSize: '14px' }}
            allowClear
          />

          <Select
            placeholder="All Status"
            style={{ width: 140, fontSize: '14px' }}
            allowClear
            value={filterStatus.length > 0 ? filterStatus : undefined}
            onChange={(value) => setFilterStatus(value ? [value] : [])}
          >
            {uniqueStatuses.map(status => (
              <Option key={status} value={status}>
                {status.charAt(0).toUpperCase() + status.slice(1)}
              </Option>
            ))}
          </Select>

          <Select
            placeholder="All Jobs"
            style={{ width: 140, fontSize: '14px' }}
            allowClear
            value={filterJob.length > 0 ? filterJob : undefined}
            onChange={(value) => setFilterJob(value ? [value] : [])}
          >
            {uniqueJobs.map(job => (
              <Option key={job} value={job}>
                {job}
              </Option>
            ))}
          </Select>

          <Select
            placeholder="All Actions"
            style={{ width: 140, fontSize: '14px' }}
            allowClear
            value={filterAction.length > 0 ? filterAction : undefined}
            onChange={(value) => setFilterAction(value ? [value] : [])}
          >
            {uniqueActions.map(action => (
              <Option key={action} value={action}>
                {action.charAt(0).toUpperCase() + action.slice(1)}
              </Option>
            ))}
          </Select>
        </div>
      </div>

      {/* File Transfer Logs Table */}
      <Card
        style={{ 
          borderRadius: '12px', 
          border: '1px solid #e5e7eb',
          background: '#ffffff'
        }}
        styles={{ body: { padding: '24px' } }}
      >
        <Table
          className="custom-table"
          columns={columns}
          dataSource={dbLogs}
          loading={loading}
          pagination={false}
          rowKey="id"
          size="small"
          scroll={{ x: 1200 }}
          style={{ width: '100%', tableLayout: 'fixed' }}
        />
        
        {/* Custom Pagination */}
        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          paddingTop: '20px',
          borderTop: '1px solid #f3f4f6',
          marginTop: '20px'
        }}>
          <span style={{ color: '#6b7280', fontSize: '13px' }}>
            Showing {((pagination.current - 1) * pagination.pageSize) + 1} to {Math.min(pagination.current * pagination.pageSize, pagination.total)} of {pagination.total} entries
          </span>
          <div style={{ display: 'flex', alignItems: 'center', gap: '24px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ color: '#6b7280', fontSize: '13px' }}>Rows per page</span>
              <select 
                style={{
                  border: '1px solid #d1d5db',
                  borderRadius: '8px',
                  padding: '4px 8px',
                  fontSize: '13px',
                  minWidth: '70px',
                  outline: 'none',
                  transition: 'all 0.2s ease'
                }}
                value={pagination.pageSize}
                onChange={(e) => {
                  const newPageSize = parseInt(e.target.value);
                  setPagination(prev => ({ ...prev, pageSize: newPageSize, current: 1 }));
                  fetchLogsFromDatabase(1, newPageSize);
                }}
              >
                <option value={5}>5</option>
                <option value={10}>10</option>
                <option value={25}>25</option>
                <option value={50}>50</option>
              </select>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
              <span style={{ color: '#6b7280', fontSize: '13px' }}>
                Page {pagination.current} of {Math.ceil(pagination.total / pagination.pageSize)}
              </span>
              <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <button 
                  style={{
                    border: '1px solid #d1d5db',
                    background: 'white',
                    padding: '6px 12px',
                    borderRadius: '6px',
                    color: '#374151',
                    cursor: pagination.current > 1 ? 'pointer' : 'not-allowed',
                    fontSize: '16px',
                    opacity: pagination.current > 1 ? 1 : 0.5
                  }}
                  disabled={pagination.current <= 1}
                  onClick={() => {
                    if (pagination.current > 1) {
                      const newPage = pagination.current - 1;
                      setPagination(prev => ({ ...prev, current: newPage }));
                      fetchLogsFromDatabase(newPage, pagination.pageSize);
                    }
                  }}
                >
                  ‹
                </button>
                <button style={{
                  border: '1px solid #10b981',
                  background: '#10b981',
                  padding: '6px 12px',
                  borderRadius: '6px',
                  color: 'white',
                  cursor: 'pointer',
                  fontSize: '13px',
                  fontWeight: 500
                }}>
                  {pagination.current}
                </button>
                <button 
                  style={{
                    border: '1px solid #d1d5db',
                    background: 'white',
                    padding: '6px 12px',
                    borderRadius: '6px',
                    color: '#374151',
                    cursor: pagination.current < Math.ceil(pagination.total / pagination.pageSize) ? 'pointer' : 'not-allowed',
                    fontSize: '16px',
                    opacity: pagination.current < Math.ceil(pagination.total / pagination.pageSize) ? 1 : 0.5
                  }}
                  disabled={pagination.current >= Math.ceil(pagination.total / pagination.pageSize)}
                  onClick={() => {
                    if (pagination.current < Math.ceil(pagination.total / pagination.pageSize)) {
                      const newPage = pagination.current + 1;
                      setPagination(prev => ({ ...prev, current: newPage }));
                      fetchLogsFromDatabase(newPage, pagination.pageSize);
                    }
                  }}
                >
                  ›
                </button>
              </div>
            </div>
          </div>
        </div>
        </Card>
        </div>
      </ConfigProvider>
    </>
  );
};

export default Reports;