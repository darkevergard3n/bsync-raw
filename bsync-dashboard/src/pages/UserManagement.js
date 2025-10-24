import React, { useState, useEffect } from 'react';
import {
  Card,
  Table,
  Button,
  Input,
  Select,
  Tag,
  Avatar,
  Dropdown,
  Space,
  Modal,
  Form,
  message,
  ConfigProvider,
} from 'antd';
import {
  EllipsisOutlined,
} from '@ant-design/icons';
import {
  Shield,
  User,
  Plus,
  Edit,
  Trash2,
  Search,
  Users,
  CheckCircle,
  CheckSquare,
  Square,
  Database
} from 'lucide-react';
import api from '../services/api';
import './UserManagement.css';

const { Option } = Select;

const UserManagement = () => {
  const [users, setUsers] = useState([]);
  const [filteredUsers, setFilteredUsers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [roleFilter, setRoleFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all');
  const [isAddModalVisible, setIsAddModalVisible] = useState(false);
  const [isEditModalVisible, setIsEditModalVisible] = useState(false);
  const [editingUser, setEditingUser] = useState(null);
  const [form] = Form.useForm();
  const [editForm] = Form.useForm();
  const [selectedRole, setSelectedRole] = useState(null);

  // Agent selection states
  const [agents, setAgents] = useState([]);
  const [selectedAgents, setSelectedAgents] = useState([]);
  const [agentSearchText, setAgentSearchText] = useState('');
  const [selectAllAgents, setSelectAllAgents] = useState(false);

  // Statistics
  const [stats, setStats] = useState({
    total: 0,
    admins: 0,
    operators: 0,
    active: 0,
  });

  // Roles
  const [roles, setRoles] = useState([]);

  useEffect(() => {
    fetchUsers();
    fetchRoles();
    fetchUserStats();
    fetchLicensedAgents();
  }, []);

  useEffect(() => {
    filterUsers();
  }, [users, searchText, roleFilter, statusFilter]);

  // Fetch roles for dropdown
  const fetchRoles = async () => {
    try {
      const response = await api.get('/api/v1/roles');
      if (response.data.success) {
        setRoles(response.data.data || []);
      }
    } catch (error) {
      console.error('Failed to fetch roles:', error);
    }
  };

  // Fetch user statistics
  const fetchUserStats = async () => {
    try {
      const response = await api.get('/api/v1/dashboard/user-stats');
      if (response.data.success) {
        const data = response.data.data;
        setStats({
          total: data.total_users || 0,
          admins: data.total_admin || 0,
          operators: data.total_operator || 0,
          active: data.active_users || 0,
        });
      }
    } catch (error) {
      console.error('Failed to fetch user stats:', error);
    }
  };

  // Fetch licensed agents for operator assignment
  const fetchLicensedAgents = async () => {
    try {
      const response = await api.get('/api/v1/agents/licensed');
      if (response.data.success) {
        const agentArray = response.data.data || [];
        // Map API response to match existing structure
        const mappedAgents = agentArray.map(agent => ({
          id: agent.agent_id,
          agent_id: agent.agent_id,
          hostname: agent.hostname || agent.name,
          name: agent.name,
          status: agent.status,
          ip_address: agent.hostname, // Use hostname as ip_address for display
        }));
        setAgents(mappedAgents);
      }
    } catch (error) {
      console.error('Failed to fetch licensed agents:', error);
    }
  };

  const fetchUsers = async () => {
    setLoading(true);
    try {
      const response = await api.get('/api/v1/users');
      const userData = response.data.data || [];
      setUsers(userData);
      calculateStats(userData);
    } catch (error) {
      message.error('Failed to fetch users');
      console.error('Error fetching users:', error);
    } finally {
      setLoading(false);
    }
  };

  const calculateStats = (userData) => {
    setStats({
      total: userData.length,
      admins: userData.filter(u => u.role === 'admin').length,
      operators: userData.filter(u => u.role === 'operator').length,
      active: userData.filter(u => u.status === 'active').length,
    });
  };

  const filterUsers = () => {
    let filtered = [...users];

    // Search filter
    if (searchText) {
      filtered = filtered.filter(user =>
        user.username.toLowerCase().includes(searchText.toLowerCase()) ||
        user.email.toLowerCase().includes(searchText.toLowerCase()) ||
        user.fullname?.toLowerCase().includes(searchText.toLowerCase())
      );
    }

    // Role filter
    if (roleFilter !== 'all') {
      filtered = filtered.filter(user => user.role === roleFilter);
    }

    // Status filter
    if (statusFilter !== 'all') {
      filtered = filtered.filter(user => user.status === statusFilter);
    }

    setFilteredUsers(filtered);
  };

  const handleAddUser = async (values) => {
    try {
      // Prepare payload with assigned_agents for operators
      const payload = {
        ...values,
        // Only include assigned_agents for operator role
        ...(values.role === 'operator' && { assigned_agents: selectedAgents })
      };

      await api.post('/api/v1/users', payload);
      message.success('User added successfully');
      setIsAddModalVisible(false);
      form.resetFields();
      setSelectedAgents([]);
      setAgentSearchText('');
      setSelectAllAgents(false);
      fetchUsers();
    } catch (error) {
      message.error(error.response?.data?.error || 'Failed to add user');
    }
  };

  const handleEditUser = async (values) => {
    try {
      await api.put(`/api/v1/users/${editingUser.id}`, values);
      message.success('User updated successfully');
      setIsEditModalVisible(false);
      editForm.resetFields();
      setEditingUser(null);
      fetchUsers();
    } catch (error) {
      message.error(error.response?.data?.error || 'Failed to update user');
    }
  };

  const handleDeleteUser = (userId) => {
    Modal.confirm({
      title: 'Delete User',
      content: 'Are you sure you want to delete this user?',
      okText: 'Delete',
      okType: 'danger',
      onOk: async () => {
        try {
          await api.delete(`/api/v1/users/${userId}`);
          message.success('User deleted successfully');
          fetchUsers();
        } catch (error) {
          message.error(error.response?.data?.error || 'Failed to delete user');
        }
      },
    });
  };

  const handleEditClick = (user) => {
    setEditingUser(user);
    editForm.setFieldsValue({
      username: user.username,
      email: user.email,
      fullname: user.fullname,
      role: user.role,
      status: user.status,
    });
    setIsEditModalVisible(true);
  };

  const getActionMenu = (record) => ({
    items: [
      {
        key: 'edit',
        icon: <Edit size={14} />,
        label: 'Edit',
        onClick: () => handleEditClick(record),
      },
      {
        key: 'delete',
        icon: <Trash2 size={14} />,
        label: 'Delete',
        danger: true,
        onClick: () => handleDeleteUser(record.id),
      },
    ],
  });

  const columns = [
    {
      title: 'User',
      dataIndex: 'fullname',
      key: 'fullname',
      width: 200,
      sorter: (a, b) => (a.fullname || '').localeCompare(b.fullname || ''),
      render: (text, record) => (
        <Space>
          <Avatar
            icon={<User size={14} />}
            src={record.avatar}
            style={{ backgroundColor: '#e0e7ff', color: '#4f46e5' }}
          />
          <span style={{ color: '#1f2937', fontWeight: 500 }}>
            {text || record.username}
          </span>
        </Space>
      ),
    },
    {
      title: 'Username',
      dataIndex: 'username',
      key: 'username',
      width: 150,
      sorter: (a, b) => a.username.localeCompare(b.username),
      render: (text) => (
        <span style={{ color: '#1f2937' }}>{text}</span>
      ),
    },
    {
      title: 'Email',
      dataIndex: 'email',
      key: 'email',
      width: 200,
      sorter: (a, b) => a.email.localeCompare(b.email),
      render: (text) => (
        <span style={{ color: '#6b7280', fontSize: '13px' }}>{text}</span>
      ),
    },
    {
      title: 'Role',
      dataIndex: 'role',
      key: 'role',
      width: 130,
      sorter: (a, b) => a.role.localeCompare(b.role),
      render: (role) => (
        <Tag
          icon={role === 'admin' ? <Shield size={12} /> : <Users size={12} />}
          color={role === 'admin' ? 'purple' : 'blue'}
          style={{
            borderRadius: '6px',
            padding: '2px 10px',
            fontSize: '12px',
            fontWeight: 500
          }}
        >
          {role.charAt(0).toUpperCase() + role.slice(1)}
        </Tag>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      sorter: (a, b) => a.status.localeCompare(b.status),
      render: (status) => (
        <Tag
          color={status === 'active' ? 'success' : 'error'}
          style={{
            borderRadius: '6px',
            padding: '2px 10px',
            fontSize: '12px',
            fontWeight: 500
          }}
        >
          {status.charAt(0).toUpperCase() + status.slice(1)}
        </Tag>
      ),
    },
    {
      title: 'Last Login',
      dataIndex: 'last_login',
      key: 'last_login',
      width: 160,
      sorter: (a, b) => new Date(a.last_login) - new Date(b.last_login),
      render: (date) => (
        <span style={{ color: '#6b7280', fontSize: '13px' }}>
          {date ? new Date(date).toLocaleString() : 'Never'}
        </span>
      ),
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 130,
      sorter: (a, b) => new Date(a.created_at) - new Date(b.created_at),
      render: (date) => (
        <span style={{ color: '#6b7280', fontSize: '13px' }}>
          {new Date(date).toLocaleDateString()}
        </span>
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      fixed: 'right',
      width: 120,
      render: (_, record) => (
        <Space size="middle">
          <Button
            type="text"
            size="small"
            icon={<Edit size={14} />}
            onClick={() => handleEditClick(record)}
            title="Edit"
            style={{ color: '#6b7280' }}
          />
          <Dropdown menu={getActionMenu(record)} trigger={['click']}>
            <Button
              type="text"
              size="small"
              icon={<EllipsisOutlined />}
              style={{ color: '#6b7280' }}
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
          .custom-table .ant-table-thead > tr > th {
            background-color: #fafafa !important;
            border-bottom: 1px solid #e5e7eb !important;
            font-weight: 600 !important;
            color: #374151 !important;
            padding: 12px 16px !important;
          }

          .custom-table .ant-table-tbody > tr:hover > td {
            background-color: #f9fafb !important;
          }

          .custom-table .ant-table-tbody > tr > td {
            border-bottom: 1px solid #f3f4f6 !important;
            padding: 12px 16px !important;
          }

          .custom-button:hover {
            border-color: #00be62 !important;
            color: inherit !important;
          }

          .custom-button:focus {
            border-color: #00be62 !important;
            color: inherit !important;
          }
        `}
      </style>
      <ConfigProvider
        theme={{
          token: {
            colorPrimary: '#00be62',
          },
        }}
      >
        <div className="user-management">
        {/* Header */}
        <div className="page-header">
        <div>
          <h1>User Management</h1>
          <p>Manage user accounts and access permissions</p>
        </div>
        <Button
          icon={<Plus size={16} />}
          size="large"
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
          onClick={() => setIsAddModalVisible(true)}
        >
          Add User
        </Button>
      </div>

      {/* Statistics Cards */}
      <div className="stats-grid">
        <Card className="stat-card">
          <div className="stat-content">
            <div className="stat-icon">
              <User size={18} />
            </div>
            <div className="stat-info">
              <div className="stat-label">Total Users</div>
              <div className="stat-value">{stats.total}</div>
            </div>
          </div>
        </Card>

        <Card className="stat-card">
          <div className="stat-content">
            <div className="stat-icon">
              <Shield size={18} />
            </div>
            <div className="stat-info">
              <div className="stat-label">Administrators</div>
              <div className="stat-value">{stats.admins}</div>
            </div>
          </div>
        </Card>

        <Card className="stat-card">
          <div className="stat-content">
            <div className="stat-icon">
              <Users size={18} />
            </div>
            <div className="stat-info">
              <div className="stat-label">Operators</div>
              <div className="stat-value">{stats.operators}</div>
            </div>
          </div>
        </Card>

        <Card className="stat-card">
          <div className="stat-content">
            <div className="stat-icon">
              <CheckCircle size={18} />
            </div>
            <div className="stat-info">
              <div className="stat-label">Active Users</div>
              <div className="stat-value">{stats.active}</div>
            </div>
          </div>
        </Card>
      </div>

      {/* User Accounts Table */}
      <Card className="users-table-card">
        <div className="table-header">
          <h2>User Accounts</h2>
          <Space>
            <Input
              placeholder="Search users..."
              prefix={<Search size={16} />}
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{ width: 250 }}
            />
            <Select
              value={roleFilter}
              onChange={setRoleFilter}
              style={{ width: 150 }}
            >
              <Option value="all">All Roles</Option>
              <Option value="admin">Admin</Option>
              <Option value="operator">Operator</Option>
            </Select>
            <Select
              value={statusFilter}
              onChange={setStatusFilter}
              style={{ width: 150 }}
            >
              <Option value="all">All Status</Option>
              <Option value="active">Active</Option>
              <Option value="disabled">Disabled</Option>
            </Select>
          </Space>
        </div>

        <Table
          className="custom-table"
          columns={columns}
          dataSource={filteredUsers}
          loading={loading}
          rowKey="id"
          pagination={false}
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
            Showing 1 to {Math.min(5, filteredUsers.length)} of {filteredUsers.length} entries
          </span>
          <div style={{ display: 'flex', alignItems: 'center', gap: '24px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ color: '#6b7280', fontSize: '14px' }}>Rows per page</span>
              <select
                style={{
                  border: '1px solid #d1d5db',
                  borderRadius: '8px',
                  padding: '4px 8px',
                  fontSize: '14px',
                  minWidth: '70px',
                  outline: 'none'
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

      {/* Add User Modal */}
      <Modal
        open={isAddModalVisible}
        onCancel={() => {
          setIsAddModalVisible(false);
          form.resetFields();
          setSelectedRole(null);
          setSelectedAgents([]);
          setAgentSearchText('');
          setSelectAllAgents(false);
        }}
        footer={null}
        width={800}
        closeIcon={<span style={{ fontSize: '16px', color: '#9ca3af' }}>×</span>}
        styles={{
          body: { padding: '24px 32px' }
        }}
      >
        <div>
          {/* Modal Header */}
          <div style={{ marginBottom: '24px' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px', marginBottom: '16px' }}>
              <div style={{
                width: '44px',
                height: '44px',
                borderRadius: '50%',
                background: '#e7f5ed',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                flexShrink: 0
              }}>
                <User size={20} color="#07be63" />
              </div>
              <div style={{ flex: 1 }}>
                <h2 style={{ margin: 0, fontSize: '16px', fontWeight: 600, color: '#1f2937', lineHeight: '22px' }}>
                  Add New User
                </h2>
                <p style={{ margin: '2px 0 0 0', fontSize: '13px', color: '#6b7280', lineHeight: '18px' }}>
                  Create a new user account with appropriate permissions and access levels.
                </p>
              </div>
            </div>
            <div style={{
              height: '1px',
              background: '#e5e7eb',
              margin: '0 -32px'
            }} />
          </div>

          <Form
            form={form}
            layout="vertical"
            onFinish={handleAddUser}
          >
            {/* General Information Section */}
            <div style={{ marginBottom: '20px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '16px' }}>
                <div style={{
                  width: '24px',
                  height: '24px',
                  borderRadius: '50%',
                  background: '#e7f5ed',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0
                }}>
                  <User size={12} color="#07be63" />
                </div>
                <h3 style={{ margin: 0, fontSize: '14px', fontWeight: 600, color: '#1f2937' }}>
                  General Information
                </h3>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
                <Form.Item
                  label={<span style={{ fontSize: '13px', fontWeight: 500 }}>Full Name</span>}
                  name="fullname"
                  rules={[{ required: true, message: 'Please input full name' }]}
                  style={{ marginBottom: 0 }}
                >
                  <Input
                    placeholder="Enter full name"
                    style={{ height: '36px', borderRadius: '6px', fontSize: '13px' }}
                  />
                </Form.Item>

                <Form.Item
                  label={<span style={{ fontSize: '13px', fontWeight: 500 }}>Username</span>}
                  name="username"
                  rules={[{ required: true, message: 'Please input username' }]}
                  style={{ marginBottom: 0 }}
                >
                  <Input
                    placeholder="Enter username"
                    style={{ height: '36px', borderRadius: '6px', fontSize: '13px' }}
                  />
                </Form.Item>
              </div>

              <Form.Item
                label={<span style={{ fontSize: '13px', fontWeight: 500 }}>Email Address</span>}
                name="email"
                rules={[
                  { required: true, message: 'Please input email' },
                  { type: 'email', message: 'Please enter valid email' }
                ]}
                style={{ marginTop: '12px', marginBottom: 0 }}
              >
                <Input
                  placeholder="Enter email address"
                  style={{ height: '36px', borderRadius: '6px', fontSize: '13px' }}
                />
              </Form.Item>

              <Form.Item
                label={<span style={{ fontSize: '13px', fontWeight: 500 }}>Role</span>}
                name="role"
                rules={[{ required: true, message: 'Please select role' }]}
                style={{ marginTop: '12px', marginBottom: 0 }}
              >
                <Select
                  placeholder="Select user role"
                  style={{ fontSize: '13px' }}
                  onChange={(value) => {
                    setSelectedRole(value);
                    // Reset agent selection when role changes
                    if (value !== 'operator') {
                      setSelectedAgents([]);
                      setAgentSearchText('');
                    }
                  }}
                >
                  {roles.map(role => (
                    <Option key={role.role_code} value={role.role_code}>
                      {role.role_label}
                    </Option>
                  ))}
                </Select>
              </Form.Item>
            </div>

            {/* Role Permissions Info */}
            {selectedRole === 'admin' && (
              <div style={{
                marginBottom: '20px',
                padding: '16px',
                background: '#f9f5ff',
                border: '1px solid #e9d5ff',
                borderRadius: '8px'
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
                  <Shield size={16} color="#9333ea" />
                  <h4 style={{ margin: 0, fontSize: '14px', fontWeight: 600, color: '#6b21a8' }}>
                    Administrator Permissions
                  </h4>
                </div>
                <p style={{ margin: 0, fontSize: '13px', color: '#7c3aed', lineHeight: '20px' }}>
                  <strong>Full System Access:</strong> Administrators have complete access to all agents, sync jobs, user management, and system settings. No specific agent assignment is required.
                </p>
              </div>
            )}

            {selectedRole === 'operator' && (
              <>
                {/* Agent Selection Section */}
                <div style={{ marginBottom: '20px' }}>
                  <div style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    marginBottom: '12px'
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <div style={{
                        width: '24px',
                        height: '24px',
                        borderRadius: '50%',
                        background: '#e7f5ed',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        flexShrink: 0
                      }}>
                        <Database size={12} color="#07be63" />
                      </div>
                      <h3 style={{ margin: 0, fontSize: '14px', fontWeight: 600, color: '#1f2937' }}>
                        Assigned Agents
                      </h3>
                      <span style={{
                        fontSize: '13px',
                        color: '#6b7280',
                        fontWeight: 400
                      }}>
                        {selectedAgents.length} selected
                      </span>
                    </div>
                  </div>

                  <p style={{
                    margin: '0 0 12px 0',
                    fontSize: '13px',
                    color: '#6b7280',
                    lineHeight: '18px'
                  }}>
                    Select the agents this operator will have access to manage and monitor.
                  </p>

                  {/* Search Agents */}
                  <Input
                    placeholder="Search agents..."
                    prefix={<Search size={14} color="#9ca3af" />}
                    value={agentSearchText}
                    onChange={(e) => setAgentSearchText(e.target.value)}
                    style={{
                      marginBottom: '12px',
                      height: '36px',
                      borderRadius: '6px',
                      fontSize: '13px'
                    }}
                  />

                  {/* Select All / Unselect All */}
                  <div style={{
                    display: 'flex',
                    gap: '8px',
                    marginBottom: '12px',
                    paddingBottom: '12px',
                    borderBottom: '1px solid #e5e7eb'
                  }}>
                    <Button
                      size="small"
                      icon={<CheckSquare size={14} />}
                      onClick={() => {
                        const filteredAgents = agents.filter(agent =>
                          agent.hostname?.toLowerCase().includes(agentSearchText.toLowerCase())
                        );
                        setSelectedAgents(filteredAgents.map(a => a.id));
                        setSelectAllAgents(true);
                      }}
                      style={{
                        fontSize: '12px',
                        height: '28px',
                        borderRadius: '4px',
                        display: 'flex',
                        alignItems: 'center'
                      }}
                    >
                      Select All
                    </Button>
                    <Button
                      size="small"
                      icon={<Square size={14} />}
                      onClick={() => {
                        setSelectedAgents([]);
                        setSelectAllAgents(false);
                      }}
                      style={{
                        fontSize: '12px',
                        height: '28px',
                        borderRadius: '4px',
                        display: 'flex',
                        alignItems: 'center'
                      }}
                    >
                      Unselect All
                    </Button>
                    <div style={{
                      flex: 1,
                      textAlign: 'right',
                      fontSize: '12px',
                      color: '#6b7280',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'flex-end'
                    }}>
                      {agents.filter(agent =>
                        agent.hostname?.toLowerCase().includes(agentSearchText.toLowerCase())
                      ).length} of {agents.length} agents
                    </div>
                  </div>

                  {/* Agent List */}
                  <div style={{
                    maxHeight: '320px',
                    overflowY: 'auto',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '12px',
                    padding: '16px',
                    background: 'white',
                    border: '1px solid #e5e7eb',
                    borderRadius: '8px'
                  }}>
                    {agents
                      .filter(agent =>
                        agent.hostname?.toLowerCase().includes(agentSearchText.toLowerCase())
                      )
                      .map((agent) => (
                        <div
                          key={agent.id}
                          onClick={() => {
                            setSelectedAgents(prev => {
                              if (prev.includes(agent.id)) {
                                return prev.filter(id => id !== agent.id);
                              } else {
                                return [...prev, agent.id];
                              }
                            });
                          }}
                          style={{
                            padding: '12px 14px',
                            background: 'white',
                            border: selectedAgents.includes(agent.id)
                              ? '2px solid #00be62'
                              : '1px solid #e5e7eb',
                            borderRadius: '12px',
                            cursor: 'pointer',
                            display: 'flex',
                            alignItems: 'center',
                            gap: '10px',
                            transition: 'all 0.2s',
                            boxShadow: '0 1px 2px rgba(0, 0, 0, 0.05)'
                          }}
                          onMouseEnter={(e) => {
                            e.currentTarget.style.boxShadow = '0 2px 4px rgba(0, 0, 0, 0.1)';
                          }}
                          onMouseLeave={(e) => {
                            e.currentTarget.style.boxShadow = '0 1px 2px rgba(0, 0, 0, 0.05)';
                          }}
                        >
                          {/* Checkbox */}
                          <div style={{
                            width: '16px',
                            height: '16px',
                            borderRadius: '3px',
                            border: selectedAgents.includes(agent.id)
                              ? '2px solid #00be62'
                              : '2px solid #d1d5db',
                            background: selectedAgents.includes(agent.id) ? '#00be62' : 'white',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            flexShrink: 0
                          }}>
                            {selectedAgents.includes(agent.id) && (
                              <span style={{ color: 'white', fontSize: '10px', fontWeight: 'bold' }}>✓</span>
                            )}
                          </div>

                          {/* Agent Icon */}
                          <div style={{
                            width: '32px',
                            height: '32px',
                            borderRadius: '6px',
                            background: '#f3f4f6',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            flexShrink: 0
                          }}>
                            <Database size={16} color="#9ca3af" />
                          </div>

                          {/* Agent Info */}
                          <div style={{ flex: 1, minWidth: 0 }}>
                            <div style={{
                              fontSize: '13px',
                              fontWeight: 600,
                              color: '#1f2937'
                            }}>
                              {agent.hostname || agent.id}
                            </div>
                          </div>

                          {/* Status Badge */}
                          <div style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '4px',
                            padding: '3px 9px',
                            borderRadius: '10px',
                            background: agent.status === 'online' ? '#d1fae5' : '#f3f4f6',
                            fontSize: '11px',
                            fontWeight: 500,
                            color: agent.status === 'online' ? '#059669' : '#6b7280'
                          }}>
                            {agent.status === 'online' ? 'Online' : 'Offline'}
                          </div>
                        </div>
                      ))}

                    {agents.filter(agent =>
                      agent.hostname?.toLowerCase().includes(agentSearchText.toLowerCase())
                    ).length === 0 && (
                      <div style={{
                        padding: '32px',
                        textAlign: 'center',
                        color: '#9ca3af',
                        fontSize: '13px',
                        background: 'white',
                        border: '1px solid #e5e7eb',
                        borderRadius: '8px'
                      }}>
                        No agents found
                      </div>
                    )}
                  </div>
                </div>
              </>
            )}

            {/* Hidden Status Field */}
            <Form.Item
              name="status"
              initialValue="active"
              hidden
            >
              <Input />
            </Form.Item>

            {/* Footer Actions */}
            <div style={{
              display: 'flex',
              justifyContent: 'flex-end',
              gap: '10px',
              paddingTop: '12px'
            }}>
              <Button
                onClick={() => {
                  setIsAddModalVisible(false);
                  form.resetFields();
                  setSelectedRole(null);
                  setSelectedAgents([]);
                  setAgentSearchText('');
                  setSelectAllAgents(false);
                }}
                style={{
                  height: '36px',
                  borderRadius: '6px',
                  padding: '0 20px',
                  fontSize: '13px',
                  fontWeight: 500
                }}
              >
                Cancel
              </Button>
              <Button
                htmlType="submit"
                icon={<Plus size={12} />}
                style={{
                  height: '36px',
                  borderRadius: '6px',
                  padding: '0 20px',
                  backgroundColor: '#00be62',
                  borderColor: '#00be62',
                  color: 'white',
                  fontSize: '13px',
                  fontWeight: 500
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
                Create User
              </Button>
            </div>
          </Form>
        </div>
      </Modal>

      {/* Edit User Modal */}
      <Modal
        title={<Space><Edit size={16} /> Edit User</Space>}
        open={isEditModalVisible}
        onCancel={() => {
          setIsEditModalVisible(false);
          editForm.resetFields();
          setEditingUser(null);
        }}
        footer={null}
        width={600}
      >
        <Form
          form={editForm}
          layout="vertical"
          onFinish={handleEditUser}
        >
          <Form.Item
            label="Username"
            name="username"
            rules={[{ required: true, message: 'Please input username' }]}
          >
            <Input placeholder="Enter username" disabled />
          </Form.Item>

          <Form.Item
            label="Email"
            name="email"
            rules={[
              { required: true, message: 'Please input email' },
              { type: 'email', message: 'Please enter valid email' }
            ]}
          >
            <Input placeholder="Enter email" />
          </Form.Item>

          <Form.Item
            label="Full Name"
            name="fullname"
            rules={[{ required: true, message: 'Please input full name' }]}
          >
            <Input placeholder="Enter full name" />
          </Form.Item>

          <Form.Item
            label="New Password (leave empty to keep current)"
            name="password"
          >
            <Input.Password placeholder="Enter new password" />
          </Form.Item>

          <Form.Item
            label="Role"
            name="role"
            rules={[{ required: true, message: 'Please select role' }]}
          >
            <Select placeholder="Select role">
              <Option value="admin">Administrator</Option>
              <Option value="operator">Operator</Option>
            </Select>
          </Form.Item>

          <Form.Item
            label="Status"
            name="status"
            rules={[{ required: true, message: 'Please select status' }]}
          >
            <Select>
              <Option value="active">Active</Option>
              <Option value="disabled">Disabled</Option>
            </Select>
          </Form.Item>

          <Form.Item>
            <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
              <Button onClick={() => {
                setIsEditModalVisible(false);
                editForm.resetFields();
                setEditingUser(null);
              }}>
                Cancel
              </Button>
              <Button type="primary" htmlType="submit">
                Update User
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      {/* Role Permissions & Access Levels */}
      <Card className="role-permissions-card">
        <div className="permissions-header">
          <h2>Role Permissions & Access Levels</h2>
          <p>Overview of what each role can access and manage</p>
        </div>

        <div className="permissions-grid">
          {/* Administrator Card */}
          <div className="permission-card admin-card">
            <div className="role-header">
              <div className="role-icon admin-icon">
                <Shield size={20} />
              </div>
              <div className="role-info">
                <h3>Administrator</h3>
                <p>Full Control</p>
              </div>
              <Tag color="purple">Admin</Tag>
            </div>

            <div className="permissions-list">
              <div className="permission-item">
                <span className="permission-bullet purple"></span>
                <div className="permission-content">
                  <h4>System Management</h4>
                  <p>Full system access & configuration</p>
                </div>
              </div>
              <div className="permission-item">
                <span className="permission-bullet purple"></span>
                <div className="permission-content">
                  <h4>User Management</h4>
                  <p>Create, edit, delete user accounts</p>
                </div>
              </div>
              <div className="permission-item">
                <span className="permission-bullet purple"></span>
                <div className="permission-content">
                  <h4>All Operations</h4>
                  <p>Complete sync & monitoring control</p>
                </div>
              </div>
            </div>
          </div>

          {/* Operator Card */}
          <div className="permission-card operator-card">
            <div className="role-header">
              <div className="role-icon operator-icon">
                <Users size={20} />
              </div>
              <div className="role-info">
                <h3>Operator</h3>
                <p>Operations Control</p>
              </div>
              <Tag color="blue">Operator</Tag>
            </div>

            <div className="permissions-list">
              <div className="permission-item">
                <span className="permission-bullet blue"></span>
                <div className="permission-content">
                  <h4>Sync Management</h4>
                  <p>Create & manage sync jobs</p>
                </div>
              </div>
              <div className="permission-item">
                <span className="permission-bullet blue"></span>
                <div className="permission-content">
                  <h4>Monitoring & Reports</h4>
                  <p>View detailed reports & analytics</p>
                </div>
              </div>
              <div className="permission-item">
                <span className="permission-bullet blue"></span>
                <div className="permission-content">
                  <h4>Agent Control</h4>
                  <p>Monitor & manage sync agents</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Security Note */}
        <div className="security-note-card">
          <div className="security-note">
            <div className="security-icon">
              <CheckCircle size={20} />
            </div>
            <div className="security-content">
              <h4>Security Note</h4>
              <p>
                All user activities are logged and monitored. Role permissions are enforced at both UI and API levels.
                Contact your administrator to request role changes or additional permissions.
              </p>
            </div>
          </div>
        </div>
      </Card>
        </div>
      </ConfigProvider>
    </>
  );
};

export default UserManagement;
