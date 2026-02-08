import React, { useState } from 'react';
import { otterService } from '../services/otter';
import './Login.css';

interface LoginProps {
  onLogin: () => void;
}

const Login: React.FC<LoginProps> = ({ onLogin }) => {
  const [passphrase, setPassphrase] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const success = await otterService.authenticate(passphrase);
      if (success) {
        onLogin();
      } else {
        setError('Invalid passphrase. Please try again.');
        setPassphrase('');
      }
    } catch (err) {
      setError('Authentication failed. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-container">
      <div className="login-box">
        <div className="login-logo">
          <h1>🦦 Otter-AI</h1>
          <p>Governed AI Agent System</p>
        </div>
        <form onSubmit={handleSubmit} className="login-form">
          <div className="form-group">
            <label htmlFor="passphrase">Passphrase</label>
            <input
              id="passphrase"
              type="password"
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
              placeholder="Enter your passphrase"
              disabled={loading}
              autoFocus
            />
          </div>
          {error && <div className="error-message">{error}</div>}
          <button type="submit" disabled={loading || !passphrase}>
            {loading ? 'Authenticating...' : 'Login'}
          </button>
        </form>
      </div>
    </div>
  );
};

export default Login;
