import React, { useState, useEffect } from 'react';
import { BrowserRouter as Router, Route, Routes, Navigate } from 'react-router-dom';
import { ConfigProvider, theme } from 'antd';
import { AuthProvider } from './contexts/AuthContext';
import { WebSocketProvider } from './contexts/WebSocketContext';

// Pages
import Login from './pages/Login';
import Dashboard from './pages/Dashboard';
import Agents from './pages/Agents';
import Jobs from './pages/Jobs';
import UserManagement from './pages/UserManagement';
import Reports from './pages/Reports';
import Settings from './pages/Settings';
import FolderStats from './pages/FolderStats';
import Licenses from './pages/Licenses';
import LicenseManagement from './pages/LicenseManagement';

// Components
import Layout from './components/Layout';
import ProtectedRoute from './components/ProtectedRoute';

// Styles
import './styles/index.css';

const App = () => {
  const [isDarkMode, setIsDarkMode] = useState(false);

  // Initialize dark mode from localStorage
  useEffect(() => {
    const savedTheme = localStorage.getItem('synctool-theme');
    if (savedTheme === 'dark') {
      setIsDarkMode(true);
    }
  }, []);

  // Toggle dark mode
  const toggleTheme = () => {
    const newTheme = !isDarkMode;
    setIsDarkMode(newTheme);
    localStorage.setItem('synctool-theme', newTheme ? 'dark' : 'light');
  };

  return (
    <ConfigProvider
      theme={{
        algorithm: isDarkMode ? theme.darkAlgorithm : theme.defaultAlgorithm,
        token: {
          colorPrimary: '#1890ff',
          borderRadius: 8,
        },
      }}
    >
      <AuthProvider>
        <WebSocketProvider>
          <Router>
            <div className={`app ${isDarkMode ? 'dark-theme' : 'light-theme'}`}>
              <Routes>
                {/* Public routes */}
                <Route path="/login" element={<Login />} />

                {/* Protected routes */}
                <Route
                  path="/*"
                  element={
                    <ProtectedRoute>
                      <Layout onThemeToggle={toggleTheme} isDarkMode={isDarkMode}>
                        <Routes>
                          <Route path="/" element={<Navigate to="/dashboard" replace />} />
                          <Route path="/dashboard" element={<Dashboard />} />
                          <Route path="/agents" element={<Agents />} />
                          <Route path="/jobs" element={<Jobs />} />
                          <Route path="/licenses" element={<Licenses />} />
                          {/* <Route path="/license-management" element={<LicenseManagement />} /> */}
                          <Route path="/folder-stats" element={<FolderStats />} />
                          <Route path="/users" element={<UserManagement />} />
                          <Route path="/reports" element={<Reports />} />
                          <Route path="/settings" element={<Settings />} />
                        </Routes>
                      </Layout>
                    </ProtectedRoute>
                  }
                />
              </Routes>
            </div>
          </Router>
        </WebSocketProvider>
      </AuthProvider>
    </ConfigProvider>
  );
};

export default App;