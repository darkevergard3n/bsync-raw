import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { Eye, EyeOff } from 'lucide-react';

const Login = () => {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const { login: contextLogin, isAuthenticated } = useAuth();

  // Redirect if already logged in
  useEffect(() => {
    if (isAuthenticated) {
      navigate('/dashboard');
    }
  }, [isAuthenticated, navigate]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    // Validation
    if (!email || !password) {
      setError('Email and password are required');
      setLoading(false);
      return;
    }

    try {
      // Use email as username for login
      const result = await contextLogin({ username: email, password });

      if (!result.success) {
        setError(result.message);
      }
      // If success, navigation will happen via useEffect when isAuthenticated changes
    } catch (err) {
      setError('An error occurred. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{
      minHeight: '100vh',
      background: 'linear-gradient(135deg, #1a2332 0%, #2d4a4f 100%)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      padding: '20px',
      fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'
    }}>
      {/* Logo */}
      <div style={{
        position: 'absolute',
        top: '40px',
        left: '40px'
      }}>
        <img
          src="/bsync-logo-utama.png"
          alt="BSync Logo"
          style={{ height: '40px', width: 'auto' }}
        />
      </div>

      {/* Login Card */}
      <div style={{
        background: 'rgba(30, 45, 56, 0.95)',
        borderRadius: '16px',
        padding: '48px',
        width: '100%',
        maxWidth: '480px',
        boxShadow: '0 20px 60px rgba(0, 0, 0, 0.3)',
        backdropFilter: 'blur(10px)'
      }}>
        {/* Header */}
        <div style={{ textAlign: 'center', marginBottom: '40px' }}>
          <h1 style={{
            fontSize: '32px',
            fontWeight: 700,
            color: '#ffffff',
            margin: '0 0 12px 0'
          }}>
            Welcome Back!
          </h1>
          <p style={{
            fontSize: '16px',
            color: '#94a3b8',
            margin: 0
          }}>
            Manage servers, track sync jobs, and stay in control
          </p>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit}>
          {/* Email Field */}
          <div style={{ marginBottom: '24px' }}>
            <label style={{
              display: 'block',
              fontSize: '14px',
              fontWeight: 500,
              color: '#ffffff',
              marginBottom: '8px'
            }}>
              Email
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="youremail@mail.com"
              disabled={loading}
              autoFocus
              style={{
                width: '100%',
                height: '48px',
                padding: '0 16px',
                fontSize: '15px',
                color: '#ffffff',
                background: 'rgba(51, 65, 85, 0.6)',
                border: '1px solid rgba(148, 163, 184, 0.2)',
                borderRadius: '8px',
                outline: 'none',
                transition: 'all 0.3s ease',
                boxSizing: 'border-box'
              }}
              onFocus={(e) => {
                e.target.style.borderColor = '#4ade80';
                e.target.style.background = 'rgba(51, 65, 85, 0.8)';
              }}
              onBlur={(e) => {
                e.target.style.borderColor = 'rgba(148, 163, 184, 0.2)';
                e.target.style.background = 'rgba(51, 65, 85, 0.6)';
              }}
            />
          </div>

          {/* Password Field */}
          <div style={{ marginBottom: '8px' }}>
            <div style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginBottom: '8px'
            }}>
              <label style={{
                fontSize: '14px',
                fontWeight: 500,
                color: '#ffffff'
              }}>
                Password
              </label>
              <a href="#" style={{
                fontSize: '14px',
                color: '#4ade80',
                textDecoration: 'none'
              }}>
                Forgot Password?
              </a>
            </div>
            <div style={{ position: 'relative' }}>
              <input
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter your password here"
                disabled={loading}
                style={{
                  width: '100%',
                  height: '48px',
                  padding: '0 48px 0 16px',
                  fontSize: '15px',
                  color: '#ffffff',
                  background: 'rgba(51, 65, 85, 0.6)',
                  border: '1px solid rgba(148, 163, 184, 0.2)',
                  borderRadius: '8px',
                  outline: 'none',
                  transition: 'all 0.3s ease',
                  boxSizing: 'border-box'
                }}
                onFocus={(e) => {
                  e.target.style.borderColor = '#4ade80';
                  e.target.style.background = 'rgba(51, 65, 85, 0.8)';
                }}
                onBlur={(e) => {
                  e.target.style.borderColor = 'rgba(148, 163, 184, 0.2)';
                  e.target.style.background = 'rgba(51, 65, 85, 0.6)';
                }}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                style={{
                  position: 'absolute',
                  right: '12px',
                  top: '50%',
                  transform: 'translateY(-50%)',
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  padding: '8px',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center'
                }}
              >
                {showPassword ? (
                  <EyeOff size={20} color="#94a3b8" />
                ) : (
                  <Eye size={20} color="#94a3b8" />
                )}
              </button>
            </div>
          </div>

          {/* Error Message */}
          {error && (
            <div style={{
              padding: '12px 16px',
              marginBottom: '24px',
              background: 'rgba(239, 68, 68, 0.1)',
              border: '1px solid rgba(239, 68, 68, 0.3)',
              borderRadius: '8px',
              color: '#fca5a5',
              fontSize: '14px'
            }}>
              {error}
            </div>
          )}

          {/* Submit Button */}
          <button
            type="submit"
            disabled={loading}
            style={{
              width: '100%',
              height: '48px',
              marginTop: '24px',
              fontSize: '16px',
              fontWeight: 600,
              color: '#ffffff',
              background: loading ? '#6b7280' : '#4ade80',
              border: 'none',
              borderRadius: '8px',
              cursor: loading ? 'not-allowed' : 'pointer',
              transition: 'all 0.3s ease',
              boxShadow: loading ? 'none' : '0 4px 12px rgba(74, 222, 128, 0.3)'
            }}
            onMouseEnter={(e) => {
              if (!loading) {
                e.target.style.background = '#22c55e';
                e.target.style.transform = 'translateY(-1px)';
                e.target.style.boxShadow = '0 6px 20px rgba(74, 222, 128, 0.4)';
              }
            }}
            onMouseLeave={(e) => {
              if (!loading) {
                e.target.style.background = '#4ade80';
                e.target.style.transform = 'translateY(0)';
                e.target.style.boxShadow = '0 4px 12px rgba(74, 222, 128, 0.3)';
              }
            }}
          >
            {loading ? 'Signing In...' : 'Sign In'}
          </button>
        </form>

        {/* Demo Credentials */}
        <div style={{
          marginTop: '32px',
          padding: '16px',
          background: 'rgba(51, 65, 85, 0.4)',
          borderRadius: '8px',
          textAlign: 'center'
        }}>
          <p style={{
            fontSize: '13px',
            color: '#94a3b8',
            margin: '0 0 8px 0'
          }}>
            Demo credentials:
          </p>
          <p style={{
            fontSize: '14px',
            color: '#cbd5e1',
            margin: '4px 0',
            fontFamily: 'monospace'
          }}>
            admin@datasync.com / admin123
          </p>
          <p style={{
            fontSize: '14px',
            color: '#cbd5e1',
            margin: '4px 0',
            fontFamily: 'monospace'
          }}>
            operator@datasync.com / operator123
          </p>
        </div>
      </div>
    </div>
  );
};

export default Login;
