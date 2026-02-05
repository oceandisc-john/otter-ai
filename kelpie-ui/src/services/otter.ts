import axios from 'axios';

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

const api = axios.create({
  baseURL: API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

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
  // Chat
  async sendMessage(message: string): Promise<string> {
    const response = await api.post('/api/v1/chat', { message });
    return response.data.response;
  },

  // Memories
  async getMemories(type: string = 'long_term'): Promise<Memory[]> {
    const response = await api.get(`/api/v1/memories?type=${type}`);
    return response.data;
  },

  async createMemory(content: string, type: string, importance: number): Promise<string> {
    const response = await api.post('/api/v1/memories', { content, type, importance });
    return response.data.id;
  },

  async deleteMemory(id: string, type: string = 'long_term'): Promise<void> {
    await api.delete(`/api/v1/memories/${id}?type=${type}`);
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
