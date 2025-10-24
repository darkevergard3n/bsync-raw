import React, { createContext, useState, useContext, useEffect } from 'react';
import api from '../services/api';
import authService from '../services/authService';

const AuthContext = createContext(null);

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return context;
};

export const AuthProvider = ({ children }) => {
  const [user, setUser] = useState(null);
  const [token, setToken] = useState(null);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [loading, setLoading] = useState(true);

  // Check for stored auth on mount
  useEffect(() => {
    const storedToken = localStorage.getItem('token');
    const storedUser = localStorage.getItem('user');

    if (storedToken && storedUser && storedUser !== 'undefined' && storedUser !== 'null') {
      try {
        const userData = JSON.parse(storedUser);
        setToken(storedToken);
        setUser(userData);
        setIsAuthenticated(true);
        api.defaults.headers.common['Authorization'] = `Bearer ${storedToken}`;
      } catch (error) {
        console.error('Error parsing stored user data:', error);
        localStorage.removeItem('token');
        localStorage.removeItem('user');
      }
    }

    setLoading(false);
  }, []);

  const login = async (credentials) => {
    try {
      const result = await authService.login(credentials.username, credentials.password);

      if (result.success) {
        const { token: authToken, user: userData } = result.data;

        setToken(authToken);
        setUser(userData);
        setIsAuthenticated(true);

        // Set default authorization header
        api.defaults.headers.common['Authorization'] = `Bearer ${authToken}`;

        return { success: true };
      } else {
        return { success: false, message: result.error };
      }
    } catch (error) {
      console.error('Login error:', error);
      return {
        success: false,
        message: error.message || 'Login failed'
      };
    }
  };

  const logout = async () => {
    try {
      await authService.logout();
    } catch (error) {
      console.error('Logout error:', error);
    }

    setToken(null);
    setUser(null);
    setIsAuthenticated(false);

    // Remove authorization header
    delete api.defaults.headers.common['Authorization'];
  };

  const updateUser = (userData) => {
    setUser(userData);
    localStorage.setItem('user', JSON.stringify(userData));
  };

  // For development/demo purposes, provide mock authentication using real backend
  const mockLogin = async () => {
    console.log('[AuthContext] Starting demo login process...');
    
    try {
      console.log('[AuthContext] Attempting demo login with backend API...');
      console.log('[AuthContext] API base URL:', api.defaults.baseURL);
      
      // Use real backend with demo admin credentials
      const response = await api.post('/api/v1/auth/login', {
        username: 'admin',
        password: 'Secure123#$%'
      });
      
      console.log('[AuthContext] Demo login successful! Response:', response.data);
      
      const { token: authToken, user: userData } = response.data;
      
      if (!authToken || !userData) {
        throw new Error('Invalid response format: missing token or user data');
      }
      
      console.log('[AuthContext] Setting up authentication state...');
      console.log('[AuthContext] Token:', authToken ? 'Present' : 'Missing');
      console.log('[AuthContext] User data:', userData);
      
      setToken(authToken);
      setUser(userData);
      setIsAuthenticated(true);
      
      // Store in localStorage
      localStorage.setItem('synctool-token', authToken);
      localStorage.setItem('synctool-user', JSON.stringify(userData));
      
      // Set default authorization header for future requests
      api.defaults.headers.common['Authorization'] = `Bearer ${authToken}`;
      
      console.log('[AuthContext] Demo login completed successfully');
      return { success: true, message: 'Demo login successful' };
      
    } catch (error) {
      console.error('[AuthContext] Demo login failed:', error);
      console.error('[AuthContext] Error details:', {
        message: error.message,
        response: error.response?.data,
        status: error.response?.status,
        statusText: error.response?.statusText
      });
      
      // Fallback to mock data if backend is not available or credentials are wrong
      console.log('[AuthContext] Using fallback mock authentication...');
      
      const mockUser = {
        id: '1',
        username: 'admin',
        email: 'admin@synctool.local',
        role: 'admin'
      };
      const mockToken = 'mock-jwt-token-for-development';
      
      setToken(mockToken);
      setUser(mockUser);
      setIsAuthenticated(true);
      
      localStorage.setItem('synctool-token', mockToken);
      localStorage.setItem('synctool-user', JSON.stringify(mockUser));
      
      // Set mock authorization header
      api.defaults.headers.common['Authorization'] = `Bearer ${mockToken}`;
      
      console.log('[AuthContext] Fallback authentication setup complete');
      
      return { 
        success: false, 
        message: error.response?.data?.error || 'Demo login failed, using fallback authentication' 
      };
    }
  };

  const value = {
    user,
    token,
    isAuthenticated,
    loading,
    login,
    logout,
    updateUser,
    mockLogin, // For development
  };

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
};