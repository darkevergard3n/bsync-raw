const API_BASE_URL = 'https://192.168.50.157:443';

class AuthService {
  // Login user
  async login(username, password) {
    try {
      const response = await fetch(`${API_BASE_URL}/api/v1/auth/login`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ username, password }),
      });

      const data = await response.json();

      // Handle different response structures
      // New API structure: data.data.access_token
      let tokenValue = data.data?.access_token || data.token || data.access_token;
      let userData = data.data?.user || data.user;

      // Check if response is successful
      if (data.success || (response.ok && tokenValue)) {
        // Save token and user data to localStorage
        if (tokenValue) {
          localStorage.setItem('token', tokenValue);
        }

        if (userData) {
          localStorage.setItem('user', JSON.stringify(userData));
        }

        return { success: true, data: { token: tokenValue, user: userData } };
      } else {
        return { success: false, error: data.error || 'Login failed' };
      }
    } catch (error) {
      console.error('Login error:', error);
      return { success: false, error: 'Network error. Please check your connection.' };
    }
  }

  // Logout user
  async logout() {
    try {
      const token = this.getToken();
      if (token) {
        await fetch(`${API_BASE_URL}/api/v1/auth/logout`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${token}`,
          },
        });
      }
    } catch (error) {
      console.error('Logout error:', error);
    } finally {
      // Always clear local storage
      localStorage.removeItem('token');
      localStorage.removeItem('user');
    }
  }

  // Get current token
  getToken() {
    return localStorage.getItem('token');
  }

  // Get current user
  getCurrentUser() {
    const userStr = localStorage.getItem('user');
    if (!userStr || userStr === 'undefined' || userStr === 'null') {
      return null;
    }
    try {
      return JSON.parse(userStr);
    } catch (error) {
      console.error('Error parsing user data:', error);
      return null;
    }
  }

  // Check if user is authenticated
  isAuthenticated() {
    return !!this.getToken();
  }

  // Check if user is admin
  isAdmin() {
    const user = this.getCurrentUser();
    return user && user.role === 'admin';
  }

  // Check if user is operator
  isOperator() {
    const user = this.getCurrentUser();
    return user && user.role === 'operator';
  }

  // Get assigned agents (for operators)
  getAssignedAgents() {
    const user = this.getCurrentUser();
    return user ? user.assigned_agents || [] : [];
  }

  // Fetch with authentication
  async fetchWithAuth(url, options = {}) {
    const token = this.getToken();

    if (!token) {
      throw new Error('No authentication token found');
    }

    const response = await fetch(url, {
      ...options,
      headers: {
        ...options.headers,
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
    });

    // Handle 401 - redirect to login
    if (response.status === 401) {
      this.logout();
      window.location.href = '/login';
      throw new Error('Session expired. Please login again.');
    }

    // Handle 403 - permission denied
    if (response.status === 403) {
      const data = await response.json();
      throw new Error(data.error || 'Access denied');
    }

    return response;
  }

  // Get with authentication
  async get(url) {
    const response = await this.fetchWithAuth(url, {
      method: 'GET',
    });
    return response.json();
  }

  // Post with authentication
  async post(url, data) {
    const response = await this.fetchWithAuth(url, {
      method: 'POST',
      body: JSON.stringify(data),
    });
    return response.json();
  }

  // Put with authentication
  async put(url, data) {
    const response = await this.fetchWithAuth(url, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
    return response.json();
  }

  // Delete with authentication
  async delete(url) {
    const response = await this.fetchWithAuth(url, {
      method: 'DELETE',
    });
    return response.json();
  }
}

const authService = new AuthService();
export default authService;
