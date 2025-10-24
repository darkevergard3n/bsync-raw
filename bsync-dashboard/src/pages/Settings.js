import React, { useState, useEffect } from 'react';
import {
  Card,
  Button,
  Switch,
  Input,
  Row,
  Col,
  message,
  ConfigProvider,
  Collapse
} from 'antd';
import {
  BellOutlined,
  MailOutlined,
  SaveOutlined
} from '@ant-design/icons';

const { Panel } = Collapse;

function Settings() {
  const [emailNotifications, setEmailNotifications] = useState(true);
  const [smtpConfig, setSMTPConfig] = useState({
    server: 'smtp.company.com',
    port: '587',
    username: 'sync@company.com',
    password: '••••••••',
    senderEmail: 'noreply@domain.com',
    senderName: 'Sync Dashboard',
    enableTLS: false
  });
  const [loading, setLoading] = useState(false);

  // Load settings from API
  useEffect(() => {
    loadSettings();
  }, []);

  const loadSettings = async () => {
    try {
      // TODO: Fetch settings from API
      // const response = await api.get('/api/v1/settings');
      // setSMTPConfig(response.data.smtp);
      // setEmailNotifications(response.data.emailNotifications);
    } catch (error) {
      console.error('Failed to load settings:', error);
    }
  };

  const handleSaveChanges = async () => {
    setLoading(true);
    try {
      // TODO: Save settings to API
      // await api.post('/api/v1/settings', {
      //   emailNotifications,
      //   smtp: smtpConfig
      // });

      message.success('Settings saved successfully');
    } catch (error) {
      message.error('Failed to save settings');
      console.error('Error saving settings:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSMTPChange = (field, value) => {
    setSMTPConfig(prev => ({
      ...prev,
      [field]: value
    }));
  };

  return (
    <>
      <style>
        {`
          .settings-page {
            padding: 24px;
          }

          .settings-header {
            display: flex;
            justify-content: space-between;
            align-items: flex-start;
            margin-bottom: 24px;
          }

          .settings-header-left h1 {
            margin: 0;
            font-size: 28px;
            font-weight: 600;
            color: #1f2937;
          }

          .settings-header-left p {
            margin: 8px 0 0 0;
            font-size: 16px;
            color: #6b7280;
          }

          .settings-card {
            border-radius: 12px;
            border: 1px solid #e5e7eb;
            background: #ffffff;
          }

          .notification-section-header {
            display: flex;
            align-items: center;
            gap: 12px;
            padding: 24px;
            border-bottom: 1px solid #f3f4f6;
          }

          .notification-icon {
            width: 48px;
            height: 48px;
            background: #f0fdf4;
            border-radius: 12px;
            display: flex;
            align-items: center;
            justify-content: center;
          }

          .notification-section-content {
            flex: 1;
          }

          .notification-section-content h2 {
            margin: 0;
            font-size: 18px;
            font-weight: 600;
            color: #1f2937;
          }

          .notification-section-content p {
            margin: 4px 0 0 0;
            font-size: 14px;
            color: #6b7280;
          }

          .email-notification-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 20px 24px;
            border-bottom: 1px solid #f3f4f6;
          }

          .email-notification-left {
            display: flex;
            align-items: center;
            gap: 16px;
          }

          .email-icon {
            width: 48px;
            height: 48px;
            background: #f0fdf4;
            border-radius: 12px;
            display: flex;
            align-items: center;
            justify-content: center;
          }

          .email-notification-text h3 {
            margin: 0;
            font-size: 16px;
            font-weight: 600;
            color: #1f2937;
          }

          .email-notification-text p {
            margin: 4px 0 0 0;
            font-size: 14px;
            color: #6b7280;
          }

          .smtp-collapse {
            border: none !important;
            background: transparent !important;
          }

          .smtp-collapse .ant-collapse-item {
            border: none !important;
          }

          .smtp-collapse .ant-collapse-header {
            padding: 20px 24px !important;
            background: #ffffff !important;
            border-radius: 0 !important;
            display: flex;
            align-items: center;
            gap: 12px;
          }

          .smtp-collapse .ant-collapse-content {
            border-top: none !important;
            background: #ffffff !important;
          }

          .smtp-collapse .ant-collapse-content-box {
            padding: 24px !important;
            background: #f9fafb;
          }

          .smtp-status-indicator {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #22c55e;
          }

          .smtp-header-text {
            flex: 1;
            font-size: 15px;
            font-weight: 600;
            color: #1f2937;
          }

          .settings-form-label {
            display: block;
            margin-bottom: 8px;
            font-size: 14px;
            font-weight: 500;
            color: #374151;
          }

          .settings-input {
            border-radius: 8px !important;
            border: 1px solid #d1d5db !important;
            font-size: 14px !important;
            padding: 8px 12px !important;
          }

          .settings-input:hover,
          .settings-input:focus {
            border-color: #00be62 !important;
            box-shadow: 0 0 0 2px rgba(0, 190, 98, 0.1) !important;
          }

          .tls-toggle-container {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 16px;
            background: #ffffff;
            border-radius: 8px;
            border: 1px solid #e5e7eb;
          }

          .tls-toggle-left h4 {
            margin: 0;
            font-size: 14px;
            font-weight: 600;
            color: #1f2937;
          }

          .tls-toggle-left p {
            margin: 4px 0 0 0;
            font-size: 13px;
            color: #6b7280;
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
        <div className="settings-page">
          {/* Page Header */}
          <div className="settings-header">
            <div className="settings-header-left">
              <h1>System Settings</h1>
              <p>Configure global application settings and preferences</p>
            </div>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              size="large"
              loading={loading}
              onClick={handleSaveChanges}
              style={{
                backgroundColor: '#00be62',
                borderColor: '#00be62',
                color: 'white',
                fontWeight: '500',
                border: '1px solid #00be62',
                borderRadius: '8px'
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
              Save Changes
            </Button>
          </div>

          {/* Notification Settings Card */}
          <Card className="settings-card" bordered={false}>
            {/* Section Header */}
            <div className="notification-section-header">
              <div className="notification-icon">
                <BellOutlined style={{ fontSize: '24px', color: '#00be62' }} />
              </div>
              <div className="notification-section-content">
                <h2>Notification Settings</h2>
                <p>Configure how you receive system notifications</p>
              </div>
            </div>

            {/* Email Notifications Toggle */}
            <div className="email-notification-item">
              <div className="email-notification-left">
                <div className="email-icon">
                  <MailOutlined style={{ fontSize: '24px', color: '#00be62' }} />
                </div>
                <div className="email-notification-text">
                  <h3>Email Notifications</h3>
                  <p>Send notifications via email</p>
                </div>
              </div>
              <Switch
                checked={emailNotifications}
                onChange={setEmailNotifications}
                style={{
                  backgroundColor: emailNotifications ? '#00be62' : '#d1d5db'
                }}
              />
            </div>

            {/* SMTP Configuration Collapse */}
            <Collapse
              className="smtp-collapse"
              bordered={false}
              expandIconPosition="end"
              defaultActiveKey={['1']}
            >
              <Panel
                header={
                  <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                    <div className="smtp-status-indicator"></div>
                    <span className="smtp-header-text">SMTP Configuration</span>
                  </div>
                }
                key="1"
              >
                <Row gutter={[16, 16]}>
                  {/* SMTP Server & Port */}
                  <Col span={16}>
                    <label className="settings-form-label">SMTP Server</label>
                    <Input
                      className="settings-input"
                      placeholder="smtp.company.com"
                      value={smtpConfig.server}
                      onChange={(e) => handleSMTPChange('server', e.target.value)}
                    />
                  </Col>
                  <Col span={8}>
                    <label className="settings-form-label">Port</label>
                    <Input
                      className="settings-input"
                      placeholder="587"
                      value={smtpConfig.port}
                      onChange={(e) => handleSMTPChange('port', e.target.value)}
                    />
                  </Col>

                  {/* Username */}
                  <Col span={24}>
                    <label className="settings-form-label">Username</label>
                    <Input
                      className="settings-input"
                      placeholder="sync@company.com"
                      value={smtpConfig.username}
                      onChange={(e) => handleSMTPChange('username', e.target.value)}
                    />
                  </Col>

                  {/* Password */}
                  <Col span={24}>
                    <label className="settings-form-label">Password</label>
                    <Input.Password
                      className="settings-input"
                      placeholder="Enter password"
                      value={smtpConfig.password}
                      onChange={(e) => handleSMTPChange('password', e.target.value)}
                    />
                  </Col>

                  {/* Sender Email & Sender Name */}
                  <Col span={12}>
                    <label className="settings-form-label">Sender Email</label>
                    <Input
                      className="settings-input"
                      placeholder="noreply@domain.com"
                      value={smtpConfig.senderEmail}
                      onChange={(e) => handleSMTPChange('senderEmail', e.target.value)}
                    />
                  </Col>
                  <Col span={12}>
                    <label className="settings-form-label">Sender Name</label>
                    <Input
                      className="settings-input"
                      placeholder="Sync Dashboard"
                      value={smtpConfig.senderName}
                      onChange={(e) => handleSMTPChange('senderName', e.target.value)}
                    />
                  </Col>

                  {/* Enable TLS */}
                  <Col span={24}>
                    <div className="tls-toggle-container">
                      <div className="tls-toggle-left">
                        <h4>Enable TLS</h4>
                        <p>Use secure TLS connection for email delivery</p>
                      </div>
                      <Switch
                        checked={smtpConfig.enableTLS}
                        onChange={(checked) => handleSMTPChange('enableTLS', checked)}
                        style={{
                          backgroundColor: smtpConfig.enableTLS ? '#00be62' : '#d1d5db'
                        }}
                      />
                    </div>
                  </Col>
                </Row>
              </Panel>
            </Collapse>
          </Card>
        </div>
      </ConfigProvider>
    </>
  );
}

export default Settings;
