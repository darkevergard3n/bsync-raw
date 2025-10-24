import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { useAuth } from './AuthContext';
import { message } from 'antd';

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
  const [notifications, setNotifications] = useState([]);
  const [onlineAgents, setOnlineAgents] = useState(0);
  const [totalAgents, setTotalAgents] = useState(0);
  const [subscribers, setSubscribers] = useState({});
  const [jobStatuses, setJobStatuses] = useState(new Map());
  const [jobHistory, setJobHistory] = useState([]);
  const [fileTransferLogs, setFileTransferLogs] = useState([]);
  const [reconnectAttempts, setReconnectAttempts] = useState(0);
  const [lastErrorTime, setLastErrorTime] = useState(0);
  const [connectionState, setConnectionState] = useState('disconnected');
  // Remove auth dependency for SyncTool integration
  const user = { id: 'synctool-user' };
  const token = 'synctool-token';
  
  // Use refs to avoid dependency issues
  const reconnectTimeoutRef = React.useRef(null);
  const socketRef = React.useRef(null);
  const initializationRef = React.useRef(false);

  const connectWebSocket = useCallback(() => {
    if (socketRef.current?.readyState === WebSocket.CONNECTING || socketRef.current?.readyState === WebSocket.OPEN) {
      return; // Prevent multiple connections
    }

    console.log('[WebSocket] Attempting to connect to SyncTool server...');
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    // Connect to SyncTool server on port 8090
    const wsUrl = `${protocol}//${window.location.hostname}:8090/ws/cli`;
    
    setConnectionState('connecting');  
    
    try {
      const ws = new WebSocket(wsUrl);
      socketRef.current = ws; // Set ref immediately to prevent duplicate connections
      
      ws.onopen = () => {
        console.log('[WebSocket] Connected successfully');
        setConnected(true);
        setSocket(ws);
        setConnectionState('connected');
        
        // Only show success message if this was a reconnection
        if (reconnectAttempts > 0) {
          message.success('Reconnected successfully', 2);
        }
        
        // Reset attempts on successful connection
        setReconnectAttempts(0);
        if (reconnectTimeoutRef.current) {
          clearTimeout(reconnectTimeoutRef.current);
          reconnectTimeoutRef.current = null;
        }
      };

      ws.onmessage = (event) => {
        try {
          const wsMessage = JSON.parse(event.data);
          handleWebSocketMessage(wsMessage);
        } catch (error) {
          console.error('[WebSocket] Failed to parse message:', error);
        }
      };

      ws.onclose = (event) => {
        console.log('[WebSocket] Disconnected:', event.code, event.reason);
        setConnected(false);
        setSocket(null);
        socketRef.current = null;
        setConnectionState('disconnected');
        
        // Temporarily disable auto-reconnect to debug the issue
        console.log('[WebSocket] Auto-reconnect disabled for debugging');
      };

      ws.onerror = (error) => {
        console.error('[WebSocket] Error:', error);
        setConnectionState('error');
        // Let onclose handle the error messaging
      };

    } catch (error) {
      console.error('[WebSocket] Failed to create connection:', error);
      setConnectionState('error');
      socketRef.current = null;
      
      const now = Date.now();
      if (now - lastErrorTime > 60000) { // 1 minute cooldown
        message.error('Failed to establish connection', 3);
        setLastErrorTime(now);
      }
    }
  }, []); // No dependencies needed for SyncTool integration

  useEffect(() => {
    if (!socket) {
      console.log('[WebSocket] Initializing SyncTool connection...');
      connectWebSocket();
    }
    
    // Cleanup on unmount
    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (socketRef.current) {
        socketRef.current.close();
      }
    };
  }, []); // No dependencies needed

  const handleWebSocketMessage = useCallback((wsMessage) => {
    console.log('WebSocket message received:', wsMessage.type);
    
    switch (wsMessage.type) {
      case 'connected':
        addNotification('info', wsMessage.data.message);
        break;
        
      case 'job_status_update':
        handleJobStatusUpdate(wsMessage.data);
        break;
        
      case 'agent_status':
        handleAgentStatusUpdate(wsMessage.data);
        break;
        
      case 'file_transfer':
        handleFileTransferUpdate(wsMessage.data);
        break;
        
      case 'file_transfer_log':
        handleFileTransferLogUpdate(wsMessage.data);
        break;
        
      default:
        console.log('Unknown WebSocket message type:', wsMessage.type);
    }
  }, []);

  const handleJobStatusUpdate = useCallback((statusData) => {
    // Update current job statuses
    setJobStatuses(prev => {
      const newMap = new Map(prev);
      newMap.set(statusData.job_id, {
        ...statusData,
        lastUpdate: new Date()
      });
      return newMap;
    });

    // Add to job history when status changes to completed states
    if (['success', 'failed'].includes(statusData.status)) {
      setJobHistory(prev => {
        const existing = prev.find(job => job.job_id === statusData.job_id);
        if (!existing || existing.status !== statusData.status) {
          const newEntry = {
            ...statusData,
            completedAt: new Date(),
            id: `${statusData.job_id}_${Date.now()}`
          };
          return [newEntry, ...prev.slice(0, 99)]; // Keep last 100
        }
        return prev;
      });
    }

    // Create notification
    const statusEmoji = {
      'running': '🔄',
      'success': '✅',
      'failed': '❌',
      'idle': '💤'
    };
    
    const message = `${statusEmoji[statusData.status]} ${statusData.job_name}: ${statusData.status}`;
    addNotification(statusData.status === 'failed' ? 'error' : 'success', message);

    // Real file transfer logs now come via separate 'file_transfer_log' messages
  }, []);

  const handleAgentStatusUpdate = useCallback((agentData) => {
    if (agentData.status === 'online') {
      setOnlineAgents(prev => prev + 1);
    } else if (agentData.status === 'offline') {
      setOnlineAgents(prev => Math.max(0, prev - 1));
    }
    
    addNotification('info', `Agent ${agentData.hostname}: ${agentData.status}`);
  }, []);

  const handleFileTransferUpdate = useCallback((fileData) => {
    setFileTransferLogs(prev => [
      {
        id: Date.now() + Math.random(),
        ...fileData,
        timestamp: new Date()
      },
      ...prev.slice(0, 199) // Keep last 200 logs
    ]);
  }, []);

  const handleFileTransferLogUpdate = useCallback((fileData) => {
    setFileTransferLogs(prev => [
      {
        id: fileData.id || Date.now() + Math.random(),
        ...fileData,
        timestamp: new Date(fileData.timestamp)
      },
      ...prev.slice(0, 199) // Keep last 200 logs
    ]);
  }, []);


  const addNotification = useCallback((type, message) => {
    const notification = {
      id: Date.now() + Math.random(),
      type,
      message,
      timestamp: new Date()
    };
    
    setNotifications(prev => [notification, ...prev.slice(0, 49)]); // Keep last 50
  }, []);

  const subscribe = (event, handler) => {
    setSubscribers(prev => ({
      ...prev,
      [event]: [...(prev[event] || []), handler],
    }));
  };

  const unsubscribe = (event, handler) => {
    setSubscribers(prev => ({
      ...prev,
      [event]: (prev[event] || []).filter(h => h !== handler),
    }));
  };

  const clearNotifications = () => {
    setNotifications([]);
  };

  const clearJobHistory = () => {
    setJobHistory([]);
  };

  const clearFileTransferLogs = () => {
    setFileTransferLogs([]);
  };

  const formatFileSize = (bytes) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  const getJobStats = () => {
    const currentJobs = Array.from(jobStatuses.values());
    return {
      total: currentJobs.length,
      running: currentJobs.filter(job => job.status === 'running').length,
      success: currentJobs.filter(job => job.status === 'success').length,
      failed: currentJobs.filter(job => job.status === 'failed').length,
      idle: currentJobs.filter(job => job.status === 'idle').length,
    };
  };

  const value = {
    // Connection state
    connected,
    socket,
    reconnectAttempts,
    connectionState,
    isReconnecting: connectionState === 'connecting',
    
    // Data
    notifications,
    onlineAgents,
    totalAgents,
    jobStatuses,
    jobHistory,
    fileTransferLogs,
    
    // Functions
    subscribe,
    unsubscribe,
    clearNotifications,
    clearJobHistory,
    clearFileTransferLogs,
    formatFileSize,
    getJobStats,
    connectWebSocket, // Expose for manual reconnection
    
    // Stats
    syncingFolders: Array.from(jobStatuses.values()).filter(job => job.status === 'running').length,
    activeFolders: jobStatuses.size,
  };

  return (
    <WebSocketContext.Provider value={value}>
      {children}
    </WebSocketContext.Provider>
  );
};