import axios, { AxiosResponse } from 'axios';
import { Account, Note, CreateAccountRequest, UpdateAccountRequest, CreateNoteRequest, UpdateNoteRequest } from './types';

const api = axios.create({
  baseURL: 'http://localhost:8080',
  headers: {
    'Content-Type': 'application/json',
  },
});

export class ApiClient {
  async getAccounts(): Promise<Account[]> {
    const response: AxiosResponse<Account[]> = await api.get('/accounts');
    return response.data;
  }

  async getAccount(id: string): Promise<Account> {
    const response: AxiosResponse<Account> = await api.get(`/accounts/${id}`);
    return response.data;
  }

  async createAccount(data: CreateAccountRequest): Promise<Account> {
    const response: AxiosResponse<Account> = await api.post('/accounts', data);
    return response.data;
  }

  async updateAccount(id: string, data: UpdateAccountRequest): Promise<Account> {
    const response: AxiosResponse<Account> = await api.put(`/accounts/${id}`, data);
    return response.data;
  }

  async getNotes(accountId: string): Promise<string[]> {
    const response: AxiosResponse<string[]> = await api.get(`/accounts/${accountId}/notes`);
    return response.data;
  }

  async getNote(accountId: string, noteId: string): Promise<Note> {
    const response: AxiosResponse<Note> = await api.get(`/accounts/${accountId}/notes/${noteId}`);
    return response.data;
  }

  async createNote(accountId: string, data: CreateNoteRequest): Promise<Note> {
    const response: AxiosResponse<Note> = await api.post(`/accounts/${accountId}/notes`, data);
    return response.data;
  }

  async updateNote(accountId: string, noteId: string, data: UpdateNoteRequest): Promise<Note> {
    const response: AxiosResponse<Note> = await api.put(`/accounts/${accountId}/notes/${noteId}`, data);
    return response.data;
  }

  async deleteNote(accountId: string, noteId: string): Promise<void> {
    await api.delete(`/accounts/${accountId}/notes/${noteId}`);
  }

  async triggerDeploy(): Promise<void> {
    await api.post('/deploy');
  }

  async healthCheck(): Promise<{ status: string; timestamp: string }> {
    const response = await api.get('/healthz');
    return response.data;
  }
}

export const apiClient = new ApiClient();