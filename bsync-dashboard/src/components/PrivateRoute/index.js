import React, { useEffect } from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { Spin } from 'antd';

function PrivateRoute({ children }) {
  const { isAuthenticated, loading } = useAuth();
  
  // Check if we have a token in localStorage as fallback
  const hasToken = localStorage.getItem('synctool-token');
  
  useEffect(() => {
    console.log('[PrivateRoute] Auth state:', { isAuthenticated, loading, hasToken });
  }, [isAuthenticated, loading, hasToken]);
  
  if (loading) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh' 
      }}>
        <Spin size="large" />
      </div>
    );
  }
  
  // Allow access if authenticated OR if we have a token (for refresh scenarios)
  return (isAuthenticated || hasToken) ? children : <Navigate to="/login" />;
}

export default PrivateRoute;
