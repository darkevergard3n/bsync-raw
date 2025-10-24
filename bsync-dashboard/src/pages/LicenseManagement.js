import React, { useState, useEffect, useCallback } from 'react';
import { Table, Button, Modal, Form, Select, Space, Tag, message, Typography, Input } from 'antd';
import { 
  KeyOutlined,
  UserAddOutlined,
  ReloadOutlined
} from '@ant-design/icons';
import { licenseAPI, agentLicenseAPI, unlicensedAgentsAPI, agentAPI } from '../services/api';

const { Option } = Select;
const { Text } = Typography;

function LicenseManagement() {
  const [licenses, setLicenses] = useState([]);
  const [unlicensedAgents, setUnlicensedAgents] = useState([]);
  const [agents, setAgents] = useState({});
  const [loading, setLoading] = useState(true);
  const [assignModalVisible, setAssignModalVisible] = useState(false);
  // selectedLicense is used in handleAssignLicense to track which license is being assigned
  const [selectedLicense, setSelectedLicense] = useState(null);
  const [assignForm] = Form.useForm();
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 10,
    total: 0,
  });

  // Fetch all agents
  const fetchAgents = useCallback(async () => {
    try {
      const response = await agentAPI.list({ limit: 1000 }); // Adjust limit as needed
      if (response.data?.data) {
        const agentsMap = {};
        response.data.data.forEach(agent => {
          agentsMap[agent.agent_id] = agent;
        });
        setAgents(agentsMap);
      }
    } catch (error) {
      console.error('Error fetching agents:', error);
    }
  }, []);

  // Fetch data
  const fetchLicenses = useCallback(async (page = 1, pageSize = 10) => {
    try {
      setLoading(true);
      const [licensesResponse] = await Promise.all([
        licenseAPI.list({
          page,
          limit: pageSize,
        }),
        fetchAgents() // Ensure agents are loaded
      ]);
      
      // The API returns { data: [...], total: X } structure
      const licensesData = licensesResponse.data?.data || [];
      const total = licensesResponse.data?.total || 0;
      
      setLicenses(licensesData.map(license => {
        const agentInfo = license.agent_info || (license.assigned_to ? agents[license.assigned_to] : null);
        return {
          ...license,
          agent_info: agentInfo || undefined,
          assigned_to: license.assigned_to || (agentInfo ? agentInfo.agent_id : null),
          in_use: Boolean(license.in_use || license.assigned_to || agentInfo)
        };
      }));
      
      setPagination(prev => ({
        ...prev,
        current: page,
        pageSize,
        total,
      }));
    } catch (error) {
      console.error('Error fetching licenses:', error);
      message.error('Failed to fetch licenses');
      setLicenses([]);
    } finally {
      setLoading(false);
    }
  }, [agents, fetchAgents]);

  const fetchUnlicensedAgents = useCallback(async () => {
    try {
      const response = await unlicensedAgentsAPI.list();
      // The API returns { data: [...], total: X } structure
      const agents = Array.isArray(response?.data?.data) ? response.data.data : [];
      
      // Transform the data to match the expected format
      const formattedAgents = agents.map(agent => ({
        ...agent,
        // Map agent_id to value for the Select component
        value: agent.agent_id || agent.id,
        // Map hostname to label for the Select component
        label: agent.hostname || agent.agent_id || `Agent ${agent.id}`,
      }));
      
      setUnlicensedAgents(formattedAgents);
    } catch (error) {
      console.error('Error fetching unlicensed agents:', error);
      message.error('Failed to fetch unlicensed agents');
      // Set empty array on error to prevent map() errors
      setUnlicensedAgents([]);
    }
  }, []);

  // Initial data fetch
  useEffect(() => {
    const fetchData = async () => {
      await Promise.all([
        fetchLicenses(pagination.current, pagination.pageSize),
        fetchUnlicensedAgents()
      ]);
    };
    
    fetchData();
    // We only want to run this effect on component mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Handle table changes
  const handleTableChange = (newPagination) => {
    fetchLicenses(newPagination.current, newPagination.pageSize);
  };

  // Handle license assignment
  const handleAssignLicense = (license) => {
    setSelectedLicense(license);
    assignForm.setFieldsValue({
      license_id: license.id,
      agent_id: undefined,
    });
    setAssignModalVisible(true);
  };

  const handleAssignSubmit = async () => {
    try {
      const values = await assignForm.validateFields();
      
      // Call the API to assign the license
      await agentLicenseAPI.assign({
        license_id: values.license_id,
        agent_id: values.agent_id,
      });
      
      message.success('License assigned successfully');
      setAssignModalVisible(false);
      
      // Refresh both licenses and unlicensed agents
      await Promise.all([
        fetchLicenses(pagination.current, pagination.pageSize),
        fetchUnlicensedAgents()
      ]);
      
    } catch (error) {
      console.error('Error assigning license:', error);
      const errorMessage = error.response?.data?.message || 'Failed to assign license';
      message.error(errorMessage);
    }
  };

  // Handle license unassignment
  const handleUnassign = async (agentId) => {
    if (!agentId) {
      message.error('No agent ID provided for unassignment');
      return;
    }
    
    Modal.confirm({
      title: 'Unassign License',
      content: 'Are you sure you want to unassign this license?',
      okText: 'Unassign',
      okType: 'danger',
      cancelText: 'Cancel',
      onOk: async () => {
        try {
          await agentLicenseAPI.unassign(agentId);
          message.success('License unassigned successfully');
          
          // Refresh both licenses and unlicensed agents
          await Promise.all([
            fetchLicenses(pagination.current, pagination.pageSize),
            fetchUnlicensedAgents()
          ]);
          
        } catch (error) {
          console.error('Error unassigning license:', error);
          const errorMessage = error.response?.data?.message || 'Failed to unassign license';
          message.error(errorMessage);
        }
      },
    });
  };

  // Table columns
  const columns = [
    {
      title: 'License Key',
      dataIndex: 'license_key',
      key: 'license_key',
      render: (text) => (
        <Space>
          <KeyOutlined />
          <Text ellipsis style={{ maxWidth: 200 }}>
            {text}
          </Text>
        </Space>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'in_use',
      key: 'status',
      render: (inUse) => (
        <Tag color={inUse ? 'success' : 'default'}>
          {inUse ? 'IN USE' : 'AVAILABLE'}
        </Tag>
      ),
    },
    {
      title: 'Agent',
      key: 'agent_info',
      render: (_, record) => {
        if (!record.assigned_to) return <Tag>Unassigned</Tag>;
        
        const agent = record.agent_info || agents[record.assigned_to];
        if (!agent) return <span>-</span>;
        
        const osType = agent.os_type || 'Unknown OS';
        const osIcon = osType.toLowerCase().includes('windows') ? 'windows' : 
                      osType.toLowerCase().includes('linux') ? 'linux' : 'desktop';
        
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span className={`anticon anticon-${osIcon}`} style={{ fontSize: '16px', color: '#666' }} />
            <div>
              <div style={{ fontWeight: 500 }}>{agent.hostname || 'Unknown Host'}</div>
              <div style={{ fontSize: '12px', color: '#666' }}>ID: {agent.agent_id}</div>
            </div>
          </div>
        );
      },
    },
    {
      title: 'IP Address',
      key: 'ip_address',
      render: (_, record) => {
        if (!record.assigned_to) return '-';
        const agent = record.agent_info || agents[record.assigned_to];
        return agent?.ip_address || '-';
      },
    },
    {
      title: 'Operating System',
      key: 'os_type',
      render: (_, record) => {
        if (!record.assigned_to) return '-';
        const agent = record.agent_info || agents[record.assigned_to];
        // Check both agent.os_type and agent.os information
        return agent?.os_type || agent?.os || '-';
      },
    },
    {
      title: 'Actions',
      key: 'actions',
      align: 'right',
      render: (_, record) => (
        <Space>
          {record.assigned_to ? (
            <Button
              type="link"
              danger
              onClick={(e) => {
                e.stopPropagation();
                handleUnassign(record.agent_id || record.assigned_to);
              }}
            >
              Revoke
            </Button>
          ) : (
            <Button
              type="link"
              icon={<UserAddOutlined />}
              onClick={() => handleAssignLicense(record)}
              title="Assign to Agent"
            />
          )}
        </Space>
      ),
    },
  ];

  return (
    <div className="site-layout-background" style={{ padding: 24, minHeight: '100%' }}>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space>
          <Typography.Title level={4} style={{ margin: 0 }}>
            <KeyOutlined /> License Management
          </Typography.Title>
          <Button 
            type="text" 
            icon={<ReloadOutlined />} 
            onClick={() => {
              fetchLicenses(pagination.current, pagination.pageSize);
              fetchUnlicensedAgents();
            }}
            loading={loading}
          />
        </Space>
      </div>
      
      <div style={{ background: '#fff', padding: 24, borderRadius: 8 }}>
        <Table
          columns={columns}
          dataSource={licenses}
          rowKey="id"
          loading={loading}
          pagination={{
            ...pagination,
            showSizeChanger: true,
            pageSizeOptions: ['10', '20', '50', '100'],
            showTotal: (total, range) => `${range[0]}-${range[1]} of ${total} licenses`,
          }}
          onChange={handleTableChange}
          scroll={{ x: 'max-content' }}
        />
      </div>

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
                option.children.props.agent.hostname.toLowerCase().includes(input.toLowerCase()) ||
                option.children.props.agent.ip_address?.toLowerCase().includes(input.toLowerCase()) ||
                option.children.props.agent.agent_id?.toLowerCase().includes(input.toLowerCase())
              }
              style={{ width: '100%' }}
              dropdownStyle={{ maxWidth: '500px' }}
            >
              {unlicensedAgents.map(agent => (
                <Option key={agent.id} value={agent.value}>
                  <span 
                    style={{ 
                      fontSize: '14px',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap'
                    }} 
                    agent={agent}
                  >
                    {`${agent.hostname || 'Unknown Host'} - ${agent.ip_address || 'No IP'}`}
                  </span>
                </Option>
              ))}
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}

export default LicenseManagement;
