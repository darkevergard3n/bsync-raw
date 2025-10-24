import React, { createContext, useContext, useEffect, useState } from 'react';
import { useAuth } from './AuthContext';

const WebSocketContext = createContext(null);

export const useWebSocket = () => {
  const context = useContext(WebSocketContext);
  if (!context) {
    throw new Error('useWebSocket must be used within WebSocketProvider');
  }
  return context;
};

export const WebSocketProvider = ({ children }) => {
  const [connected, setConnected] = useState(false);
  const [socket, setSocket] = useState(null);
  const [fileTransferLogs, setFileTransferLogs] = useState([]);
  const { user, token } = useAuth();
  
  useEffect(() => {
    if (!user || !token) return;
    
    console.log('[WebSocket] Simple connection attempt');
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.hostname}:8090/ws?token=${encodeURIComponent(token)}`;
    
    const ws = new WebSocket(wsUrl);
    
    ws.onopen = () => {
      console.log('[WebSocket] Simple connection established');
      setConnected(true);
      setSocket(ws);
    };
    
    ws.onmessage = (event) => {
      try {
        const wsMessage = JSON.parse(event.data);
        console.log('[WebSocket] Message received:', wsMessage);
        
        if (wsMessage.type === 'file_transfer_log') {
          setFileTransferLogs(prev => [
            {
              id: wsMessage.data.id || Date.now() + Math.random(),
              ...wsMessage.data,
              timestamp: new Date(wsMessage.data.timestamp)
            },
            ...prev.slice(0, 199)
          ]);
        }
      } catch (error) {
        console.error('[WebSocket] Parse error:', error);
      }
    };
    
    ws.onclose = (event) => {
      console.log('[WebSocket] Simple connection closed:', event.code);
      setConnected(false);
      setSocket(null);
    };
    
    ws.onerror = (error) => {
      console.error('[WebSocket] Simple connection error:', error);
    };
    
    return () => {
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close();
      }
    };
  }, [user, token]);
  
  const clearFileTransferLogs = () => {
    setFileTransferLogs([]);
  };
  
  const value = {
    connected,
    socket,
    fileTransferLogs,
    clearFileTransferLogs,
    connectionState: connected ? 'connected' : 'disconnected',
    isReconnecting: false,
    
    // Mock values for compatibility
    notifications: [],
    onlineAgents: 0,
    totalAgents: 0,
    jobStatuses: new Map(),
    jobHistory: [],
    clearNotifications: () => {},
    clearJobHistory: () => {},
    formatFileSize: (bytes) => {
      if (bytes === 0) return '0 B';
      const k = 1024;
      const sizes = ['B', 'KB', 'MB', 'GB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    },
    getJobStats: () => ({ total: 0, running: 0, success: 0, failed: 0, idle: 0 }),
    syncingFolders: 0,
    activeFolders: 0,
  };
  
  return (
    <WebSocketContext.Provider value={value}>
      {children}
    </WebSocketContext.Provider>
  );
};