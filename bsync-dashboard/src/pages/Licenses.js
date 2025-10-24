import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  Table,
  Button,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Tag,
  message,
  Typography,
  Row,
  Col,
  DatePicker,
  Alert
} from 'antd';
import {
  KeyOutlined,
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  ReloadOutlined,
  SearchOutlined,
  UserAddOutlined
} from '@ant-design/icons';
import { licenseAPI, agentLicenseAPI, unlicensedAgentsAPI } from '../services/api';

const { Option } = Select;
const { Text } = Typography;

function Licenses() {
  const [licenses, setLicenses] = useState([]);
  const [filteredLicenses, setFilteredLicenses] = useState([]);
  // Agents state is kept for potential future use
  const [, setAgents] = useState([]);
  const [unlicensedAgents, setUnlicensedAgents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [assignModalVisible, setAssignModalVisible] = useState(false);
  const [form] = Form.useForm();
  const [assignForm] = Form.useForm();
  const [selectedLicense, setSelectedLicense] = useState(null);
  const [searchText, setSearchText] = useState('');
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 10,
    total: 0,
  });

  // Define fetch functions with useCallback to prevent recreation on each render
  const fetchLicenses = useCallback(async () => {
    try {
      setLoading(true);
      const { current, pageSize } = pagination;
      const response = await licenseAPI.list({
        page: current,
        limit: pageSize,
      });
      
      setLicenses(response.data.data || []);
      setPagination(prev => ({
        ...prev,
        total: response.data.total || 0,
      }));
    } catch (error) {
      console.error('Error fetching licenses:', error);
      message.error('Failed to fetch licenses');
    } finally {
      setLoading(false);
    }
  }, [pagination]);

  const fetchAgents = useCallback(async () => {
    try {
      const response = await agentLicenseAPI.list();
      // Convert agents object to array and add agent_id if not present
      const agentsArray = Object.entries(response.data || {}).map(([id, agent]) => ({
        ...agent,
        id: agent.id || id,
        agent_id: agent.agent_id || id,
      }));
      setAgents(agentsArray);
    } catch (error) {
      console.error('Error fetching agents:', error);
      message.error('Failed to fetch agents');
    }
  }, []);

  const fetchUnlicensedAgents = useCallback(async () => {
    try {
      const response = await unlicensedAgentsAPI.list();
      setUnlicensedAgents(response.data || []);
    } catch (error) {
      console.error('Error fetching unlicensed agents:', error);
      message.error('Failed to fetch unlicensed agents');
    }
  }, []);

  // Fetch data on component mount and when pagination changes
  useEffect(() => {
    const fetchAllData = async () => {
      try {
        setLoading(true);
        await Promise.all([
          fetchLicenses(),
          fetchAgents(),
          fetchUnlicensedAgents()
        ]);
      } catch (error) {
        console.error('Error fetching data:', error);
        message.error('Failed to fetch data');
      } finally {
        setLoading(false);
      }
    };
    
    fetchAllData();
  }, [fetchLicenses, fetchAgents, fetchUnlicensedAgents]);

  // Apply filters and search
  useEffect(() => {
    let filtered = [...licenses];
    
    if (searchText) {
      const searchLower = searchText.toLowerCase();
      filtered = filtered.filter(license => 
        (license.license_key?.toLowerCase().includes(searchLower)) ||
        (license.assigned_agent?.hostname?.toLowerCase().includes(searchLower)) ||
        (license.assigned_agent?.ip_address?.includes(searchText)) ||
        (license.assigned_agent?.id?.toLowerCase().includes(searchLower))
      );
    }
    
    setFilteredLicenses(filtered);
  }, [searchText, licenses]);
  
  // Handle table pagination change
  const handleTableChange = (pagination, filters, sorter) => {
    setPagination({
      ...pagination,
      current: pagination.current,
      pageSize: pagination.pageSize,
    });
  };


  const handleCreate = () => {
    setSelectedLicense(null);
    form.resetFields();
    setModalVisible(true);
  };
  
  const handleAssignLicense = (license) => {
    setSelectedLicense(license);
    assignForm.setFieldsValue({
      license_id: license.id,
      agent_id: undefined,
    });
    setAssignModalVisible(true);
  };

  const handleEdit = (license) => {
    setSelectedLicense(license);
    form.setFieldsValue({
      agent_id: license.agent_id,
      license_key: license.license_key
    });
    setModalVisible(true);
  };

  const handleDelete = async (id) => {
    Modal.confirm({
      title: 'Delete License',
      content: 'Are you sure you want to delete this license? This will also stop any related jobs.',
      okText: 'Delete',
      okType: 'danger',
      cancelText: 'Cancel',
      onOk: async () => {
        try {
          await licenseAPI.delete(id);
          message.success('License deleted successfully');
          fetchLicenses();
          fetchUnlicensedAgents();
        } catch (error) {
          console.error('Error deleting license:', error);
          const errorMessage = error.response?.data?.message || 'Failed to delete license';
          message.error(errorMessage);
        }
      },
    });
  };
  
  const handleUnassign = async (agentId) => {
    Modal.confirm({
      title: 'Unassign License',
      content: 'Are you sure you want to unassign this license from the agent?',
      okText: 'Unassign',
      okType: 'danger',
      cancelText: 'Cancel',
      onOk: async () => {
        try {
          await agentLicenseAPI.unassign(agentId);
          message.success('License unassigned successfully');
          fetchLicenses();
          fetchUnlicensedAgents();
        } catch (error) {
          console.error('Error unassigning license:', error);
          const errorMessage = error.response?.data?.message || 'Failed to unassign license';
          message.error(errorMessage);
        }
      },
    });
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);
      
      if (selectedLicense) {
        await licenseAPI.update(selectedLicense.id, values);
        message.success('License updated successfully');
      } else {
        // Generate a unique license key if not provided
        if (!values.license_key) {
          values.license_key = `LIC-${Date.now()}-${Math.random().toString(36).substr(2, 8).toUpperCase()}`;
        }
        await licenseAPI.create(values);
        message.success('License created successfully');
      }
      
      setModalVisible(false);
      fetchLicenses();
      fetchUnlicensedAgents();
    } catch (error) {
      console.error('Error saving license:', error);
      const errorMessage = error.response?.data?.message || 'Failed to save license';
      message.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };
  
  const handleAssignSubmit = async () => {
    try {
      const values = await assignForm.validateFields();
      setLoading(true);
      
      await agentLicenseAPI.assign({
        license_id: values.license_id,
        agent_id: values.agent_id,
      });
      
      message.success('License assigned to agent successfully');
      setAssignModalVisible(false);
      fetchLicenses();
      fetchUnlicensedAgents();
    } catch (error) {
      console.error('Error assigning license:', error);
      const errorMessage = error.response?.data?.message || 'Failed to assign license';
      message.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const columns = [
    {
      title: 'License Key',
      dataIndex: 'license_key',
      key: 'license_key',
      render: (text, record) => (
        <Space>
          <KeyOutlined />
          <Text 
            ellipsis 
            style={{ maxWidth: 200, cursor: 'pointer' }}
            onClick={() => {
              setSelectedLicense(record);
              form.setFieldsValue(record);
              setModalVisible(true);
            }}
          >
            {text}
          </Text>
        </Space>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status) => {
        let color = 'default';
        if (status === 'active') color = 'success';
        if (status === 'expired') color = 'error';
        if (status === 'inactive') color = 'default';
        return <Tag color={color}>{status?.toUpperCase() || 'UNKNOWN'}</Tag>;
      },
      filters: [
        { text: 'Active', value: 'active' },
        { text: 'Inactive', value: 'inactive' },
        { text: 'Expired', value: 'expired' },
      ],
      onFilter: (value, record) => record.status === value,
    },
    {
      title: 'Assigned To',
      dataIndex: 'assigned_agent',
      key: 'assigned_agent',
      render: (agent, record) => {
        if (!agent) return <Tag>Unassigned</Tag>;
        return (
          <Space>
            <Tag color="blue">{agent.hostname || agent.id}</Tag>
            <Button
              type="link"
              size="small"
              onClick={(e) => {
                e.stopPropagation();
                handleUnassign(agent.id);
              }}
              danger
            >
              Unassign
            </Button>
          </Space>
        );
      },
    },
    {
      title: 'Created At',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date) => date ? new Date(date).toLocaleString() : 'N/A',
      sorter: (a, b) => new Date(a.created_at) - new Date(b.created_at),
    },
    {
      title: 'Expires At',
      dataIndex: 'expires_at',
      key: 'expires_at',
      render: (date) => date ? new Date(date).toLocaleDateString() : 'Never',
    },
    {
      title: 'Actions',
      key: 'actions',
      align: 'right',
      render: (_, record) => (
        <Space>
          {!record.assigned_agent && (
            <Button
              type="link"
              icon={<UserAddOutlined />}
              onClick={() => handleAssignLicense(record)}
              title="Assign to Agent"
            />
          )}
          <Button
            type="text"
            icon={<EditOutlined />}
            onClick={() => handleEdit(record)}
            title="Edit"
          />
          <Button
            type="text"
            danger
            icon={<DeleteOutlined />}
            onClick={() => handleDelete(record.id)}
            title="Delete"
          />
        </Space>
      ),
    },
  ];

  return (
    <div className="licenses-page">
      <Card
        title={
          <Space>
            <KeyOutlined />
            <span>License Management</span>
          </Space>
        }
        extra={
          <Space>
            <Input
              placeholder="Search licenses..."
              prefix={<SearchOutlined />}
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{ width: 200 }}
              allowClear
            />
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={handleCreate}
            >
              Map License
            </Button>
            <Button
              icon={<ReloadOutlined />}
              onClick={fetchLicenses}
              loading={loading}
            >
              Refresh
            </Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={filteredLicenses}
          rowKey="id"
          loading={loading}
          onChange={handleTableChange}
          pagination={{
            ...pagination,
            showSizeChanger: true,
            showTotal: (total) => `Total ${total} licenses`,
            pageSizeOptions: ['10', '20', '50', '100'],
          }}
          scroll={{ x: 'max-content' }}
          onRow={(record) => ({
            onClick: () => {
              setSelectedLicense(record);
              form.setFieldsValue(record);
              setModalVisible(true);
            },
            style: { cursor: 'pointer' },
          })}
        />
      </Card>

      {/* License Mapping Modal */}
      {/* License Form Modal */}
      <Modal
        title={
          <>
            <KeyOutlined /> {selectedLicense ? 'Edit License' : 'Create New License'}
          </>
        }
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
        confirmLoading={loading}
        width={700}
      >
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            status: 'active',
            license_type: 'perpetual',
            max_agents: 1,
          }}
        >
          <Row gutter={16}>
            <Col span={24}>
              <Form.Item
                name="license_key"
                label="License Key"
                rules={[
                  {
                    required: true,
                    message: 'Please enter a license key',
                  },
                ]}
              >
                <Input 
                  placeholder="Enter license key or leave blank to auto-generate"
                  disabled={!!selectedLicense}
                />
              </Form.Item>
            </Col>
            
            <Col span={12}>
              <Form.Item
                name="status"
                label="Status"
                rules={[{ required: true }]}
              >
                <Select>
                  <Option value="active">Active</Option>
                  <Option value="inactive">Inactive</Option>
                  <Option value="expired">Expired</Option>
                </Select>
              </Form.Item>
            </Col>
            
            <Col span={12}>
              <Form.Item
                name="license_type"
                label="License Type"
                rules={[{ required: true }]}
              >
                <Select>
                  <Option value="trial">Trial</Option>
                  <Option value="subscription">Subscription</Option>
                  <Option value="perpetual">Perpetual</Option>
                </Select>
              </Form.Item>
            </Col>
            
            <Col span={12}>
              <Form.Item
                name="max_agents"
                label="Max Agents"
                rules={[{ required: true }]}
              >
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            
            <Col span={12}>
              <Form.Item
                name="expires_at"
                label="Expiration Date"
              >
                <DatePicker 
                  style={{ width: '100%' }} 
                  showTime={false}
                  format="YYYY-MM-DD"
                  placeholder="Never expires"
                />
              </Form.Item>
            </Col>
            
            <Col span={24}>
              <Form.Item
                name="notes"
                label="Notes"
              >
                <Input.TextArea rows={3} placeholder="Additional notes about this license" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
      
      {/* Assign License Modal */}
      <Modal
        title={
          <>
            <UserAddOutlined /> Assign License to Agent
          </>
        }
        open={assignModalVisible}
        onOk={handleAssignSubmit}
        onCancel={() => setAssignModalVisible(false)}
        confirmLoading={loading}
        width={500}
      >
        <Form
          form={assignForm}
          layout="vertical"
        >
          <Form.Item name="license_id" hidden>
            <Input />
          </Form.Item>
          
          <Form.Item
            name="agent_id"
            label="Select Agent"
            rules={[{ required: true, message: 'Please select an agent' }]}
          >
            <Select
              placeholder="Select an agent"
              showSearch
              optionFilterProp="children"
              filterOption={(input, option) =>
                option.children.toLowerCase().includes(input.toLowerCase())
              }
            >
              {unlicensedAgents.map(agent => (
                <Option key={agent.id} value={agent.id}>
                  {agent.hostname} ({agent.ip_address}) - {agent.os}
                </Option>
              ))}
            </Select>
          </Form.Item>
          
          {selectedLicense && (
            <Alert 
              message="License Information"
              description={
                <Space direction="vertical">
                  <div><strong>Key:</strong> {selectedLicense.license_key}</div>
                  <div><strong>Type:</strong> {selectedLicense.license_type || 'N/A'}</div>
                  <div><strong>Status:</strong> {selectedLicense.status || 'N/A'}</div>
                </Space>
              }
              type="info"
              showIcon
            />
          )}
        </Form>
      </Modal>
    </div>
  );
}

export default Licenses;
