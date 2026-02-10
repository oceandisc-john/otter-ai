import axios from 'axios';

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

const api = axios.create({
  baseURL: API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Storage key for authentication token
const AUTH_TOKEN_KEY = 'otter_auth_token';

// Add authentication interceptor
api.interceptors.request.use((config) => {
  const token = localStorage.getItem(AUTH_TOKEN_KEY);
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Handle unauthorized responses
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Clear invalid token and redirect to login
      localStorage.removeItem(AUTH_TOKEN_KEY);
      window.location.reload();
    }
    return Promise.reject(error);
  }
);

export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
}

export interface Memory {
  id: string;
  type: string;
  content: string;
  timestamp: number;
  importance: number;
}

export interface Rule {
  rule_id: string;
  scope: string;
  body: string;
  version: number;
  timestamp: string;
  adopted_at?: string;
}

export interface Proposal {
  proposal_id: string;
  rule: Rule;
  proposed_by: string;
  status: string;
  result: string;
  quorum_met: boolean;
}

export const otterService = {
  // Authentication
  async authenticate(passphrase: string): Promise<boolean> {
    try {
      const response = await api.post('/api/v1/auth', { passphrase });
      if (response.data.authenticated) {
        // Store JWT token instead of passphrase
        const token = response.data.token || passphrase; // Fallback for backward compatibility
        localStorage.setItem(AUTH_TOKEN_KEY, token);
        return true;
      }
      return false;
    } catch {
      return false;
    }
  },

  isAuthenticated(): boolean {
    return !!localStorage.getItem(AUTH_TOKEN_KEY);
  },

  logout(): void {
    localStorage.removeItem(AUTH_TOKEN_KEY);
  },

  // Chat
  async sendMessage(message: string): Promise<string> {
    const response = await api.post('/api/v1/chat', { message });
    return response.data.response;
  },

  async clearConversation(): Promise<void> {
    await api.post('/api/v1/chat/clear');
  },

  // Memories
  async getMemories(type: string = 'long_term'): Promise<Memory[]> {
    const response = await api.get(`/api/v1/memories?type=${type}`);
    return response.data;
  },

  // Governance
  async getRules(): Promise<Record<string, Rule>> {
    const response = await api.get('/api/v1/governance/rules');
    return response.data;
  },

  async proposeRule(scope: string, body: string, proposedBy: string, baseRuleId?: string): Promise<Proposal> {
    const response = await api.post('/api/v1/governance/rules', {
      scope,
      body,
      proposed_by: proposedBy,
      base_rule_id: baseRuleId,
    });
    return response.data;
  },

  async vote(proposalId: string, voterId: string, vote: 'YES' | 'NO' | 'ABSTAIN'): Promise<void> {
    await api.post('/api/v1/governance/vote', {
      proposal_id: proposalId,
      voter_id: voterId,
      vote,
    });
  },

  async getMembers(): Promise<any[]> {
    const response = await api.get('/api/v1/governance/members');
    return response.data;
  },

  // Health check
  async healthCheck(): Promise<boolean> {
    try {
      await api.get('/health');
      return true;
    } catch {
      return false;
    }
  },
};

export default otterService;
