export interface Account {
  id: string;
  name: string;
  isMigrating: boolean;
  shard?: string;
}

export interface Note {
  id: string;
  creator: string;
  createdAt: string;
  updatedAt: string;
  content: string;
}

export interface ErrorResponse {
  error: string;
}

export interface CreateAccountRequest {
  name: string;
}

export interface UpdateAccountRequest {
  name: string;
}

export interface CreateNoteRequest {
  content: string;
}

export interface UpdateNoteRequest {
  content: string;
}