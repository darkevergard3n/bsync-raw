import React, { useState, useEffect } from 'react';
import {
  Card,
  Table,
  Button,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  TimePicker,
  Space,
  Tag,
  message,
  Row,
  Col,
  Typography,
  Divider,
  Tooltip,
  ConfigProvider,
  Dropdown,
  Tree,
  Spin
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  SearchOutlined,
  ReloadOutlined,
  FilterOutlined,
  CheckOutlined,
  CloseOutlined,
  PlayCircleOutlined,
  PauseCircleOutlined,
  MoreOutlined,
  DesktopOutlined,
  SwapOutlined,
  ArrowRightOutlined,
  ExclamationCircleOutlined,
  SettingOutlined,
  DatabaseOutlined,
  FolderOutlined,
  AimOutlined,
  FileOutlined,
  ClockCircleOutlined,
  CalendarOutlined,
} from '@ant-design/icons';
import apiInstance from '../services/api';

const { Option } = Select;
const { TextArea } = Input;
const { Text } = Typography;
const { DirectoryTree } = Tree;

// API endpoints for sync jobs
const syncJobAPI = {
  list: () => apiInstance.get('/api/v1/sync-jobs'),
  create: (data) => apiInstance.post('/api/v1/sync-jobs', data),
  update: (id, data) => apiInstance.put(`/api/v1/sync-jobs/${id}`, data),
  delete: (id) => apiInstance.delete(`/api/v1/sync-jobs/${id}`),
  pause: (id) => apiInstance.post(`/api/v1/sync-jobs/${id}/pause`),
  resume: (id) => apiInstance.post(`/api/v1/sync-jobs/${id}/resume`),
};

// API endpoints for agents
const agentAPI = {
  list: () => apiInstance.get('/api/integrated-agents'),
  browse: (agentId, path = '/', depth = 2) =>
    apiInstance.get(`/api/agents/${agentId}/browse?path=${encodeURIComponent(path)}&depth=${depth}`),
  browseFolders: (id, params) => {
    console.log('[AgentAPI] Browsing folders for agent:', id, 'params:', params);
    return apiInstance.get(`/api/agents/${id}/browse`, { params });
  }
};

function Jobs() {
  const [jobs, setJobs] = useState([]);
  const [filteredJobs, setFilteredJobs] = useState([]);
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingJob, setEditingJob] = useState(null);
  const [form] = Form.useForm();
  
  // Search and Filter states
  const [searchText, setSearchText] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [modeFilter, setModeFilter] = useState('all');
  const [editingJobName, setEditingJobName] = useState(null);
  const [editingName, setEditingName] = useState('');
  
  // File browser states - following browse_folder.md documentation
  const [browserModalVisible, setBrowserModalVisible] = useState(false);
  const [browsingAgent, setBrowsingAgent] = useState(null);
  const [folderTree, setFolderTree] = useState([]);
  const [loadingTree, setLoadingTree] = useState(false);
  const [selectedPath, setSelectedPath] = useState(null);
  const [browsingFor, setBrowsingFor] = useState(''); // 'source' or 'destination'
  
  // Schedule configuration state
  const [selectedScheduleType, setSelectedScheduleType] = useState('continuous');
  
  // Time picker modal state
  const [timePickerModalVisible, setTimePickerModalVisible] = useState(false);
  const [selectedHour, setSelectedHour] = useState('12');
  const [selectedMinute, setSelectedMinute] = useState('00');
  const [selectedTime, setSelectedTime] = useState('12:00');
  
  // Agent selection state for browse buttons
  const [selectedSourceAgent, setSelectedSourceAgent] = useState(null);
  const [selectedDestinationAgent, setSelectedDestinationAgent] = useState(null);

  // Multiple destinations state
  const [destinations, setDestinations] = useState([{ id: 1, agent: null, path: '' }]);

  useEffect(() => {
    fetchJobs();
    fetchAgents();
  }, []);

  // Filter effect
  useEffect(() => {
    let filtered = jobs;

    // Search filter
    if (searchText) {
      filtered = filtered.filter(job =>
        job.name?.toLowerCase().includes(searchText.toLowerCase()) ||
        job.source_agent_name?.toLowerCase().includes(searchText.toLowerCase()) ||
        job.destination_agent_name?.toLowerCase().includes(searchText.toLowerCase())
      );
    }

    // Status filter
    if (statusFilter !== 'all') {
      filtered = filtered.filter(job => {
        if (statusFilter === 'active') return job.status === 'active' || !job.is_paused;
        if (statusFilter === 'paused') return job.status === 'paused' || job.is_paused;
        return true;
      });
    }

    // Mode filter
    if (modeFilter !== 'all') {
      filtered = filtered.filter(job => job.mode === modeFilter);
    }

    setFilteredJobs(filtered);
  }, [jobs, searchText, statusFilter, modeFilter]);

  const fetchJobs = async () => {
    setLoading(true);
    try {
      const response = await syncJobAPI.list();
      const jobsData = response.data.sync_jobs || response.data.data || response.data || [];
      const jobsArray = Array.isArray(jobsData) ? jobsData : [];

      // Jobs from API already contain source_agent_name and destination_agent_name
      // Just map sync_type to mode for consistency
      const enrichedJobs = jobsArray.map(job => ({
        ...job,
        mode: job.sync_mode || (job.sync_type === 'sendreceive' ? 'two-way' : 'one-way'),
        status: job.is_paused ? 'paused' : 'active'
      }));

      setJobs(enrichedJobs);
      setFilteredJobs(enrichedJobs);
    } catch (error) {
      console.error('Failed to fetch sync jobs:', error);
      message.error('Failed to fetch sync jobs');
      setJobs([]);
      setFilteredJobs([]);
    } finally {
      setLoading(false);
    }
  };

  const fetchAgents = async () => {
    try {
      const response = await agentAPI.list();
      const agentData = response.data.data || response.data || [];
      const availableAgents = agentData.filter(
        agent => agent.approval_status === 'approved' && agent.status === 'online'
      );
      setAgents(availableAgents);
    } catch (error) {
      console.error('Failed to fetch agents:', error);
      message.error('Failed to fetch agents');
    }
  };

  const handleCreateJob = () => {
    setEditingJob(null);
    form.resetFields();
    // Reset agent selection state
    setSelectedSourceAgent(null);
    setSelectedDestinationAgent(null);
    // Reset destinations to single entry
    setDestinations([{ id: 1, agent: null, path: '' }]);
    // Don't set sourcePath and destinationPath to allow browse folder selection to work
    form.setFieldsValue({
      name: '',
      mode: 'one-way',
      schedule: 'continuous',
      sourceAgent: null,
      ignore_patterns: '',
      rescan_interval: 3600
    });
    setModalVisible(true);
  };

  const handleEditJob = (job) => {
    setEditingJob(job);
    form.setFieldsValue({
      name: job.name,
      mode: job.mode || 'two-way',
      schedule: job.schedule || 'continuous',
      sourceAgent: job.source_agent_id,
      destinationAgent: job.destination_agent_id,
      sourcePath: job.source_path,
      destinationPath: job.destination_path,
      ignore_patterns: job.ignore_patterns?.join('\n') || '',
      rescan_interval: job.rescan_interval || 3600
    });
    setModalVisible(true);
  };

  const handleResumeJob = async (jobId) => {
    try {
      await syncJobAPI.resume(jobId);
      message.success('Sync job resumed');
      fetchJobs();
    } catch (error) {
      message.error('Failed to resume sync job');
    }
  };

  const handlePauseJob = async (jobId) => {
    try {
      await syncJobAPI.pause(jobId);
      message.success('Sync job paused');
      fetchJobs();
    } catch (error) {
      message.error('Failed to pause sync job');
    }
  };

  const handleDeleteJob = async (jobId) => {
    Modal.confirm({
      title: 'Delete Sync Job',
      content: 'Are you sure you want to delete this sync job?',
      icon: <ExclamationCircleOutlined />,
      onOk: async () => {
        try {
          await syncJobAPI.delete(jobId);
          message.success('Sync job deleted successfully');
          fetchJobs();
        } catch (error) {
          message.error('Failed to delete sync job');
        }
      },
    });
  };

  const handleJobNameEdit = (record) => {
    setEditingJobName(record.id);
    setEditingName(record.name);
  };

  const handleJobNameSave = async (jobId) => {
    try {
      if (!editingName.trim()) {
        message.error('Job name cannot be empty');
        return;
      }
      
      const currentJob = jobs.find(j => j.id === jobId);
      if (!currentJob) {
        message.error('Job not found');
        return;
      }

      const updateData = {
        name: editingName.trim(),
        sync_mode: currentJob.mode,
        source_agent_id: currentJob.source_agent_id,
        destination_agent_id: currentJob.destination_agent_id,
        source_path: currentJob.source_path,
        destination_path: currentJob.destination_path,
        schedule: currentJob.schedule || 'continuous'
      };

      await syncJobAPI.update(jobId, updateData);
      message.success('Job name updated successfully');
      
      await fetchJobs();
      setEditingJobName(null);
      setEditingName('');
    } catch (error) {
      console.error('Failed to update job name:', error);
      message.error('Failed to update job name');
    }
  };

  const handleJobNameCancel = () => {
    setEditingJobName(null);
    setEditingName('');
  };

  const handleSubmit = async (values) => {
    try {
      // Format schedule based on type
      let formattedSchedule = values.schedule || 'continuous';
      if (values.schedule === 'daily') {
        // Format as daily_HHMM (e.g., daily_0900 for 09:00)
        const hour = selectedHour.padStart(2, '0');
        const minute = selectedMinute.padStart(2, '0');
        formattedSchedule = `daily_${hour}${minute}`;
      }

      // Map sync_mode to sync_type
      const syncType = values.mode === 'one-way' ? 'sendonly' : 'sendreceive';

      // Build destinations array from form values
      const destinationsArray = destinations.map(dest => {
        const agentId = values[`destinationAgent_${dest.id}`];
        const path = values[`destinationPath_${dest.id}`];

        if (!agentId || !path) {
          throw new Error(`Please complete all destination fields`);
        }

        return {
          agent_id: agentId,
          path: path
        };
      });

      const data = {
        name: values.name,
        source_agent_id: values.sourceAgent,
        source_path: values.sourcePath,
        destinations: destinationsArray,
        sync_type: syncType,
        schedule_type: formattedSchedule,
        rescan_interval: values.rescan_interval || 3600,
        ignore_patterns: values.ignore_patterns
          ? values.ignore_patterns.split('\n').filter(p => p.trim())
          : []
      };

      if (editingJob) {
        await syncJobAPI.update(editingJob.id, data);
        message.success('Sync job updated successfully');
      } else {
        await syncJobAPI.create(data);
        message.success('Sync job created successfully');
      }

      await fetchJobs();
      setModalVisible(false);
      setEditingJob(null);
      form.resetFields();
      // Reset destinations to single entry
      setDestinations([{ id: 1, agent: null, path: '' }]);
    } catch (error) {
      console.error('Error saving job:', error);
      message.error('Failed to save job: ' + (error.response?.data?.error || error.message));
    }
  };

  // Agent selection handlers
  const handleSourceAgentChange = (agentId) => {
    setSelectedSourceAgent(agentId);
    form.setFieldsValue({ sourceAgent: agentId });
  };

  const handleDestinationAgentChange = (agentId) => {
    setSelectedDestinationAgent(agentId);
    form.setFieldsValue({ destinationAgent: agentId });
  };

  // Browse functionality - following browse_folder.md documentation
  const handleBrowseFolders = async (type, destinationId = null) => {
    const formValues = form.getFieldsValue();
    let agentId;

    if (type === 'source') {
      agentId = formValues.sourceAgent;
    } else if (type === 'destination' && destinationId) {
      agentId = formValues[`destinationAgent_${destinationId}`];
    } else {
      agentId = formValues.destinationAgent;
    }

    if (!agentId) {
      message.warning('Please select an agent first');
      return;
    }

    const agent = agents.find(a => a.agent_id === agentId);
    if (!agent) {
      message.error('Agent not found');
      return;
    }

    setBrowsingAgent(agent);
    setBrowserModalVisible(true);
    setLoadingTree(true);
    setSelectedPath(null);
    setBrowsingFor(destinationId ? `destination_${destinationId}` : type);

    try {
      const response = await agentAPI.browseFolders(agent.agent_id, {
        path: '/',
        depth: 2,
      });
      setFolderTree(convertToTreeData(response.data));
    } catch (error) {
      message.error('Failed to browse folders');
      console.error('Failed to browse folders:', error);
    } finally {
      setLoadingTree(false);
    }
  };

  // Convert API response to Tree component format
  const convertToTreeData = (folderInfo) => {
    const convert = (info) => ({
      title: info.name,
      key: info.path,
      icon: info.is_directory ? <FolderOutlined /> : <FileOutlined />,
      children: info.children?.map(convert),
      isLeaf: !info.is_directory,
      selectable: info.is_directory,
    });

    return [convert(folderInfo)];
  };

  // Handle tree node expansion (lazy loading)
  const onLoadData = async (treeNode) => {
    try {
      const response = await agentAPI.browseFolders(browsingAgent.agent_id, {
        path: treeNode.key,
        depth: 2,
      });

      const newChildren = response.data.children?.map(child => convertToTreeData(child)[0]) || [];
      
      setFolderTree(prevTree => updateTreeData(prevTree, treeNode.key, newChildren));
    } catch (error) {
      message.error('Failed to load folder contents');
    }
  };

  // Update tree data with new children
  const updateTreeData = (list, key, children) => {
    return list.map(node => {
      if (node.key === key) {
        return { ...node, children };
      }
      if (node.children) {
        return { ...node, children: updateTreeData(node.children, key, children) };
      }
      return node;
    });
  };

  // Handle folder selection in tree
  const handleSelectFolder = (selectedKeys) => {
    if (selectedKeys.length > 0) {
      setSelectedPath(selectedKeys[0]);
    }
  };

  // Confirm path selection and update form
  const handleConfirmSelection = () => {
    if (selectedPath) {
      console.log('[Path Selection] Selected path:', selectedPath);
      console.log('[Path Selection] Browsing for:', browsingFor);

      if (browsingFor === 'source') {
        console.log('[Path Selection] Setting sourcePath to:', selectedPath);
        form.setFieldValue('sourcePath', selectedPath);
        message.success('Source folder path selected');
      } else if (browsingFor.startsWith('destination_')) {
        // Extract destination ID from browsingFor (e.g., "destination_1" -> "1")
        const destId = browsingFor.split('_')[1];
        const fieldName = `destinationPath_${destId}`;
        console.log('[Path Selection] Setting', fieldName, 'to:', selectedPath);
        form.setFieldValue(fieldName, selectedPath);
        message.success('Destination folder path selected');
      } else if (browsingFor === 'destination') {
        console.log('[Path Selection] Setting destinationPath to:', selectedPath);
        form.setFieldValue('destinationPath', selectedPath);
        message.success('Destination folder path selected');
      }

      // Debug: Check form values after setting
      console.log('[Path Selection] Form values after setting:', form.getFieldsValue());

      setBrowserModalVisible(false);
      setSelectedPath(null);
      setBrowsingFor('');
    }
  };

  // Time picker functionality
  const handleTimePickerClick = () => {
    setTimePickerModalVisible(true);
  };

  const handleQuickTimeSelect = (time) => {
    const [hour, minute] = time.split(':');
    setSelectedHour(hour);
    setSelectedMinute(minute);
    setSelectedTime(time);
  };

  const handleApplyTime = () => {
    const time = `${selectedHour}:${selectedMinute}`;
    setSelectedTime(time);
    form.setFieldsValue({ run_time: time });
    setTimePickerModalVisible(false);
  };

  const getActionsMenuItems = (record) => {
    return [
      {
        key: 'start',
        icon: <PlayCircleOutlined />,
        label: 'Start Job',
        disabled: !record.is_paused,
        onClick: () => handleResumeJob(record.id)
      },
      {
        key: 'pause',
        icon: <PauseCircleOutlined />,
        label: 'Pause Job',
        disabled: record.is_paused,
        onClick: () => handlePauseJob(record.id)
      },
      {
        type: 'divider'
      },
      {
        key: 'delete',
        icon: <DeleteOutlined />,
        label: 'Delete Job',
        danger: true,
        onClick: () => handleDeleteJob(record.id)
      }
    ];
  };

  const columns = [
    {
      title: 'Job Name',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      ellipsis: true,
      render: (text, record) => {
        const isEditing = editingJobName === record.id;
        
        if (isEditing) {
          return (
            <Space.Compact>
              <Input
                value={editingName}
                onChange={(e) => setEditingName(e.target.value)}
                onPressEnter={() => handleJobNameSave(record.id)}
                size="small"
                style={{ width: 120 }}
                autoFocus
              />
              <Button
                type="primary"
                size="small"
                icon={<CheckOutlined />}
                onClick={() => handleJobNameSave(record.id)}
              />
              <Button
                size="small"
                icon={<CloseOutlined />}
                onClick={handleJobNameCancel}
              />
            </Space.Compact>
          );
        }
        
        return (
          <Space>
            <Text strong title={text}>{text}</Text>
            <Button
              type="text"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleJobNameEdit(record)}
              style={{ opacity: 0.5 }}
            />
          </Space>
        );
      },
    },
    {
      title: 'Source Server',
      key: 'source',
      width: 180,
      ellipsis: true,
      render: (_, record) => (
        <div 
          title={record.source_path || 'N/A'}
          style={{ 
            cursor: 'default',
            color: '#6b7280',
            fontSize: '14px',
            display: 'flex',
            alignItems: 'center',
            gap: '6px'
          }}
        >
          <DesktopOutlined style={{ fontSize: '14px' }} />
          <span>{record.source_agent_name || 'N/A'}</span>
        </div>
      ),
    },
    {
      title: 'Destination Server',
      key: 'destination',
      width: 180,
      ellipsis: true,
      render: (_, record) => {
        // Check if multi-destination
        if (record.is_multi_destination && record.destinations && record.destinations.length > 0) {
          const firstDest = record.destinations[0];
          const remainingCount = record.destinations.length - 1;

          // If only 1 destination, show hostname only (like source server)
          if (record.destinations.length === 1) {
            return (
              <div
                title={firstDest.path || 'N/A'}
                style={{
                  cursor: 'default',
                  color: '#6b7280',
                  fontSize: '14px',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '6px'
                }}
              >
                <DesktopOutlined style={{ fontSize: '14px' }} />
                <span>{firstDest.agent_name || firstDest.destination_agent_id || 'N/A'}</span>
              </div>
            );
          }

          // If multiple destinations, show first + count tag
          return (
            <div style={{
              cursor: 'default',
              color: '#6b7280',
              fontSize: '14px',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <DesktopOutlined style={{ fontSize: '14px' }} />
                <span>{firstDest.agent_name || firstDest.destination_agent_id || 'N/A'}</span>
              </div>
              <Tag
                style={{
                  marginTop: '4px',
                  backgroundColor: '#f3f4f6',
                  color: '#374151',
                  border: 'none',
                  fontSize: '12px',
                  padding: '2px 8px'
                }}
              >
                +{remainingCount} more
              </Tag>
            </div>
          );
        }

        // Single destination (legacy)
        return (
          <div
            title={record.destination_path || 'N/A'}
            style={{
              cursor: 'default',
              color: '#6b7280',
              fontSize: '14px',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <DesktopOutlined style={{ fontSize: '14px' }} />
            <span>{record.destination_agent_name || 'N/A'}</span>
          </div>
        );
      },
    },
    {
      title: 'Mode',
      key: 'mode',
      width: 100,
      render: (_, record) => {
        const mode = record.mode;
        const isOneWay = mode === 'one-way';
        
        return (
          <Tag 
            color={isOneWay ? "default" : "green"}
            style={{
              backgroundColor: isOneWay ? '#f3f4f6' : '#dcfce7',
              color: isOneWay ? '#374151' : '#166534',
              border: 'none'
            }}
          >
            {isOneWay ? 'One-way' : 'Two-way'}
          </Tag>
        );
      },
    },
    {
      title: 'Schedule',
      dataIndex: 'schedule',
      key: 'schedule',
      width: 100,
      render: (schedule) => {
        let displayText = schedule;
        
        if (!schedule || schedule === 'continuous') {
          displayText = '24/7';
        } else if (schedule.startsWith('daily_')) {
          const time = schedule.substring(6);
          displayText = time;
        }
        
        return (
          <span style={{
            color: '#6b7280',
            fontSize: '14px'
          }}>
            {displayText}
          </span>
        );
      },
    },
    {
      title: 'Status',
      key: 'status',
      width: 100,
      render: (_, record) => {
        const isActive = record.status === 'active';
        return (
          <Tag 
            color={isActive ? "green" : "orange"}
            style={{
              backgroundColor: isActive ? '#dcfce7' : '#fed7aa',
              color: isActive ? '#166534' : '#9a3412',
              border: 'none'
            }}
          >
            {isActive ? 'Active' : 'Pending'}
          </Tag>
        );
      },
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 120,
      render: (_, record) => (
        <Space size="small">
          <Button
            type="link"
            icon={<EditOutlined />}
            size="small"
            onClick={() => handleEditJob(record)}
            style={{ color: '#1890ff' }}
          />
          <Dropdown menu={{ items: getActionsMenuItems(record) }} trigger={['click']} placement="bottomRight">
            <Button
              type="link"
              icon={<MoreOutlined />}
              size="small"
              style={{ color: '#666' }}
            />
          </Dropdown>
        </Space>
      ),
    },
  ];

  return (
    <>
      <style>
        {`
          .custom-search .ant-input:hover,
          .custom-search .ant-input:focus {
            border-color: #10b981 !important;
            box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2) !important;
          }
          
          .custom-select .ant-select-selector:hover,
          .custom-select .ant-select-focused .ant-select-selector {
            border-color: #10b981 !important;
            box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2) !important;
          }
          
          .custom-button:hover {
            border-color: #10b981 !important;
            color: inherit !important;
          }
          
          .custom-button:focus {
            border-color: #10b981 !important;
            color: inherit !important;
          }
          
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
          
          .modal-primary-button {
            background-color: #00be62 !important;
            border-color: #00be62 !important;
          }
          
          .modal-primary-button:hover,
          .modal-primary-button:focus {
            background-color: #00a855 !important;
            border-color: #00a855 !important;
          }

          /* Custom Tree Selection Colors - Only target the selected item with blue background */
          .ant-tree .ant-tree-node-content-wrapper.ant-tree-node-selected,
          .ant-tree .ant-tree-node-content-wrapper.ant-tree-node-selected.ant-tree-node-content-wrapper-close,
          .ant-tree .ant-tree-node-content-wrapper.ant-tree-node-selected.ant-tree-node-content-wrapper-open {
            background: #00be62 !important;
            background-color: #00be62 !important;
            color: white !important;
          }
          
          /* Target the specific selected node content wrapper */
          .ant-tree .ant-tree-node-content-wrapper-close.ant-tree-node-selected {
            background: #00be62 !important;
            background-color: #00be62 !important;
            color: white !important;
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
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '16px' }}>
          <div>
            <h1 style={{ margin: 0, fontSize: '28px', fontWeight: '600', color: '#1f2937' }}>
              Sync Jobs Management
            </h1>
            <p style={{ margin: '8px 0 0 0', fontSize: '16px', color: '#6b7280' }}>
              Manage and monitor data synchronization jobs
            </p>
          </div>
          <Button
            icon={<PlusOutlined />}
            style={{ 
              backgroundColor: '#00be62', 
              borderColor: '#00be62',
              color: 'white',
              fontWeight: '500',
              border: '1px solid #00be62'
            }}
            onMouseEnter={(e) => {
              e.target.style.backgroundColor = '#00a855';
              e.target.style.borderColor = '#00a855';
            }}
            onMouseLeave={(e) => {
              e.target.style.backgroundColor = '#00be62';
              e.target.style.borderColor = '#00be62';
            }}
            onClick={handleCreateJob}
          >
            New Job
          </Button>
        </div>
      </div>
        
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
            <h2 style={{ margin: 0, fontSize: '18px', fontWeight: '600', color: '#1f2937' }}>
              Sync Jobs
            </h2>
          </div>
          
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
            <Input
              placeholder="Search jobs, agents, or paths..."
              prefix={<SearchOutlined />}
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{ width: 280 }}
              allowClear
              size="middle"
              className="custom-search"
            />
            <Select
              value={statusFilter}
              onChange={setStatusFilter}
              style={{ width: 140 }}
              size="middle"
              placeholder="All Status"
              className="custom-select"
            >
              <Option value="all">All Status</Option>
              <Option value="active">Active</Option>
              <Option value="paused">Pending</Option>
            </Select>
            <Select
              value={modeFilter}
              onChange={setModeFilter}
              style={{ width: 140 }}
              size="middle"
              placeholder="All Modes"
              className="custom-select"
            >
              <Option value="all">All Modes</Option>
              <Option value="one-way">One-way</Option>
              <Option value="two-way">Two-way</Option>
            </Select>
            <Button
              icon={<ReloadOutlined />}
              onClick={fetchJobs}
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
          dataSource={filteredJobs}
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
            Showing 1 to {Math.min(5, filteredJobs.length)} of {filteredJobs.length} entries
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
      </ConfigProvider>

      {/* Modal outside ConfigProvider to use default Ant Design styling */}
      <Modal
        title={
          <div style={{ 
            display: 'flex',
            alignItems: 'center',
            paddingBottom: '16px',
            borderBottom: '1px solid #f0f0f0',
            marginBottom: '24px',
            marginLeft: '-24px',
            marginRight: '-24px',
            paddingLeft: '24px',
            paddingRight: '24px'
          }}>
            <div style={{
              width: '40px',
              height: '40px',
              backgroundColor: '#ffffff',
              borderRadius: '12px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              marginRight: '12px',
              border: '1px solid #e5e7eb'
            }}>
              <SettingOutlined style={{ fontSize: '18px', color: '#00be62' }} />
            </div>
            <div>
              <h3 style={{ 
                margin: 0, 
                fontSize: '18px', 
                fontWeight: '600',
                color: '#1f2937'
              }}>
                {editingJob ? 'Edit Sync Job' : 'Create New Sync Job'}
              </h3>
              <p style={{ 
                margin: '4px 0 0 0', 
                fontSize: '14px', 
                color: '#6b7280' 
              }}>
                Configure synchronization between agents with custom settings
              </p>
            </div>
          </div>
        }
        open={modalVisible}
        onCancel={() => {
          setModalVisible(false);
          setEditingJob(null);
          form.resetFields();
          setDestinations([{ id: 1, agent: null, path: '' }]);
        }}
        centered
        footer={
          <div style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            paddingTop: '16px',
            borderTop: '1px solid #f0f0f0',
            marginLeft: '-24px',
            marginRight: '-24px',
            paddingLeft: '24px',
            paddingRight: '24px'
          }}>
            <span style={{ 
              color: '#6b7280', 
              fontSize: '14px',
              fontStyle: 'italic'
            }}>
              All required fields (*) must be filled
            </span>
            <div style={{ display: 'flex', gap: '12px' }}>
              <Button
                onClick={() => {
                  setModalVisible(false);
                  setEditingJob(null);
                  form.resetFields();
                  setDestinations([{ id: 1, agent: null, path: '' }]);
                }}
              >
                Cancel
              </Button>
              <Button 
                onClick={() => form.submit()}
                style={{ 
                  backgroundColor: '#00be62', 
                  borderColor: '#00be62',
                  color: 'white',
                  fontWeight: '500',
                  border: '1px solid #00be62'
                }}
                onMouseEnter={(e) => {
                  e.target.style.backgroundColor = '#00a855';
                  e.target.style.borderColor = '#00a855';
                }}
                onMouseLeave={(e) => {
                  e.target.style.backgroundColor = '#00be62';
                  e.target.style.borderColor = '#00be62';
                }}
              >
                {editingJob ? 'Update Job' : 'Create Sync Job'}
              </Button>
            </div>
          </div>
        }
        width={800}
        styles={{
          body: {
            maxHeight: '60vh',
            overflowY: 'auto',
            padding: '0 24px 0 50px',
            overscrollBehavior: 'contain',
            WebkitOverflowScrolling: 'touch',
            backgroundColor: '#f8fafc'
          },
          content: {
            overscrollBehavior: 'contain',
            backgroundColor: '#f8fafc'
          },
          header: {
            backgroundColor: '#f8fafc'
          },
          wrapper: {
            overscrollBehavior: 'none'
          }
        }}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={{
            mode: 'one-way',
            schedule: 'continuous',
            ignore_patterns: '',
            rescan_interval: 3600
          }}
        >
          {/* Basic Configuration Section */}
          <div style={{ marginBottom: '32px' }}>
            <div style={{ 
              display: 'flex', 
              alignItems: 'center', 
              marginBottom: '16px',
              marginLeft: '-44px'
            }}>
              <div style={{
                width: '32px',
                height: '32px',
                backgroundColor: '#00be62',
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                marginRight: '12px',
                color: 'white',
                fontWeight: '600',
                fontSize: '16px'
              }}>
                1
              </div>
              <div>
                <h3 style={{ 
                  margin: 0, 
                  fontSize: '18px', 
                  fontWeight: '600',
                  color: '#1f2937'
                }}>
                  Basic Configuration
                </h3>
                <p style={{ 
                  margin: '2px 0 0 0', 
                  fontSize: '14px', 
                  color: '#6b7280' 
                }}>
                  Set job name and sync mode
                </p>
              </div>
            </div>
            
            <div style={{
              backgroundColor: '#ffffff',
              border: '1px solid #e5e7eb',
              borderRadius: '12px',
              padding: '24px',
              boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'
            }}>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    name="name"
                    label={<span style={{ color: '#374151', fontWeight: '500' }}>Job Name </span>}
                    rules={[{ required: true, message: 'Please enter job name' }]}
                  >
                    <Input 
                      placeholder="e.g., Project Files Sync"
                      style={{
                        borderRadius: '8px',
                        border: '1px solid #d1d5db',
                        fontSize: '14px'
                      }}
                    />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item
                    name="mode"
                    label={<span style={{ color: '#374151', fontWeight: '500' }}>Sync Mode </span>}
                    rules={[{ required: true, message: 'Please select sync mode' }]}
                  >
                    <Select 
                      placeholder="Choose sync direction"
                      style={{
                        borderRadius: '8px',
                        fontSize: '14px'
                      }}
                    >
                      <Option value="one-way">
                        <Space>
                          <ArrowRightOutlined />
                          One-way (Source → Destination)
                        </Space>
                      </Option>
                      <Option value="two-way">
                        <Space>
                          <SwapOutlined />
                          Two-way (Bidirectional)
                        </Space>
                      </Option>
                    </Select>
                  </Form.Item>
                </Col>
              </Row>
            </div>
          </div>

          {/* Step 2: Source & Destination Section */}
          <div style={{ marginBottom: '32px' }}>
            <div style={{ 
              display: 'flex', 
              alignItems: 'center', 
              marginBottom: '16px',
              marginLeft: '-44px'
            }}>
              <div style={{
                width: '32px',
                height: '32px',
                backgroundColor: '#00be62',
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                marginRight: '12px',
                color: 'white',
                fontWeight: '600',
                fontSize: '16px'
              }}>
                2
              </div>
              <div>
                <h3 style={{ 
                  margin: 0, 
                  fontSize: '18px', 
                  fontWeight: '600',
                  color: '#1f2937'
                }}>
                  Source & Destination
                </h3>
                <p style={{ 
                  margin: '2px 0 0 0', 
                  fontSize: '14px', 
                  color: '#6b7280' 
                }}>
                  Configure sync endpoints
                </p>
              </div>
            </div>
            
            <div style={{
              backgroundColor: '#ffffff',
              border: '1px solid #e5e7eb',
              borderRadius: '12px',
              padding: '24px',
              boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'
            }}>
              {/* Source Configuration Header */}
              <div style={{ 
                display: 'flex', 
                alignItems: 'center', 
                marginBottom: '20px'
              }}>
                <DatabaseOutlined style={{ 
                  fontSize: '16px', 
                  color: '#00be62',
                  marginRight: '8px' 
                }} />
                <span style={{
                  fontSize: '16px',
                  fontWeight: '600',
                  color: '#374151'
                }}>
                  Source Configuration
                </span>
              </div>
              
              {/* Source Fields */}
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    name="sourceAgent"
                    label={
                      <span style={{ 
                        color: '#374151', 
                        fontWeight: '500',
                        fontSize: '14px'
                      }}>
                        Source Agent 
                        {/* <span style={{ color: '#ef4444' }}>*</span> */}
                      </span>
                    }
                    rules={[{ required: true, message: 'Please select source agent' }]}
                    style={{ marginBottom: '20px' }}
                  >
                    <Select 
                      placeholder="Select agent"
                      value={selectedSourceAgent}
                      onChange={handleSourceAgentChange}
                      style={{
                        borderRadius: '8px',
                        fontSize: '14px'
                      }}
                      suffixIcon={<div style={{
                        transform: 'rotate(90deg)',
                        fontSize: '12px',
                        color: '#9ca3af'
                      }}>❯</div>}
                    >
                      {agents.map(agent => (
                        <Option key={agent.agent_id} value={agent.agent_id}>
                          <Space>
                            <DesktopOutlined />
                            {agent.hostname} ({agent.ip_address})
                          </Space>
                        </Option>
                      ))}
                    </Select>
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item
                    name="sourcePath"
                    label={
                      <span style={{ 
                        color: '#374151', 
                        fontWeight: '500',
                        fontSize: '14px'
                      }}>
                        Source Path 
                        {/* <span style={{ color: '#ef4444' }}>*</span> */}
                      </span>
                    }
                    rules={[{ required: true, message: 'Please enter source path' }]}
                    style={{ marginBottom: '20px', position: 'relative' }}
                  >
                    <Input 
                      placeholder="/var/data/projectA"
                      style={{
                        borderRadius: '8px',
                        border: '1px solid #d1d5db',
                        fontSize: '14px',
                        paddingRight: '10px'
                      }}
                      suffix={
                        <Form.Item noStyle shouldUpdate={(prevValues, currentValues) => prevValues.sourceAgent !== currentValues.sourceAgent}>
                          {({ getFieldValue }) => (
                            <Button
                              type="text"
                              icon={<FolderOutlined />}
                              onClick={() => handleBrowseFolders('source')}
                              disabled={!getFieldValue('sourceAgent')}
                              style={{
                                border: 'none',
                                background: 'transparent',
                                color: getFieldValue('sourceAgent') ? '#9ca3af' : '#d1d5db',
                                padding: '2px',
                                margin: '0',
                                height: 'auto',
                                minWidth: 'auto',
                                cursor: getFieldValue('sourceAgent') ? 'pointer' : 'not-allowed'
                              }}
                            />
                          )}
                        </Form.Item>
                      }
                    />
                  </Form.Item>
                </Col>
              </Row>
            </div>
          </div>

          {/* Destination Configuration Section - Multiple Destinations */}
          <div style={{ marginBottom: '32px' }}>
            <div style={{
              backgroundColor: '#ffffff',
              border: '1px solid #e5e7eb',
              borderRadius: '12px',
              padding: '24px',
              boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'
            }}>
              {/* Destination Configuration Header with Add Button */}
              <div style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                marginBottom: '20px'
              }}>
                <div style={{ display: 'flex', alignItems: 'center' }}>
                  <AimOutlined style={{
                    fontSize: '16px',
                    color: '#00be62',
                    marginRight: '8px'
                  }} />
                  <span style={{
                    fontSize: '16px',
                    fontWeight: '600',
                    color: '#374151'
                  }}>
                    Destination Configuration
                  </span>
                </div>

                <Button
                  type="default"
                  icon={<PlusOutlined />}
                  onClick={() => {
                    if (destinations.length < 5) {
                      setDestinations([...destinations, {
                        id: destinations.length + 1,
                        agent: null,
                        path: ''
                      }]);
                    }
                  }}
                  disabled={destinations.length >= 5}
                  style={{
                    borderRadius: '8px',
                    border: '1px solid #e5e7eb',
                    color: '#374151',
                    fontWeight: 500,
                    fontSize: '14px',
                    height: '36px',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '6px'
                  }}
                >
                  Add Destination
                </Button>
              </div>

              {/* Multiple Destination Fields */}
              {destinations.map((dest, index) => (
                <div key={dest.id} style={{
                  marginBottom: index < destinations.length - 1 ? '24px' : '0',
                  paddingBottom: index < destinations.length - 1 ? '24px' : '0',
                  borderBottom: index < destinations.length - 1 ? '1px solid #e5e7eb' : 'none'
                }}>
                  {/* Destination Header with Remove Button */}
                  <div style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    marginBottom: '16px'
                  }}>
                    <span style={{
                      fontSize: '14px',
                      fontWeight: '600',
                      color: '#6b7280'
                    }}>
                      Destination {index + 1}
                    </span>
                    {destinations.length > 1 && (
                      <Button
                        type="text"
                        danger
                        icon={<CloseOutlined />}
                        onClick={() => {
                          setDestinations(destinations.filter(d => d.id !== dest.id));
                        }}
                        style={{
                          fontSize: '18px',
                          width: '24px',
                          height: '24px',
                          padding: 0,
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center'
                        }}
                      />
                    )}
                  </div>

                  <Row gutter={16}>
                    <Col span={12}>
                      <Form.Item
                        name={`destinationAgent_${dest.id}`}
                        label={
                          <span style={{
                            color: '#374151',
                            fontWeight: '500',
                            fontSize: '14px'
                          }}>
                            Agent <span style={{ color: '#ef4444' }}>*</span>
                          </span>
                        }
                        rules={[{ required: true, message: 'Please select agent' }]}
                        style={{ marginBottom: '0' }}
                      >
                        <Select
                          placeholder="Select agent"
                          style={{
                            borderRadius: '8px',
                            fontSize: '14px'
                          }}
                          suffixIcon={<div style={{
                            transform: 'rotate(90deg)',
                            fontSize: '12px',
                            color: '#9ca3af'
                          }}>❯</div>}
                        >
                          {agents.map(agent => (
                            <Option key={agent.agent_id} value={agent.agent_id}>
                              <Space>
                                <DesktopOutlined />
                                {agent.hostname} ({agent.ip_address})
                              </Space>
                            </Option>
                          ))}
                        </Select>
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        name={`destinationPath_${dest.id}`}
                        label={
                          <span style={{
                            color: '#374151',
                            fontWeight: '500',
                            fontSize: '14px'
                          }}>
                            Path <span style={{ color: '#ef4444' }}>*</span>
                          </span>
                        }
                        rules={[{ required: true, message: 'Please enter path' }]}
                        style={{ marginBottom: '0' }}
                      >
                        <Input
                          placeholder="/mnt/backup/projectA"
                          style={{
                            borderRadius: '8px',
                            border: '1px solid #d1d5db',
                            fontSize: '14px',
                            paddingRight: '10px'
                          }}
                          suffix={
                            <Form.Item noStyle shouldUpdate>
                              {({ getFieldValue }) => (
                                <Button
                                  type="text"
                                  icon={<FolderOutlined />}
                                  onClick={() => handleBrowseFolders('destination', dest.id)}
                                  disabled={!getFieldValue(`destinationAgent_${dest.id}`)}
                                  style={{
                                    border: 'none',
                                    background: 'transparent',
                                    color: getFieldValue(`destinationAgent_${dest.id}`) ? '#9ca3af' : '#d1d5db',
                                    padding: '2px',
                                    margin: '0',
                                    height: 'auto',
                                    minWidth: 'auto',
                                    cursor: getFieldValue(`destinationAgent_${dest.id}`) ? 'pointer' : 'not-allowed'
                                  }}
                                />
                              )}
                            </Form.Item>
                          }
                        />
                      </Form.Item>
                    </Col>
                  </Row>
                </div>
              ))}
            </div>
          </div>

          {/* Step 3: Schedule & Settings Section */}
          <div style={{ marginBottom: '32px' }}>
            <div style={{ 
              display: 'flex', 
              alignItems: 'center', 
              marginBottom: '16px',
              marginLeft: '-44px'
            }}>
              <div style={{
                width: '32px',
                height: '32px',
                backgroundColor: '#00be62',
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                marginRight: '12px',
                color: 'white',
                fontWeight: '600',
                fontSize: '16px'
              }}>
                3
              </div>
              <div>
                <h3 style={{ 
                  margin: 0, 
                  fontSize: '18px', 
                  fontWeight: '600',
                  color: '#1f2937'
                }}>
                  Schedule & Settings
                </h3>
                <p style={{ 
                  margin: '2px 0 0 0', 
                  fontSize: '14px', 
                  color: '#6b7280' 
                }}>
                  Configure sync frequency and advanced options
                </p>
              </div>
            </div>
            
            <div style={{
              backgroundColor: '#ffffff',
              border: '1px solid #e5e7eb',
              borderRadius: '12px',
              padding: '24px',
              boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'
            }}>
              {/* Schedule Configuration Header */}
              <div style={{ 
                display: 'flex', 
                alignItems: 'center', 
                marginBottom: '20px'
              }}>
                <ClockCircleOutlined style={{ 
                  fontSize: '16px', 
                  color: '#00be62',
                  marginRight: '8px' 
                }} />
                <span style={{
                  fontSize: '16px',
                  fontWeight: '600',
                  color: '#374151'
                }}>
                  Schedule Configuration
                </span>
              </div>
              
              {/* Schedule Type Field */}
              <Form.Item
                name="schedule"
                label={
                  <span style={{ 
                    color: '#374151', 
                    fontWeight: '500',
                    fontSize: '14px'
                  }}>
                    Schedule Type <span style={{ color: '#ef4444' }}>*</span>
                  </span>
                }
                rules={[{ required: true, message: 'Please select schedule type' }]}
                style={{ marginBottom: '32px' }}
              >
                <Select 
                  placeholder="Select schedule type"
                  value={selectedScheduleType}
                  onChange={(value) => {
                    setSelectedScheduleType(value);
                    form.setFieldsValue({ schedule: value });
                  }}
                  style={{
                    borderRadius: '8px',
                    fontSize: '14px'
                  }}
                  suffixIcon={<div style={{
                    transform: 'rotate(90deg)',
                    fontSize: '12px',
                    color: '#9ca3af'
                  }}>❯</div>}
                >
                  <Option value="continuous">
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <div style={{
                        width: '8px',
                        height: '8px',
                        borderRadius: '50%',
                        backgroundColor: '#00be62',
                        marginRight: '12px'
                      }} />
                      <span style={{ fontWeight: '500', color: '#1f2937' }}>Continuous</span>
                      <span style={{ 
                        marginLeft: '8px', 
                        color: '#9ca3af',
                        fontSize: '14px'
                      }}>
                        - Real-time sync
                      </span>
                    </div>
                  </Option>
                  <Option value="hourly">
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <ClockCircleOutlined style={{
                        fontSize: '16px',
                        color: '#6b7280',
                        marginRight: '12px'
                      }} />
                      <span style={{ fontWeight: '500', color: '#1f2937' }}>Hourly</span>
                      <span style={{ 
                        marginLeft: '8px', 
                        color: '#9ca3af',
                        fontSize: '14px'
                      }}>
                        - Every hour
                      </span>
                    </div>
                  </Option>
                  <Option value="daily">
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <CalendarOutlined style={{
                        fontSize: '16px',
                        color: '#6b7280',
                        marginRight: '12px'
                      }} />
                      <span style={{ fontWeight: '500', color: '#1f2937' }}>Daily</span>
                      <span style={{ 
                        marginLeft: '8px', 
                        color: '#9ca3af',
                        fontSize: '14px'
                      }}>
                        - Once per day
                      </span>
                    </div>
                  </Option>
                </Select>
              </Form.Item>

              {/* Conditional Run Time Field - Only show when Daily is selected */}
              {selectedScheduleType === 'daily' && (
                <div style={{
                  backgroundColor: '#f8f9fa',
                  border: '1px solid #e9ecef',
                  borderRadius: '12px',
                  padding: '20px',
                  marginBottom: '32px'
                }}>
                  <h4 style={{
                    margin: '0 0 16px 0',
                    fontSize: '16px',
                    fontWeight: '500',
                    color: '#374151'
                  }}>
                    Run Time
                  </h4>
                  
                  <Form.Item
                    name="run_time"
                    style={{ marginBottom: '0' }}
                  >
                    <Input
                      placeholder="12:00"
                      value={selectedTime}
                      readOnly
                      onClick={handleTimePickerClick}
                      style={{
                        width: '100%',
                        borderRadius: '8px',
                        fontSize: '14px',
                        cursor: 'pointer'
                      }}
                      suffix={<ClockCircleOutlined style={{ color: '#9ca3af' }} />}
                    />
                  </Form.Item>
                </div>
              )}

              {/* Border Separator */}
              <div style={{
                borderTop: '1px solid #e5e7eb',
                margin: '32px 0',
                width: '100%'
              }} />

              {/* Advanced Settings Header */}
              <div style={{ 
                display: 'flex', 
                alignItems: 'center', 
                marginBottom: '20px'
              }}>
                <SettingOutlined style={{ 
                  fontSize: '16px', 
                  color: '#00be62',
                  marginRight: '8px' 
                }} />
                <span style={{
                  fontSize: '16px',
                  fontWeight: '600',
                  color: '#374151'
                }}>
                  Advanced Settings
                </span>
              </div>
              
              {/* Advanced Settings Fields */}
              <Row gutter={16}>
                <Col span={8}>
                  <Form.Item
                    name="rescan_interval"
                    label={
                      <span style={{ 
                        color: '#374151', 
                        fontWeight: '500',
                        fontSize: '14px'
                      }}>
                        Rescan Interval
                      </span>
                    }
                    style={{ marginBottom: '20px' }}
                  >
                    <InputNumber 
                      placeholder="3600"
                      min={0}
                      style={{
                        width: '100%',
                        borderRadius: '8px',
                        fontSize: '14px'
                      }}
                    />
                  </Form.Item>
                  <div style={{
                    fontSize: '12px',
                    color: '#6b7280',
                    marginTop: '-15px',
                    marginBottom: '20px'
                  }}>
                    Seconds
                  </div>
                </Col>
                {/* Temporarily commented out - Max File Size */}
                {/* <Col span={8}>
                  <Form.Item
                    name="max_file_size"
                    label={
                      <span style={{ 
                        color: '#374151', 
                        fontWeight: '500',
                        fontSize: '14px'
                      }}>
                        Max File Size
                      </span>
                    }
                    style={{ marginBottom: '20px' }}
                  >
                    <InputNumber 
                      placeholder="100"
                      min={0}
                      style={{
                        width: '100%',
                        borderRadius: '8px',
                        fontSize: '14px'
                      }}
                    />
                  </Form.Item>
                  <div style={{
                    fontSize: '12px',
                    color: '#6b7280',
                    marginTop: '-15px',
                    marginBottom: '20px'
                  }}>
                    MB (0 = no limit)
                  </div>
                </Col> */}
                {/* Temporarily commented out - Bandwidth Limit */}
                {/* <Col span={8}>
                  <Form.Item
                    name="bandwidth_limit"
                    label={
                      <span style={{ 
                        color: '#374151', 
                        fontWeight: '500',
                        fontSize: '14px'
                      }}>
                        Bandwidth Limit
                      </span>
                    }
                    style={{ marginBottom: '20px' }}
                  >
                    <InputNumber 
                      placeholder="50"
                      min={0}
                      style={{
                        width: '100%',
                        borderRadius: '8px',
                        fontSize: '14px'
                      }}
                    />
                  </Form.Item>
                  <div style={{
                    fontSize: '12px',
                    color: '#6b7280',
                    marginTop: '-15px',
                    marginBottom: '20px'
                  }}>
                    MB/s (0 = unlimited)
                  </div>
                </Col> */}
              </Row>

              {/* Border Separator */}
              <div style={{
                borderTop: '1px solid #e5e7eb',
                margin: '32px 0',
                width: '100%'
              }} />

              {/* Ignore Patterns Section */}
              <div style={{ 
                display: 'flex', 
                alignItems: 'center', 
                marginBottom: '20px'
              }}>
                <FilterOutlined style={{ 
                  fontSize: '16px', 
                  color: '#00be62',
                  marginRight: '8px' 
                }} />
                <span style={{
                  fontSize: '16px',
                  fontWeight: '600',
                  color: '#374151'
                }}>
                  Ignore Patterns
                </span>
                <span style={{
                  fontSize: '14px',
                  color: '#9ca3af',
                  marginLeft: '8px',
                  fontWeight: '400'
                }}>
                  (Optional)
                </span>
              </div>
              
              <Form.Item
                name="ignore_patterns"
                style={{ marginBottom: '0' }}
              >
                <TextArea
                  rows={4}
                  placeholder="*.tmp&#10;.DS_Store&#10;node_modules/&#10;*.log"
                  style={{
                    borderRadius: '8px',
                    border: '1px solid #d1d5db',
                    fontSize: '14px',
                    fontFamily: 'monospace',
                    resize: 'vertical'
                  }}
                />
              </Form.Item>
              <div style={{
                fontSize: '12px',
                color: '#6b7280',
                marginTop: '8px'
              }}>
                One pattern per line. Standard glob syntax supported (* ? [])
              </div>
            </div>
          </div>
        </Form>
      </Modal>

      {/* File Browser Modal - Following browse_folder.md documentation */}
      <Modal
        title={`Browse ${browsingFor ? browsingFor.charAt(0).toUpperCase() + browsingFor.slice(1) : ''} Folders on ${browsingAgent?.hostname}`}
        open={browserModalVisible}
        onCancel={() => setBrowserModalVisible(false)}
        width={600}
        footer={[
          <Button key="cancel" onClick={() => setBrowserModalVisible(false)}>
            Cancel
          </Button>,
          <Button 
            key="select" 
            onClick={handleConfirmSelection}
            disabled={!selectedPath}
            style={{ 
              backgroundColor: '#00be62', 
              borderColor: '#00be62',
              color: 'white',
              fontWeight: '500',
              border: '1px solid #00be62'
            }}
            onMouseEnter={(e) => {
              if (!e.target.disabled) {
                e.target.style.backgroundColor = '#00a855';
                e.target.style.borderColor = '#00a855';
              }
            }}
            onMouseLeave={(e) => {
              if (!e.target.disabled) {
                e.target.style.backgroundColor = '#00be62';
                e.target.style.borderColor = '#00be62';
              }
            }}
          >
            Select
          </Button>,
        ]}
        styles={{
          body: {
            padding: '16px',
            maxHeight: '400px',
            overflowY: 'auto'
          }
        }}
      >
        {loadingTree ? (
          <div style={{ textAlign: 'center', padding: '50px' }}>
            <Spin size="large" />
          </div>
        ) : (
          <>
            {selectedPath && (
              <div style={{ marginBottom: 16, padding: 8, backgroundColor: '#f0f0f0', borderRadius: 4 }}>
                <Text strong>Selected: </Text>
                <Text code>{selectedPath}</Text>
              </div>
            )}
            <DirectoryTree
              treeData={folderTree}
              onSelect={handleSelectFolder}
              loadData={onLoadData}
              height={400}
            />
          </>
        )}
      </Modal>

      {/* Custom Time Picker Modal */}
      <Modal
        title="Select Time"
        open={timePickerModalVisible}
        onCancel={() => setTimePickerModalVisible(false)}
        footer={null}
        width={400}
        centered
      >
        <div style={{ padding: '20px 0' }}>
          {/* Hour and Minute Selectors */}
          <Row gutter={16} style={{ marginBottom: '24px' }}>
            <Col span={12}>
              <div style={{ marginBottom: '8px' }}>
                <span style={{ 
                  fontSize: '14px', 
                  fontWeight: '500', 
                  color: '#374151' 
                }}>
                  Hour
                </span>
              </div>
              <Select
                value={selectedHour}
                onChange={setSelectedHour}
                style={{ width: '100%' }}
                placeholder="Hour"
              >
                {Array.from({ length: 24 }, (_, i) => (
                  <Option key={i} value={i.toString().padStart(2, '0')}>
                    {i.toString().padStart(2, '0')}
                  </Option>
                ))}
              </Select>
            </Col>
            <Col span={12}>
              <div style={{ marginBottom: '8px' }}>
                <span style={{ 
                  fontSize: '14px', 
                  fontWeight: '500', 
                  color: '#374151' 
                }}>
                  Minute
                </span>
              </div>
              <Select
                value={selectedMinute}
                onChange={setSelectedMinute}
                style={{ width: '100%' }}
                placeholder="Minute"
              >
                {Array.from({ length: 60 }, (_, i) => (
                  <Option key={i} value={i.toString().padStart(2, '0')}>
                    {i.toString().padStart(2, '0')}
                  </Option>
                ))}
              </Select>
            </Col>
          </Row>

          {/* Quick Select */}
          <div style={{ marginBottom: '24px' }}>
            <div style={{ marginBottom: '12px' }}>
              <span style={{ 
                fontSize: '14px', 
                fontWeight: '500', 
                color: '#374151' 
              }}>
                Quick Select
              </span>
            </div>
            <div style={{ 
              display: 'grid', 
              gridTemplateColumns: 'repeat(3, 1fr)', 
              gap: '8px' 
            }}>
              {['08:00', '12:00', '18:00', '00:00', '06:00', '23:00'].map((time) => (
                <Button
                  key={time}
                  onClick={() => handleQuickTimeSelect(time)}
                  style={{
                    borderRadius: '8px',
                    borderColor: selectedTime === time ? '#00be62' : '#d1d5db',
                    backgroundColor: selectedTime === time ? '#f0fdf4' : 'white',
                    color: selectedTime === time ? '#00be62' : '#374151'
                  }}
                >
                  {time}
                </Button>
              ))}
            </div>
          </div>

          {/* Apply Button */}
          <Button
            type="primary"
            block
            onClick={handleApplyTime}
            style={{
              backgroundColor: '#00be62',
              borderColor: '#00be62',
              borderRadius: '8px',
              height: '44px',
              fontWeight: '500'
            }}
          >
            Apply Time
          </Button>
        </div>
      </Modal>
    </>
  );
}

export default Jobs;