import React, { useState, useEffect } from 'react';
import { Link, useParams } from 'react-router-dom';
import { apiClient } from '../api';
import { Note, Account } from '../types';

const NoteList: React.FC = () => {
  const { accountId } = useParams<{ accountId: string }>();
  const [notes, setNotes] = useState<Note[]>([]);
  const [account, setAccount] = useState<Account | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (accountId) {
      loadAccountAndNotes(accountId);
    }
  }, [accountId]);

  const loadAccountAndNotes = async (id: string) => {
    try {
      setLoading(true);
      const [accountData, noteIds] = await Promise.all([
        apiClient.getAccount(id),
        apiClient.getNotes(id)
      ]);
      
      setAccount(accountData);

      if (noteIds.length > 0) {
        const notePromises = noteIds.map(noteId => 
          apiClient.getNote(id, noteId)
        );
        const notesData = await Promise.all(notePromises);
        setNotes(notesData.sort((a, b) => 
          new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
        ));
      } else {
        setNotes([]);
      }
      
      setError('');
    } catch (err) {
      setError('Failed to load notes');
      console.error('Error loading notes:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteNote = async (noteId: string) => {
    if (!accountId) return;
    
    if (window.confirm('Are you sure you want to delete this note?')) {
      try {
        await apiClient.deleteNote(accountId, noteId);
        setNotes(notes.filter(note => note.id !== noteId));
      } catch (err) {
        alert('Failed to delete note');
        console.error('Error deleting note:', err);
      }
    }
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  if (loading) return <div className="loading">Loading notes...</div>;

  return (
    <div className="note-list">
      <div className="page-header">
        <div>
          <h1>Notes for {account?.name}</h1>
          <p className="account-info">
            Account ID: {accountId}
            {account?.isMigrating && (
              <span className="migration-status">• Migrating</span>
            )}
            {account?.shard && (
              <span className="shard-info">• Shard: {account.shard}</span>
            )}
          </p>
        </div>
        <div className="page-actions">
          <Link to="/" className="btn btn-secondary">
            Back to Accounts
          </Link>
          <Link 
            to={`/accounts/${accountId}/notes/new`} 
            className="btn btn-primary"
          >
            New Note
          </Link>
        </div>
      </div>

      {error && (
        <div className="error-message">
          {error}
          <button 
            onClick={() => accountId && loadAccountAndNotes(accountId)} 
            className="btn btn-link"
          >
            Retry
          </button>
        </div>
      )}

      <div className="notes-grid">
        {notes.map((note) => (
          <div key={note.id} className="note-card">
            <div className="note-content">
              <p className="note-text">
                {note.content.length > 200 
                  ? `${note.content.substring(0, 200)}...` 
                  : note.content
                }
              </p>
            </div>
            <div className="note-meta">
              <div className="note-dates">
                <small>Created: {formatDate(note.createdAt)}</small>
                {note.updatedAt !== note.createdAt && (
                  <small>Updated: {formatDate(note.updatedAt)}</small>
                )}
              </div>
              <div className="note-actions">
                <Link 
                  to={`/accounts/${accountId}/notes/${note.id}/edit`}
                  className="btn btn-secondary btn-sm"
                >
                  Edit
                </Link>
                <button
                  onClick={() => handleDeleteNote(note.id)}
                  className="btn btn-danger btn-sm"
                >
                  Delete
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>

      {notes.length === 0 && !loading && (
        <div className="empty-state">
          <p>No notes found for this account.</p>
          <Link 
            to={`/accounts/${accountId}/notes/new`} 
            className="btn btn-primary"
          >
            Create your first note
          </Link>
        </div>
      )}
    </div>
  );
};

export default NoteList;