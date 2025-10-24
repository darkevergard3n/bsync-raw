import React, { useState, useEffect } from 'react';
import {
  Card,
  Row,
  Col,
  Typography,
  Space,
  Tag,
  Spin,
} from 'antd';
import {
  DesktopOutlined,
  FolderOutlined,
  SyncOutlined,
  CheckCircleOutlined,
  ExclamationCircleOutlined,
  ClockCircleOutlined,
  DatabaseOutlined,
} from '@ant-design/icons';
import { Bar } from 'react-chartjs-2';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Title as ChartTitle,
  Tooltip,
  Legend,
} from 'chart.js';
import api from '../services/api';

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  BarElement,
  ChartTitle,
  Tooltip,
  Legend
);

const { Title, Text } = Typography;

const Dashboard = () => {
  const [loading, setLoading] = useState(true);
  const [dashboardStats, setDashboardStats] = useState(null);
  const [dailyTransferStats, setDailyTransferStats] = useState([]);
  const [topJobsPerformance, setTopJobsPerformance] = useState(null);
  const [recentEvents, setRecentEvents] = useState([]);
  const [performanceTab, setPerformanceTab] = useState('files'); // 'files' or 'volume'

  useEffect(() => {
    fetchDashboardData();
  }, []);

  const fetchDashboardData = async () => {
    try {
      setLoading(true);
      console.log('[Dashboard] Fetching dashboard data...');

      // Fetch Dashboard Statistics
      try {
        const statsResponse = await api.get('/api/v1/dashboard/stats');
        if (statsResponse.data.success) {
          setDashboardStats(statsResponse.data.data);
          console.log('[Dashboard] Stats loaded:', statsResponse.data.data);
        }
      } catch (err) {
        console.error('[Dashboard] Failed to fetch stats:', err);
      }

      // Fetch Daily Transfer Statistics
      try {
        const dailyStatsResponse = await api.get('/api/v1/dashboard/daily-transfer-stats');
        if (dailyStatsResponse.data.success) {
          setDailyTransferStats(dailyStatsResponse.data.data);
          console.log('[Dashboard] Daily transfer stats loaded:', dailyStatsResponse.data.data.length);
        }
      } catch (err) {
        console.error('[Dashboard] Failed to fetch daily transfer stats:', err);
      }

      // Fetch Top Jobs Performance
      try {
        const topJobsResponse = await api.get('/api/v1/dashboard/top-jobs-performance');
        if (topJobsResponse.data.success) {
          setTopJobsPerformance(topJobsResponse.data.data);
          console.log('[Dashboard] Top jobs performance loaded');
        }
      } catch (err) {
        console.error('[Dashboard] Failed to fetch top jobs performance:', err);
      }

      // Fetch Recent File Transfer Events
      try {
        const eventsResponse = await api.get('/api/v1/dashboard/recent-events');
        if (eventsResponse.data.success) {
          setRecentEvents(eventsResponse.data.data);
          console.log('[Dashboard] Recent events loaded:', eventsResponse.data.data.length);
        }
      } catch (err) {
        console.error('[Dashboard] Failed to fetch recent events:', err);
      }

    } catch (error) {
      console.error('Failed to fetch dashboard data:', error);
    } finally {
      setLoading(false);
      console.log('Dashboard loading complete');
    }
  };

  // Chart data for files transferred - bar chart data from API
  const filesTransferredData = {
    labels: dailyTransferStats.map(stat => stat.day_name || 'N/A'),
    datasets: [
      {
        label: 'Files Transferred',
        data: dailyTransferStats.map(stat => stat.file_count || 0),
        backgroundColor: '#07be63',
        borderRadius: 8,
        barThickness: 40,
      },
    ],
  };

  // Helper function to format bytes to human readable
  const formatBytes = (bytes) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
  };

  // Top sync jobs data from API
  const getTopSyncJobs = () => {
    if (!topJobsPerformance) return [];

    const jobs = performanceTab === 'files'
      ? topJobsPerformance.top_jobs_by_file_count || []
      : topJobsPerformance.top_jobs_by_data_size || [];

    return jobs.slice(0, 5).map((job, index) => {
      const maxFiles = jobs[0]?.total_files_transferred || 1;
      const maxBytes = jobs[0]?.total_bytes_transferred || 1;
      const progress = performanceTab === 'files'
        ? Math.round((job.total_files_transferred / maxFiles) * 100)
        : Math.round((job.total_bytes_transferred / maxBytes) * 100);

      return {
        rank: index + 1,
        name: job.job_name,
        files: job.total_files_transferred,
        volume: formatBytes(job.total_bytes_transferred),
        progress: progress
      };
    });
  };

  const topSyncJobs = getTopSyncJobs();

  // Helper function to format duration
  const formatDuration = (seconds) => {
    if (!seconds || seconds === 0) return '<1s';
    if (seconds < 60) return `${Math.round(seconds)}s`;
    const mins = Math.floor(seconds / 60);
    const secs = Math.round(seconds % 60);
    return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`;
  };

  // Helper function to format timestamp
  const formatTimestamp = (dateString) => {
    if (!dateString) return 'N/A';
    const now = new Date();
    const date = new Date(dateString);
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins} min ago`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`;
    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays} day${diffDays > 1 ? 's' : ''} ago`;
  };

  // Recent file transfers data from API
  const recentTransfers = recentEvents.map(event => ({
    id: event.id,
    filename: event.file_name,
    job: event.job_name,
    source: event.source_agent_name,
    target: event.destination_agent_name,
    size: formatBytes(event.file_size),
    duration: event.status === 'completed' ? formatDuration(event.duration) : 'timeout',
    timestamp: formatTimestamp(event.completed_at || event.started_at),
    status: event.status === 'completed' ? 'success' : 'error'
  }));


  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '400px' }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div>
      <Title level={2}>Dashboard</Title>
      <Text type="secondary">Welcome to BSync P2P Management System</Text>

      {/* Key Metrics */}
      <Row gutter={[16, 16]} style={{ marginTop: 24 }}>
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
                <DesktopOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Total Agents
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {dashboardStats?.total_agents || 0}
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
                <SyncOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Active Jobs
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {dashboardStats?.total_active_jobs || 0}
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
                <FolderOutlined style={{ fontSize: '18px', color: '#10b981' }} />
              </div>
              <div>
                <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                  Total Files
                </Text>
                <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                  {(dashboardStats?.total_files || 0).toLocaleString()}
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
                  {formatBytes(dashboardStats?.total_data_transferred || 0)}
                </div>
              </div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* Charts Row */}
      <Row gutter={[16, 16]} style={{ marginTop: 24 }}>
        <Col xs={24} lg={12}>
          <Card
            style={{
              borderRadius: '12px',
              border: '1px solid #e5e7eb',
              background: '#ffffff',
              height: '100%'
            }}
            styles={{ body: { padding: '24px' } }}
          >
            <div style={{ marginBottom: '20px' }}>
              <Title level={3} style={{ margin: 0, fontSize: '18px', fontWeight: 600, color: '#1f2937' }}>
                Files Transferred (7 Days)
              </Title>
              <Text type="secondary" style={{ fontSize: '13px', color: '#6b7280' }}>
                Total files successfully transferred per day
              </Text>
            </div>
            <div style={{ height: '320px' }}>
              <Bar
                data={filesTransferredData}
                options={{
                  responsive: true,
                  maintainAspectRatio: false,
                  plugins: {
                    legend: {
                      display: false,
                    },
                    tooltip: {
                      backgroundColor: '#ffffff',
                      titleColor: '#1f2937',
                      bodyColor: '#6b7280',
                      borderColor: '#e5e7eb',
                      borderWidth: 1,
                      padding: 12,
                      displayColors: false,
                      callbacks: {
                        label: function(context) {
                          return context.parsed.y.toLocaleString() + ' files';
                        }
                      }
                    }
                  },
                  scales: {
                    y: {
                      beginAtZero: true,
                      max: 2000,
                      ticks: {
                        stepSize: 500,
                        color: '#9ca3af',
                        font: {
                          size: 11,
                        },
                        callback: function(value) {
                          return value.toLocaleString();
                        }
                      },
                      grid: {
                        color: '#f3f4f6',
                        drawBorder: false,
                      },
                      border: {
                        display: false,
                      }
                    },
                    x: {
                      ticks: {
                        color: '#6b7280',
                        font: {
                          size: 12,
                          weight: 500,
                        }
                      },
                      grid: {
                        display: false,
                        drawBorder: false,
                      },
                      border: {
                        display: false,
                      }
                    }
                  },
                }}
              />
            </div>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card
            style={{
              borderRadius: '12px',
              border: '1px solid #e5e7eb',
              background: '#ffffff',
              height: '100%'
            }}
            styles={{ body: { padding: '24px' } }}
          >
            <div style={{ marginBottom: '20px' }}>
              <Title level={3} style={{ margin: 0, fontSize: '18px', fontWeight: 600, color: '#1f2937' }}>
                Top Sync Jobs Performance
              </Title>
              <Text type="secondary" style={{ fontSize: '13px', color: '#6b7280' }}>
                Highest performing jobs in last 30 days
              </Text>
            </div>

            {/* Toggle Tabs */}
            <div style={{
              display: 'flex',
              gap: '0',
              marginBottom: '20px'
            }}>
              <div
                onClick={() => setPerformanceTab('files')}
                style={{
                  flex: 1,
                  height: '40px',
                  borderRadius: '8px 0 0 8px',
                  background: performanceTab === 'files' ? '#07be63' : 'transparent',
                  border: performanceTab === 'files' ? '1px solid #07be63' : '1px solid #e5e7eb',
                  borderRight: performanceTab === 'files' ? '1px solid #07be63' : 'none',
                  color: performanceTab === 'files' ? '#ffffff' : '#6b7280',
                  fontWeight: 500,
                  fontSize: '13px',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  cursor: 'pointer',
                  transition: 'all 0.3s ease'
                }}
              >
                <FolderOutlined />
                Most Files
              </div>
              <div
                onClick={() => setPerformanceTab('volume')}
                style={{
                  flex: 1,
                  height: '40px',
                  borderRadius: '0 8px 8px 0',
                  background: performanceTab === 'volume' ? '#07be63' : 'transparent',
                  border: performanceTab === 'volume' ? '1px solid #07be63' : '1px solid #e5e7eb',
                  borderLeft: performanceTab === 'volume' ? '1px solid #07be63' : 'none',
                  color: performanceTab === 'volume' ? '#ffffff' : '#6b7280',
                  fontWeight: 500,
                  fontSize: '13px',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  cursor: 'pointer',
                  transition: 'all 0.3s ease'
                }}
              >
                <DatabaseOutlined />
                Data Volume
              </div>
            </div>

            {/* Job List */}
            <Space direction="vertical" style={{ width: '100%' }} size={12}>
              {topSyncJobs.map((job) => (
                <div
                  key={job.rank}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '12px'
                  }}
                >
                  <div
                    style={{
                      width: '24px',
                      height: '24px',
                      borderRadius: '50%',
                      background: '#07be63',
                      color: '#ffffff',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: '12px',
                      fontWeight: 600,
                      flexShrink: 0
                    }}
                  >
                    {job.rank}
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{
                      fontSize: '13px',
                      fontWeight: 500,
                      color: '#1f2937',
                      marginBottom: '4px',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis'
                    }}>
                      {job.name}
                    </div>
                    <div style={{
                      width: '100%',
                      height: '6px',
                      background: '#e5e7eb',
                      borderRadius: '3px',
                      overflow: 'hidden'
                    }}>
                      <div style={{
                        width: `${job.progress}%`,
                        height: '100%',
                        background: '#07be63',
                        borderRadius: '3px',
                        transition: 'width 0.3s ease'
                      }} />
                    </div>
                  </div>
                  <div style={{
                    fontSize: '14px',
                    fontWeight: 600,
                    color: '#07be63',
                    textAlign: 'right',
                    minWidth: '80px',
                    flexShrink: 0
                  }}>
                    {performanceTab === 'files'
                      ? `${job.files.toLocaleString()} files`
                      : job.volume
                    }
                  </div>
                </div>
              ))}
            </Space>
          </Card>
        </Col>
      </Row>

      {/* Recent Activity */}
      <Row gutter={[16, 16]} style={{ marginTop: 24 }}>
        <Col xs={24}>
          <Card
            style={{
              borderRadius: '12px',
              border: '1px solid #e5e7eb',
              background: '#ffffff'
            }}
            styles={{ body: { padding: '24px' } }}
          >
            <div style={{ marginBottom: '20px' }}>
              <Title level={3} style={{ margin: 0, fontSize: '18px', fontWeight: 600, color: '#1f2937' }}>
                Recent Activity
              </Title>
              <Text type="secondary" style={{ fontSize: '13px', color: '#6b7280' }}>
                Latest file transfer operations
              </Text>
            </div>

            <Space direction="vertical" style={{ width: '100%' }} size={16}>
              {recentTransfers.map((transfer) => (
                <div
                  key={transfer.id}
                  style={{
                    padding: '16px',
                    background: transfer.status === 'error' ? '#fef2f2' : '#f0fdf4',
                    borderRadius: '12px',
                    border: `1px solid ${transfer.status === 'error' ? '#fee2e2' : '#dcfce7'}`
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
                    {/* Status Icon */}
                    <div style={{
                      width: '40px',
                      height: '40px',
                      borderRadius: '50%',
                      background: transfer.status === 'error' ? '#fecaca' : '#bbf7d0',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      flexShrink: 0
                    }}>
                      {transfer.status === 'success' ? (
                        <CheckCircleOutlined style={{ fontSize: '20px', color: '#16a34a' }} />
                      ) : (
                        <ExclamationCircleOutlined style={{ fontSize: '20px', color: '#dc2626' }} />
                      )}
                    </div>

                    {/* Content */}
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px' }}>
                        <FolderOutlined style={{ fontSize: '14px', color: '#6b7280' }} />
                        <Text strong style={{ fontSize: '14px', color: '#1f2937' }}>
                          {transfer.filename}
                        </Text>
                      </div>

                      <div style={{
                        display: 'flex',
                        flexWrap: 'wrap',
                        gap: '16px',
                        fontSize: '13px',
                        color: '#6b7280'
                      }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                          <Text style={{ color: '#6b7280', fontSize: '13px' }}>Job:</Text>
                          <Tag
                            style={{
                              background: '#e7f5ed',
                              border: '1px solid #07be63',
                              color: '#07be63',
                              fontSize: '12px',
                              padding: '0 8px',
                              margin: 0
                            }}
                          >
                            {transfer.job}
                          </Tag>
                        </div>

                        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                          <DatabaseOutlined style={{ fontSize: '12px' }} />
                          <Text style={{ fontSize: '13px' }}>Source:</Text>
                          <Text strong style={{ fontSize: '13px', color: '#1f2937' }}>{transfer.source}</Text>
                        </div>

                        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                          <DatabaseOutlined style={{ fontSize: '12px' }} />
                          <Text style={{ fontSize: '13px' }}>Target:</Text>
                          <Text strong style={{ fontSize: '13px', color: '#1f2937' }}>{transfer.target}</Text>
                        </div>

                        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                          <FolderOutlined style={{ fontSize: '12px' }} />
                          <Text style={{ fontSize: '13px' }}>Size:</Text>
                          <Text strong style={{ fontSize: '13px', color: '#1f2937' }}>{transfer.size}</Text>
                        </div>

                        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                          <SyncOutlined style={{ fontSize: '12px' }} />
                          <Text style={{ fontSize: '13px' }}>Duration:</Text>
                          <Text strong style={{ fontSize: '13px', color: '#1f2937' }}>{transfer.duration}</Text>
                        </div>
                      </div>
                    </div>

                    {/* Timestamp */}
                    <div style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: '4px',
                      color: '#9ca3af',
                      fontSize: '12px',
                      flexShrink: 0
                    }}>
                      <ClockCircleOutlined />
                      {transfer.timestamp}
                    </div>
                  </div>
                </div>
              ))}
            </Space>
          </Card>
        </Col>
      </Row>

    </div>
  );
};

export default Dashboard;