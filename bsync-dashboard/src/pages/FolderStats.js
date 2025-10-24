import React, { useState, useEffect, useCallback, useRef } from 'react';
import {
  Card,
  Row,
  Col,
  Table,
  Button,
  Space,
  Typography,
  message,
  Modal,
  ConfigProvider,
  Input,
  Select
} from 'antd';
import {
  FolderOutlined,
  ReloadOutlined,
  DatabaseOutlined,
  ClockCircleOutlined,
  CheckCircleOutlined,
  LoadingOutlined,
  FileOutlined,
  BarChartOutlined,
  SearchOutlined
} from '@ant-design/icons';
import { fetchAPI, agentAPI } from '../services/api';

const { Title, Text } = Typography;
const { Search } = Input;
const { Option } = Select;

const FolderStats = () => {
  const [jobs, setJobs] = useState([]);
  const [agents, setAgents] = useState([]);
  const [folderStats, setFolderStats] = useState({});
  const [loading] = useState(false);
  const [loadingJobs, setLoadingJobs] = useState({}); // Track loading state per job
  const [selectedJob, setSelectedJob] = useState(null);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [expandedRowKeys, setExpandedRowKeys] = useState([]); // Track expanded rows
  const refreshIntervalsRef = useRef({}); // Use ref to avoid re-renders
  const [stats, setStats] = useState({
    activeJobs: 0,
    totalFiles: 0,
    dataVolume: 0,
    successRate: 0
  });
  const [searchText, setSearchText] = useState('');
  const [filterStatus, setFilterStatus] = useState('all');
  const [statusOptions, setStatusOptions] = useState([]);

  // Load jobs and agents on component mount
  useEffect(() => {
    loadJobsAndAgents();
    loadStatusOptions();
    fetchStats();
  }, []);

  // Reload data when search or filter changes
  useEffect(() => {
    console.log('🔄 Filter/Search changed:', { searchText, filterStatus });
    loadJobsAndAgents(searchText, filterStatus);
    fetchStats(searchText, filterStatus);
  }, [searchText, filterStatus]);

  // Fetch status options from API
  const loadStatusOptions = async () => {
    try {
      const response = await fetchAPI('/api/v1/master/sync-status');
      console.log('📋 Status options response:', response);

      if (response.data && Array.isArray(response.data)) {
        // Sort by order field
        const sortedStatuses = response.data.sort((a, b) => a.order - b.order);
        setStatusOptions(sortedStatuses);
        console.log('✅ Loaded status options:', sortedStatuses.length);
      }
    } catch (error) {
      console.error('Failed to load status options:', error);
      message.error('Failed to load status options');
    }
  };

  const loadJobsAndAgents = async (searchQuery = '', statusFilter = 'all') => {
    try {
      // Build query parameters for sync-jobs
      const params = new URLSearchParams();
      if (searchQuery) {
        params.append('search', searchQuery);
      }
      if (statusFilter && statusFilter !== 'all') {
        params.append('sync_status', statusFilter);
      }

      const queryString = params.toString();
      const jobsUrl = `/api/v1/sync-jobs${queryString ? `?${queryString}` : ''}`;

      console.log('📋 Fetching jobs from:', jobsUrl);
      const [jobsResponse, agentsResponse] = await Promise.all([
        fetchAPI(jobsUrl),
        agentAPI.list()
      ]);
      
      console.log('📋 Jobs response:', jobsResponse);
      console.log('👥 Agents response:', agentsResponse);

      // Transform sync jobs to match our expected format with real API data
      const transformedJobs = (jobsResponse.sync_jobs || []).map((job) => {
        // Handle both single destination (legacy) and multi-destination
        const isMultiDest = job.is_multi_destination && job.destinations && job.destinations.length > 0;
        const firstDest = isMultiDest ? job.destinations[0] : null;

        return {
          id: `job-${job.id}`,
          name: job.name,
          source_agent_id: job.source_agent_id,
          // For backward compatibility, use first destination or legacy field
          // Note: In multi-dest, the field is 'agent_id' not 'destination_agent_id'
          destination_agent_id: isMultiDest ? firstDest.agent_id : job.destination_agent_id,
          source_agent_name: job.source_agent_name,
          destination_agent_name: isMultiDest ? firstDest.agent_name : job.destination_agent_name,
          folder_id: `job-${job.id}`, // Use job ID as folder ID
          status: job.status,
          sync_mode: job.sync_mode,
          source_path: job.source_path,
          destination_path: isMultiDest ? firstDest.path : job.destination_path,
          // Multi-destination support
          is_multi_destination: isMultiDest,
          destinations: job.destinations || [],
          // Real API data
          progress: job.progress_percentage || 0,
          syncedFiles: job.progress_file || '0/0',
          syncStatus: job.sync_status || 'Pending',
          lastSync: job.last_synced || null,
          lastSyncedRaw: job.last_synced // Keep raw date for processing
        };
      });
      
      setJobs(transformedJobs);
      setAgents(agentsResponse.data?.data || []);

      console.log('✅ Loaded jobs:', transformedJobs.length);
      console.log('✅ Loaded agents:', agentsResponse.data?.data?.length || 0);
    } catch (error) {
      console.error('Failed to load data:', error);
      message.error('Failed to load data');
    }
  };

  // Fetch stats from API with filters
  const fetchStats = async (searchQuery = '', statusFilter = 'all') => {
    try {
      // Build query parameters
      const params = new URLSearchParams();
      if (searchQuery) {
        params.append('search', searchQuery);
      }
      if (statusFilter && statusFilter !== 'all') {
        params.append('sync_status', statusFilter);
      }

      const queryString = params.toString();
      const url = `/api/v1/folder-stats/stats${queryString ? `?${queryString}` : ''}`;

      console.log('📊 Fetching stats from:', url);
      const response = await fetchAPI(url);
      console.log('📊 Stats response:', response);

      if (response.data) {
        setStats({
          activeJobs: response.data.active_jobs || 0,
          totalFiles: response.data.total_files || 0,
          dataVolume: response.data.data_volume_bytes || 0,
          successRate: response.data.success_rate_value || 0
        });
        console.log('✅ Stats updated successfully');
      }
    } catch (error) {
      console.error('Failed to fetch stats:', error);
      message.error('Failed to load statistics');
    }
  };

  const fetchFolderStats = async (agentId, folderId, forceRefresh = false) => {
    console.log(`🔍 Fetching real folder stats for agent: ${agentId}, folder: ${folderId}, forceRefresh: ${forceRefresh}`);
    
    // Add timestamp to force refresh and bypass cache
    const timestamp = forceRefresh ? `&_t=${Date.now()}` : '';
    const url = `/api/folder-stats?agent_id=${agentId}&folder_id=${folderId}${timestamp}`;
    console.log(`📡 URL: ${url}`);
    
    const response = await fetchAPI(url);
    console.log('✅ Raw API response:', JSON.stringify(response, null, 2));
    
    // Extract the actual Syncthing stats from nested structure
    const actualStats = response.stats?.stats || response.stats || response;
    console.log('📊 Extracted stats:', JSON.stringify(actualStats, null, 2));
    console.log('🔍 Stats fields:', Object.keys(actualStats));
    console.log('📈 Key values:');
    console.log('  - globalFiles:', actualStats.globalFiles);
    console.log('  - globalBytes:', actualStats.globalBytes);
    console.log('  - state:', actualStats.state);
    
    const result = {
      agent_id: agentId,
      folder_id: folderId,
      stats: actualStats,
      success: true,
      lastUpdated: new Date().toISOString(),
      source: 'real_api'
    };
    
    console.log('🎯 Returning result:', JSON.stringify(result, null, 2));
    return result;
  };

  const refreshJobStats = useCallback(async (job) => {
    console.log('🔄 refreshJobStats called for job:', JSON.stringify(job, null, 2));

    // Set loading state only for this specific job
    setLoadingJobs(prev => ({ ...prev, [job.id]: true }));

    try {
      // Get source stats
      console.log(`📤 Fetching source stats: agent=${job.source_agent_id}, folder=${job.folder_id}`);
      const sourceStats = await fetchFolderStats(job.source_agent_id, job.folder_id, true);
      console.log('📤 Source stats result:', JSON.stringify(sourceStats, null, 2));

      // Get destination stats - handle both single and multi-destination
      let destinationsStats = [];

      if (job.is_multi_destination && job.destinations && job.destinations.length > 0) {
        // Multi-destination: fetch stats for all destinations
        console.log(`📥 Fetching stats for ${job.destinations.length} destinations`);

        const destStatsPromises = job.destinations.map(async (dest, index) => {
          console.log(`📥 Fetching dest ${index + 1} stats: agent=${dest.agent_id}, folder=${job.folder_id}`);
          const stats = await fetchFolderStats(dest.agent_id, job.folder_id, true);
          console.log(`📥 Dest ${index + 1} stats result:`, JSON.stringify(stats, null, 2));
          return {
            agent_id: dest.agent_id,
            agent_name: dest.agent_name,
            path: dest.path,
            stats: stats
          };
        });

        destinationsStats = await Promise.all(destStatsPromises);
        console.log(`✅ All ${destinationsStats.length} destination stats fetched`);
      } else {
        // Single destination (legacy)
        console.log(`📥 Fetching single dest stats: agent=${job.destination_agent_id}, folder=${job.folder_id}`);
        const destStats = await fetchFolderStats(job.destination_agent_id, job.folder_id, true);
        console.log('📥 Dest stats result:', JSON.stringify(destStats, null, 2));

        destinationsStats = [{
          agent_id: job.destination_agent_id,
          agent_name: job.destination_agent_name,
          path: job.destination_path,
          stats: destStats
        }];
      }

      const newFolderStats = {
        source: sourceStats,
        destinations: destinationsStats,
        // Keep legacy 'destination' field for backward compatibility (first destination)
        destination: destinationsStats[0]?.stats,
        is_multi_destination: job.is_multi_destination,
        lastUpdated: new Date().toISOString()
      };

      console.log(`💾 Storing folderStats[${job.id}]:`, JSON.stringify(newFolderStats, null, 2));

      setFolderStats(prev => ({
        ...prev,
        [job.id]: newFolderStats
      }));

      // COMMENTED OUT: Fast refresh logic is disabled - using manual refresh only
      // Fast refresh logic was handled by useEffect monitoring folderStats changes

    } catch (error) {
      console.error('Error refreshing stats:', error);
      message.error('Failed to refresh stats for job: ' + job.name);
    } finally {
      // Clear loading state only for this specific job
      setLoadingJobs(prev => ({ ...prev, [job.id]: false }));
    }
  }, []);

  // Handle expand/collapse row with auto-refresh on expand
  const handleExpandRow = (expanded, record) => {
    console.log(`🔄 Row ${expanded ? 'expanded' : 'collapsed'} for job: ${record.id}`);
    
    if (expanded) {
      // Auto-refresh when row is expanded
      console.log(`🚀 Auto-refreshing stats for expanded row: ${record.id}`);
      refreshJobStats(record);
      setExpandedRowKeys(prev => [...prev, record.id]);
    } else {
      // Only update UI state when collapsed, no API request
      console.log(`📥 Collapsing row without API request: ${record.id}`);
      setExpandedRowKeys(prev => prev.filter(key => key !== record.id));
    }
  };



  // Stop fast refresh interval for a specific job
  const stopFastRefresh = (job) => {
    if (refreshIntervalsRef.current[job.id]) {
      console.log(`⏹️ Stopping fast refresh for job ${job.id}`);
      clearInterval(refreshIntervalsRef.current[job.id]);
      const { [job.id]: removed, ...rest } = refreshIntervalsRef.current;
      refreshIntervalsRef.current = rest;
      console.log('⏹️ Updated refreshIntervals:', refreshIntervalsRef.current);
    }
  };

  // Monitor folderStats changes to automatically start/stop intervals
  // Only run auto-refresh when detail modal is open and for selected job
  // COMMENTED OUT: Auto-refresh interval functionality temporarily disabled for manual refresh only
  /*
  useEffect(() => {
    console.log('🔄 useEffect monitoring folderStats triggered');
    console.log('🔄 Detail modal visible:', detailModalVisible);
    console.log('🔄 Selected job:', selectedJob?.id);
    console.log('🔄 Current refreshIntervals:', refreshIntervalsRef.current);
    
    if (detailModalVisible && selectedJob) {
      const stats = folderStats[selectedJob.id];
      console.log(`🔄 Processing selected job ${selectedJob.id}, has stats:`, !!stats);
      
      if (stats) {
        const needsFastRefresh = checkIfNeedsFastRefresh(stats);
        const hasInterval = !!refreshIntervalsRef.current[selectedJob.id];
        
        console.log(`🔄 Job ${selectedJob.id} - needsFastRefresh: ${needsFastRefresh}, hasInterval: ${hasInterval}`);
        
        if (needsFastRefresh && !hasInterval) {
          console.log(`🔄 Starting interval for selected job ${selectedJob.id}`);
          startFastRefresh(selectedJob);
        } else if (!needsFastRefresh && hasInterval) {
          console.log(`🔄 Stopping interval for selected job ${selectedJob.id}`);
          stopFastRefresh(selectedJob);
        }
      }
    } else {
      // Stop all intervals when modal is closed
      console.log('🔄 Modal closed, stopping all intervals');
      Object.keys(refreshIntervalsRef.current).forEach(jobId => {
        const job = jobs.find(j => j.id === jobId);
        if (job) {
          stopFastRefresh(job);
        }
      });
    }
  }, [folderStats, jobs, detailModalVisible, selectedJob]); // Monitor modal state and selected job
  */

  // Cleanup intervals on component unmount
  useEffect(() => {
    return () => {
      console.log('🧹 Cleaning up all intervals:', refreshIntervalsRef.current);
      Object.values(refreshIntervalsRef.current).forEach(intervalId => {
        clearInterval(intervalId);
      });
      refreshIntervalsRef.current = {};
    };
  }, []); // Empty dependency array - only run on unmount

  const refreshAllStats = useCallback(async () => {
    // Refresh jobs list and stats with current filters
    await loadJobsAndAgents(searchText, filterStatus);
    await fetchStats(searchText, filterStatus);

    // Refresh individual job stats if there are jobs
    if (jobs.length > 0) {
      const promises = jobs.map(job => refreshJobStats(job));
      await Promise.all(promises);
    }
  }, [jobs, searchText, filterStatus, refreshJobStats]);

  // Auto refresh for active jobs
  // COMMENTED OUT: Auto-refresh interval temporarily disabled for manual refresh only
  /*
  useEffect(() => {
    let interval;
    if (autoRefresh && jobs.length > 0) {
      interval = setInterval(() => {
        refreshAllStats();
      }, 1000); // Refresh every 1 second for faster status updates
    }
    return () => clearInterval(interval);
  }, [autoRefresh, refreshAllStats]);
  */


  const formatFileSize = (bytes) => {
    if (!bytes) return '0 B';
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${sizes[i]}`;
  };

  // Format last sync time to human readable format
  const formatLastSync = (lastSyncDate) => {
    if (!lastSyncDate) return 'Never';

    const now = new Date();
    const syncTime = new Date(lastSyncDate);
    const diffMs = now - syncTime;
    const diffMins = Math.floor(diffMs / 60000);

    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins} minute${diffMins > 1 ? 's' : ''} ago`;

    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`;

    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) return `${diffDays} day${diffDays > 1 ? 's' : ''} ago`;

    const diffWeeks = Math.floor(diffDays / 7);
    if (diffWeeks < 4) return `${diffWeeks} week${diffWeeks > 1 ? 's' : ''} ago`;

    const diffMonths = Math.floor(diffDays / 30);
    return `${diffMonths} month${diffMonths > 1 ? 's' : ''} ago`;
  };


  const columns = [
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Job Name
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'name',
      key: 'name',
      width: 200,
      render: (text) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <FolderOutlined style={{ color: '#6b7280' }} />
          <Text strong style={{ color: '#1f2937' }}>{text}</Text>
        </div>
      ),
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Source Agent
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'source_agent_id',
      key: 'source_agent_id',
      width: 180,
      render: (agentId) => {
        const agent = agents.find(a => a.agent_id === agentId);
        return <span style={{ color: '#6b7280', fontSize: '14px' }}>{agent?.agent_id || agentId}</span>;
      },
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Destination Agent
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'destination_agent_id',
      key: 'destination_agent_id',
      width: 180,
      render: (agentId) => {
        const agent = agents.find(a => a.agent_id === agentId);
        return <span style={{ color: '#6b7280', fontSize: '14px' }}>{agent?.agent_id || agentId}</span>;
      },
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Progress
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'progress',
      key: 'progress',
      width: 180,
      render: (progress, record) => {
        const roundedProgress = Math.round(progress * 10) / 10; // Round to 1 decimal
        const progressColor = roundedProgress === 100 ? '#22c55e' : roundedProgress > 50 ? '#f59e0b' : '#ef4444';
        return (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{
                fontSize: '14px',
                fontWeight: 600,
                color: progressColor,
                minWidth: '45px'
              }}>
                {roundedProgress}%
              </span>
              <span style={{ fontSize: '12px', color: '#9ca3af' }}>
                ({record.syncedFiles})
              </span>
            </div>
          </div>
        );
      },
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
      dataIndex: 'syncStatus',
      key: 'syncStatus',
      width: 120,
      align: 'center',
      render: (status) => {
        const statusConfig = {
          'Complete': { bg: '#dcfce7', color: '#166534' },
          'Partial': { bg: '#fef3c7', color: '#92400e' },
          'Pending': { bg: '#f3f4f6', color: '#374151' },
        };
        const config = statusConfig[status] || statusConfig['Pending'];

        return (
          <span style={{
            backgroundColor: config.bg,
            color: config.color,
            padding: '4px 12px',
            borderRadius: '12px',
            fontSize: '12px',
            fontWeight: 500,
            display: 'inline-block'
          }}>
            {status}
          </span>
        );
      },
    },
    {
      title: (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          Last Sync
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderBottom: '4px solid #d1d5db' }}></div>
            <div style={{ width: 0, height: 0, borderLeft: '3px solid transparent', borderRight: '3px solid transparent', borderTop: '4px solid #d1d5db' }}></div>
          </div>
        </div>
      ),
      dataIndex: 'lastSyncedRaw',
      key: 'lastSync',
      width: 150,
      render: (lastSyncedRaw) => (
        <span style={{ color: '#6b7280', fontSize: '14px' }}>
          {formatLastSync(lastSyncedRaw)}
        </span>
      ),
    },
    /* Folder Stats column commented out - replaced with expandable rows
    {
      title: 'Folder Stats',
      key: 'stats',
      render: (_, record) => {
        const stats = folderStats[record.id];
        console.log(`🎨 Table render for job ${record.id}:`, JSON.stringify(stats, null, 2));
        
        if (!stats) {
          console.log(`❌ No stats found for job ${record.id}, showing Load Stats button`);
          return <Button icon={<EyeOutlined />} onClick={() => refreshJobStats(record)}>Load Stats</Button>;
        }
        
        const sourceStats = stats.source?.stats;
        console.log(`🎨 Source stats for rendering:`, JSON.stringify(sourceStats, null, 2));
        console.log(`🎨 Accessing: globalFiles=${sourceStats?.globalFiles}, globalBytes=${sourceStats?.globalBytes}, state=${sourceStats?.state}`);
        
        return (
          <Space direction="vertical" size="small" style={{ width: '100%' }}>
            <Space>
              <FileOutlined />
              <Text>{stats.source?.stats?.globalFiles || 0} files</Text>
              <DatabaseOutlined />
              <Text>{formatFileSize(stats.source?.stats?.globalBytes)}</Text>
            </Space>
            <Space>
              {getStatusBadge(stats.source?.stats?.state)}
              <Button size="small" icon={<EyeOutlined />} onClick={() => showDetailModal(record)}>
                Details
              </Button>
            </Space>
          </Space>
        );
      },
    },
    */
    {
      title: 'Actions',
      key: 'actions',
      render: (_, record) => (
        <Space size="middle">
          <Button
            type="text"
            size="small" 
            icon={<ReloadOutlined />}
            onClick={() => refreshJobStats(record)}
            title="Refresh"
            style={{ color: '#6b7280' }}
          />
        </Space>
      ),
    },
  ];

  const renderStatsCard = (title, stats, type = 'source') => {
    console.log('🎨 renderStatsCard called with:', { title, stats: JSON.stringify(stats, null, 2), type });
    
    if (!stats) {
      return (
        <div style={{
          backgroundColor: '#ffffff',
          borderRadius: '16px',
          padding: '24px',
          border: '1px solid #e5e7eb',
          textAlign: 'center'
        }}>
          <LoadingOutlined style={{ fontSize: '24px', color: '#9ca3af' }} />
          <div style={{ marginTop: '12px', color: '#6b7280' }}>Loading...</div>
        </div>
      );
    }

    // API response structure: { stats: { progress: {...}, stats: {...} } }
    const data = stats.stats || {};
    const locationText = type === 'source' ? 'Source Location' : 'Destination Location';
    const pathText = title.includes(':') ? title.split(': ')[1] : title;
    const timestamp = new Date(stats.lastUpdated || Date.now()).toLocaleTimeString();
    
    return (
      <div style={{
        backgroundColor: '#ffffff',
        borderRadius: '16px',
        border: '1px solid #e5e7eb',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
        height: '100%'
      }}>
        {/* Header - Green soft background */}
        <div style={{
          backgroundColor: '#f6fcf8',
          padding: '20px 24px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          minHeight: '100px'
        }}>
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px', flex: 1, minWidth: 0 }}>
            <div style={{
              width: '40px',
              height: '40px',
              backgroundColor: '#e3f7e9',
              borderRadius: '12px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0
            }}>
              <FolderOutlined style={{
                fontSize: '20px',
                color: '#22c55e'
              }} />
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{
                fontSize: '14px',
                fontWeight: '600',
                color: '#374151',
                marginBottom: '4px'
              }}>
                {locationText}
              </div>
              <div style={{
                fontSize: '13px',
                color: '#6b7280',
                wordBreak: 'break-word',
                overflowWrap: 'break-word',
                lineHeight: '1.4'
              }}>
                {pathText}
              </div>
            </div>
          </div>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '4px',
            fontSize: '11px',
            color: '#9ca3af',
            flexShrink: 0,
            marginLeft: '8px'
          }}>
            <ClockCircleOutlined style={{ fontSize: '11px' }} />
            <span style={{ whiteSpace: 'nowrap' }}>{timestamp}</span>
          </div>
        </div>

        {/* Divider */}
        <div style={{
          height: '1px',
          backgroundColor: '#e5e7eb'
        }}></div>

        {/* Stats Section - White background */}
        <div style={{
          backgroundColor: '#ffffff',
          padding: '24px',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          gap: '16px',
          flex: 1
        }}>
          {/* Total Size Card */}
          <div style={{
            backgroundColor: '#f6fcf8',
            borderRadius: '12px',
            padding: '16px 20px',
            textAlign: 'center',
            flex: 1,
            minWidth: '120px'
          }}>
            <div style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: '6px',
              marginBottom: '8px',
              flexWrap: 'wrap'
            }}>
              <DatabaseOutlined style={{ fontSize: '14px', color: '#22c55e' }} />
              <span style={{ fontSize: '13px', color: '#6b7280', fontWeight: '500' }}>Size</span>
            </div>
            <div style={{
              fontSize: '24px',
              fontWeight: '700',
              color: '#22c55e',
              lineHeight: '1.2',
              wordBreak: 'break-word'
            }}>
              {formatFileSize(data.localBytes || 0)}
            </div>
          </div>

          {/* Files Card */}
          <div style={{
            backgroundColor: '#f6fcf8',
            borderRadius: '12px',
            padding: '16px 20px',
            textAlign: 'center',
            flex: 1,
            minWidth: '120px'
          }}>
            <div style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: '6px',
              marginBottom: '8px',
              flexWrap: 'wrap'
            }}>
              <CheckCircleOutlined style={{ fontSize: '14px', color: '#22c55e' }} />
              <span style={{ fontSize: '13px', color: '#6b7280', fontWeight: '500' }}>Files</span>
            </div>
            <div style={{
              fontSize: '24px',
              fontWeight: '700',
              color: '#22c55e',
              lineHeight: '1.2',
              wordBreak: 'break-word'
            }}>
              {(data.localFiles || 0).toLocaleString()}
            </div>
          </div>
        </div>
      </div>
    );
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
          }
          
          .custom-table .ant-table-tbody > tr:hover > td {
            background-color: #f9fafb !important;
          }
          
          .custom-table .ant-table-tbody > tr > td {
            border-bottom: 1px solid #f3f4f6 !important;
            padding: 12px 16px !important;
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
            Folder Statistics
          </Title>
          <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280' }}>
            Monitor folder synchronization across agents
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
                  <FolderOutlined style={{ fontSize: '18px', color: '#10b981' }} />
                </div>
                <div>
                  <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                    Active Jobs
                  </Text>
                  <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                    {stats.activeJobs}
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', marginTop: '8px' }}>
                    <Text type="secondary" style={{ fontSize: '12px' }}>
                      Monitoring sync jobs
                    </Text>
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
                  <FileOutlined style={{ fontSize: '18px', color: '#10b981' }} />
                </div>
                <div>
                  <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                    Total Files
                  </Text>
                  <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                    {stats.totalFiles.toLocaleString()}
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', marginTop: '8px' }}>
                    <Text type="secondary" style={{ fontSize: '12px' }}>
                      Across all jobs
                    </Text>
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
                    Data Volume
                  </Text>
                  <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                    {formatFileSize(stats.dataVolume)}
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', marginTop: '8px' }}>
                    <Text type="secondary" style={{ fontSize: '12px' }}>
                      Total synchronized
                    </Text>
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
                  <BarChartOutlined style={{ fontSize: '18px', color: '#10b981' }} />
                </div>
                <div>
                  <Text type="secondary" style={{ fontSize: '14px', color: '#6b7280', fontWeight: 500 }}>
                    Success Rate
                  </Text>
                  <div style={{ fontSize: '32px', fontWeight: 700, color: '#1f2937', marginTop: '4px' }}>
                    {stats.successRate}%
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', marginTop: '8px' }}>
                    <Text type="secondary" style={{ fontSize: '12px' }}>
                      Overall performance
                    </Text>
                  </div>
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
          <div style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '16px',
            flexWrap: 'wrap',
            gap: '16px'
          }}>
            <Title level={4} style={{ margin: 0, color: '#1f2937', fontWeight: 600 }}>
              Job Statistics
            </Title>

            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
              <Search
                placeholder="Search jobs, agents..."
                allowClear
                value={searchText}
                onChange={(e) => setSearchText(e.target.value)}
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
                {statusOptions.map(status => (
                  <Option key={status.code} value={status.code}>
                    {status.label}
                  </Option>
                ))}
              </Select>

              <Button
                icon={<ReloadOutlined />}
                onClick={refreshAllStats}
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
          <Table
            className="custom-table"
            columns={columns}
            dataSource={jobs}
            rowKey="id"
            pagination={false}
            loading={loading}
            expandable={{
            expandedRowKeys: expandedRowKeys,
            onExpand: handleExpandRow,
            expandedRowRender: (record) => {
              const stats = folderStats[record.id];
              const isLoading = loadingJobs[record.id];
              
              // COMMENTED OUT: Auto-load disabled - user must manually click refresh
              // Auto-load statistics when expanded if not already loaded and not currently loading
              /*
              if (!stats && !isLoading) {
                setTimeout(() => {
                  refreshJobStats(record);
                }, 100);
              }
              */
              
              // Show loading state for this specific job
              if (!stats || isLoading) {
                return (
                  <div style={{ padding: '20px', textAlign: 'center' }}>
                    <Space direction="vertical">
                      <LoadingOutlined style={{ fontSize: '24px' }} />
                      <Text type="secondary">Loading statistics for {record.name}...</Text>
                    </Space>
                  </div>
                );
              }

              // Get destination count
              const destCount = stats.destinations ? stats.destinations.length : 1;

              // Layout 1: Single destination (1 dest) - Horizontal Source + Arrow + Destination
              if (destCount === 1) {
                return (
                  <div style={{
                    padding: '32px 24px',
                    backgroundColor: '#ffffff'
                  }}>
                    <div style={{
                      display: 'flex',
                      alignItems: 'stretch',
                      gap: '24px'
                    }}>
                      {/* Source Card */}
                      <div style={{ flex: 1 }}>
                        {renderStatsCard(
                          `Source: ${record.source_path}`,
                          stats.source,
                          'source'
                        )}
                      </div>

                      {/* Sync Flow Arrow */}
                      <div style={{
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        gap: '8px',
                        minWidth: '90px'
                      }}>
                        <div style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: '8px',
                          backgroundColor: '#ffffff',
                          padding: '8px 16px',
                          borderRadius: '24px',
                          border: '1px solid #e5e7eb',
                          boxShadow: '0 1px 2px 0 rgba(0, 0, 0, 0.05)'
                        }}>
                          <div style={{
                            width: '8px',
                            height: '8px',
                            borderRadius: '50%',
                            backgroundColor: '#22c55e'
                          }}></div>
                          {record.sync_mode === 'two-way' ? (
                            <>
                              <div style={{ fontSize: '14px', color: '#6b7280', fontWeight: '500' }}>←</div>
                              <div style={{ fontSize: '14px', color: '#6b7280', fontWeight: '500' }}>→</div>
                            </>
                          ) : (
                            <div style={{ fontSize: '14px', color: '#6b7280', fontWeight: '500' }}>→</div>
                          )}
                          <div style={{
                            width: '8px',
                            height: '8px',
                            borderRadius: '50%',
                            backgroundColor: '#22c55e'
                          }}></div>
                        </div>
                        <div style={{
                          fontSize: '12px',
                          color: '#9ca3af',
                          textAlign: 'center',
                          fontWeight: '400'
                        }}>
                          SYNC
                        </div>
                      </div>

                      {/* Destination Card */}
                      <div style={{ flex: 1 }}>
                        {renderStatsCard(
                          `Destination: ${stats.destinations ? stats.destinations[0].path : record.destination_path}`,
                          stats.destinations ? stats.destinations[0].stats : stats.destination,
                          'destination'
                        )}
                      </div>
                    </div>
                  </div>
                );
              }

              // Layout 2: 2 Destinations - Source on top, 2 destinations in row below
              if (destCount === 2) {
                return (
                  <div style={{
                    padding: '32px 24px',
                    backgroundColor: '#ffffff'
                  }}>
                    {/* Source Section - Full width on top */}
                    <div style={{ marginBottom: '32px' }}>
                      <div style={{
                        fontSize: '14px',
                        fontWeight: '600',
                        color: '#374151',
                        marginBottom: '12px',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px'
                      }}>
                        <div style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          backgroundColor: '#22c55e'
                        }}></div>
                        Source
                      </div>
                      {renderStatsCard(
                        `${record.source_agent_name} - ${record.source_path}`,
                        stats.source,
                        'source'
                      )}
                    </div>

                    {/* Sync Arrow */}
                    <div style={{
                      display: 'flex',
                      justifyContent: 'center',
                      marginBottom: '32px'
                    }}>
                      <div style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px',
                        backgroundColor: '#ffffff',
                        padding: '8px 16px',
                        borderRadius: '24px',
                        border: '1px solid #e5e7eb',
                        boxShadow: '0 1px 2px 0 rgba(0, 0, 0, 0.05)'
                      }}>
                        <div style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          backgroundColor: '#22c55e'
                        }}></div>
                        <div style={{ fontSize: '14px', color: '#6b7280', fontWeight: '500' }}>SYNC</div>
                        <div style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          backgroundColor: '#22c55e'
                        }}></div>
                      </div>
                    </div>

                    {/* Destinations Section - 2 columns */}
                    <div>
                      <div style={{
                        fontSize: '14px',
                        fontWeight: '600',
                        color: '#374151',
                        marginBottom: '12px',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px'
                      }}>
                        <div style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          backgroundColor: '#22c55e'
                        }}></div>
                        {destCount} Destinations
                      </div>

                      <div style={{
                        display: 'grid',
                        gridTemplateColumns: 'repeat(2, 1fr)',
                        gap: '16px'
                      }}>
                        {stats.destinations.map((dest, index) => (
                          <div key={dest.agent_id || index}>
                            {renderStatsCard(
                              `${dest.agent_name} - ${dest.path}`,
                              dest.stats,
                              'destination'
                            )}
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                );
              }

              // Layout 3: 3 Destinations - Source on top, 3 destinations in row below
              if (destCount === 3) {
                return (
                  <div style={{
                    padding: '32px 24px',
                    backgroundColor: '#ffffff'
                  }}>
                    {/* Source Section - Full width on top */}
                    <div style={{ marginBottom: '32px' }}>
                      <div style={{
                        fontSize: '14px',
                        fontWeight: '600',
                        color: '#374151',
                        marginBottom: '12px',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px'
                      }}>
                        <div style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          backgroundColor: '#22c55e'
                        }}></div>
                        Source
                      </div>
                      {renderStatsCard(
                        `${record.source_agent_name} - ${record.source_path}`,
                        stats.source,
                        'source'
                      )}
                    </div>

                    {/* Sync Arrow */}
                    <div style={{
                      display: 'flex',
                      justifyContent: 'center',
                      marginBottom: '32px'
                    }}>
                      <div style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px',
                        backgroundColor: '#ffffff',
                        padding: '8px 16px',
                        borderRadius: '24px',
                        border: '1px solid #e5e7eb',
                        boxShadow: '0 1px 2px 0 rgba(0, 0, 0, 0.05)'
                      }}>
                        <div style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          backgroundColor: '#22c55e'
                        }}></div>
                        <div style={{ fontSize: '14px', color: '#6b7280', fontWeight: '500' }}>SYNC</div>
                        <div style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          backgroundColor: '#22c55e'
                        }}></div>
                      </div>
                    </div>

                    {/* Destinations Section - 3 columns */}
                    <div>
                      <div style={{
                        fontSize: '14px',
                        fontWeight: '600',
                        color: '#374151',
                        marginBottom: '12px',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px'
                      }}>
                        <div style={{
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          backgroundColor: '#22c55e'
                        }}></div>
                        {destCount} Destinations
                      </div>

                      <div style={{
                        display: 'grid',
                        gridTemplateColumns: 'repeat(3, 1fr)',
                        gap: '16px'
                      }}>
                        {stats.destinations.map((dest, index) => (
                          <div key={dest.agent_id || index}>
                            {renderStatsCard(
                              `${dest.agent_name} - ${dest.path}`,
                              dest.stats,
                              'destination'
                            )}
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                );
              }

              // Layout 4+: 4+ Destinations - Source on top, destinations in 2x2 grid or more
              return (
                <div style={{
                  padding: '32px 24px',
                  backgroundColor: '#ffffff'
                }}>
                  {/* Source Section - Full width on top */}
                  <div style={{ marginBottom: '32px' }}>
                    <div style={{
                      fontSize: '14px',
                      fontWeight: '600',
                      color: '#374151',
                      marginBottom: '12px',
                      display: 'flex',
                      alignItems: 'center',
                      gap: '8px'
                    }}>
                      <div style={{
                        width: '8px',
                        height: '8px',
                        borderRadius: '50%',
                        backgroundColor: '#22c55e'
                      }}></div>
                      Source
                    </div>
                    {renderStatsCard(
                      `${record.source_agent_name} - ${record.source_path}`,
                      stats.source,
                      'source'
                    )}
                  </div>

                  {/* Sync Arrow */}
                  <div style={{
                    display: 'flex',
                    justifyContent: 'center',
                    marginBottom: '32px'
                  }}>
                    <div style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: '8px',
                      backgroundColor: '#ffffff',
                      padding: '8px 16px',
                      borderRadius: '24px',
                      border: '1px solid #e5e7eb',
                      boxShadow: '0 1px 2px 0 rgba(0, 0, 0, 0.05)'
                    }}>
                      <div style={{
                        width: '8px',
                        height: '8px',
                        borderRadius: '50%',
                        backgroundColor: '#22c55e'
                      }}></div>
                      <div style={{ fontSize: '14px', color: '#6b7280', fontWeight: '500' }}>SYNC</div>
                      <div style={{
                        width: '8px',
                        height: '8px',
                        borderRadius: '50%',
                        backgroundColor: '#22c55e'
                      }}></div>
                    </div>
                  </div>

                  {/* Destinations Section - 2 columns grid for 4+ destinations */}
                  <div>
                    <div style={{
                      fontSize: '14px',
                      fontWeight: '600',
                      color: '#374151',
                      marginBottom: '12px',
                      display: 'flex',
                      alignItems: 'center',
                      gap: '8px'
                    }}>
                      <div style={{
                        width: '8px',
                        height: '8px',
                        borderRadius: '50%',
                        backgroundColor: '#22c55e'
                      }}></div>
                      {destCount} Destinations
                    </div>

                    <div style={{
                      display: 'grid',
                      gridTemplateColumns: 'repeat(2, 1fr)',
                      gap: '16px'
                    }}>
                      {stats.destinations.map((dest, index) => (
                        <div key={dest.agent_id || index}>
                          {renderStatsCard(
                            `${dest.agent_name} - ${dest.path}`,
                            dest.stats,
                            'destination'
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              );
            },
            rowExpandable: () => true, // All rows are expandable
          }}
          />
          
          {/* Custom Pagination */}
          <div style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            paddingTop: '16px',
            borderTop: '1px solid #f3f4f6',
            marginTop: '16px'
          }}>
            <span style={{ color: '#6b7280', fontSize: '14px' }}>
              Showing 1 to {Math.min(10, jobs.length)} of {jobs.length} entries
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
                  defaultValue="10"
                >
                  <option value="5">5</option>
                  <option value="10">10</option>
                  <option value="25">25</option>
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
                    fontWeight: 500
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
        title={
          <Space>
            <FolderOutlined />
            Folder Statistics: {selectedJob?.name}
          </Space>
        }
        open={detailModalVisible}
        onCancel={() => {
          console.log('🚪 Modal closing - stopping all intervals');
          // Stop all intervals when modal closes
          Object.keys(refreshIntervalsRef.current).forEach(jobId => {
            const job = jobs.find(j => j.id === jobId);
            if (job) {
              stopFastRefresh(job);
            }
          });
          setDetailModalVisible(false);
        }}
        width={1000}
        footer={[
          <Button key="refresh" icon={<ReloadOutlined />} onClick={() => refreshJobStats(selectedJob)}>
            Refresh
          </Button>,
          <Button key="close" onClick={() => {
            console.log('🚪 Close button clicked - stopping all intervals');
            // Stop all intervals when modal closes
            Object.keys(refreshIntervalsRef.current).forEach(jobId => {
              const job = jobs.find(j => j.id === jobId);
              if (job) {
                stopFastRefresh(job);
              }
            });
            setDetailModalVisible(false);
          }}>
            Close
          </Button>
        ]}
      >
        {selectedJob && folderStats[selectedJob.id] && (
          <Row gutter={16}>
            <Col span={12}>
              {renderStatsCard(
                `Source: ${selectedJob.source_path}`,
                folderStats[selectedJob.id].source,
                'source'
              )}
            </Col>
            <Col span={12}>
              {renderStatsCard(
                `Destination: ${selectedJob.destination_path}`,
                folderStats[selectedJob.id].destination,
                'destination'
              )}
            </Col>
          </Row>
        )}
        </Modal>
        </div>
      </ConfigProvider>
    </>
  );
};

export default FolderStats;