import React, { useState, useEffect } from 'react';
import { useNavigate, useParams, Link } from 'react-router-dom';
import { apiClient } from '../api';
import { Note, Account } from '../types';

const NoteForm: React.FC = () => {
  const navigate = useNavigate();
  const { accountId, noteId } = useParams<{ accountId: string; noteId?: string }>();
  const isEditing = !!noteId;

  const [content, setContent] = useState('');
  const [account, setAccount] = useState<Account | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (accountId) {
      loadAccount(accountId);
    }
    if (isEditing && accountId && noteId) {
      loadNote(accountId, noteId);
    }
  }, [isEditing, accountId, noteId]);

  const loadAccount = async (id: string) => {
    try {
      const accountData = await apiClient.getAccount(id);
      setAccount(accountData);
    } catch (err) {
      setError('Failed to load account');
      console.error('Error loading account:', err);
    }
  };

  const loadNote = async (accountId: string, noteId: string) => {
    try {
      setLoading(true);
      const note = await apiClient.getNote(accountId, noteId);
      setContent(note.content);
      setError('');
    } catch (err) {
      setError('Failed to load note');
      console.error('Error loading note:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!accountId) {
      setError('Account ID is required');
      return;
    }

    if (!content.trim()) {
      setError('Note content is required');
      return;
    }

    try {
      setLoading(true);
      setError('');

      if (isEditing && noteId) {
        await apiClient.updateNote(accountId, noteId, { content: content.trim() });
      } else {
        await apiClient.createNote(accountId, { content: content.trim() });
      }

      navigate(`/accounts/${accountId}/notes`);
    } catch (err) {
      setError(`Failed to ${isEditing ? 'update' : 'create'} note`);
      console.error(`Error ${isEditing ? 'updating' : 'creating'} note:`, err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="note-form">
      <div className="page-header">
        <div>
          <h1>{isEditing ? 'Edit Note' : 'New Note'}</h1>
          <p className="account-info">
            Account: {account?.name} ({accountId})
          </p>
        </div>
        <Link 
          to={`/accounts/${accountId}/notes`} 
          className="btn btn-secondary"
        >
          Back to Notes
        </Link>
      </div>

      {error && (
        <div className="error-message">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="form">
        <div className="form-group">
          <label htmlFor="content" className="form-label">
            Note Content
          </label>
          <textarea
            id="content"
            value={content}
            onChange={(e) => setContent(e.target.value)}
            className="form-textarea"
            placeholder="Enter your note content..."
            disabled={loading}
            rows={10}
            maxLength={10000}
          />
          <div className="form-helper">
            {content.length}/10,000 characters
          </div>
        </div>

        <div className="form-actions">
          <button
            type="submit"
            disabled={loading || !content.trim()}
            className="btn btn-primary"
          >
            {loading ? 'Saving...' : (isEditing ? 'Update Note' : 'Create Note')}
          </button>
          <Link 
            to={`/accounts/${accountId}/notes`} 
            className="btn btn-secondary"
          >
            Cancel
          </Link>
        </div>
      </form>
    </div>
  );
};

export default NoteForm;