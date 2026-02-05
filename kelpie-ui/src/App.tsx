import React from 'react';
import Chat from './components/Chat';
import './App.css';

const App: React.FC = () => {
  return (
    <div className="app">
      <header className="app-header">
        <h1>🦦 Kelpie</h1>
        <p className="subtitle">Governed AI Agent Interface</p>
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
