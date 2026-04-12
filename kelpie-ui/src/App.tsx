import React, { useState, useEffect } from 'react';
import Chat from './components/Chat';
import Login from './components/Login';
import { otterService } from './services/otter';
import './App.css';

const App: React.FC = () => {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let isMounted = true;

    const initAuth = async () => {
      if (otterService.isAuthenticated()) {
        if (isMounted) {
          setIsAuthenticated(true);
          setIsLoading(false);
        }
        return;
      }

      const ok = await otterService.bootstrapAuth();
      if (isMounted) {
        setIsAuthenticated(ok);
        setIsLoading(false);
      }
    };

    initAuth();

    return () => {
      isMounted = false;
    };
  }, []);

  const handleLogin = () => {
    setIsAuthenticated(true);
  };

  const handleLogout = () => {
    otterService.logout();
    setIsAuthenticated(false);
  };

  if (isLoading) {
    return (
      <div className="app loading">
        <div className="loader">Loading...</div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Login onLogin={handleLogin} />;
  }

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-content">
          <div>
            <h1>🦦 Kelpie</h1>
            <p className="subtitle">Governed AI Agent Interface</p>
          </div>
          <button className="logout-button" onClick={handleLogout}>
            Logout
          </button>
        </div>
      </header>
      <main className="app-main">
        <Chat />
      </main>
      <footer className="app-footer">
        <p>Otter-AI - Governed, Local-First AI Agent System</p>
      </footer>
    </div>
  );
};

export default App;
