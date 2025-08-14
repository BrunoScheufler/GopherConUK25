import React, { useState, useEffect } from 'react';
import { useNavigate, useParams, Link } from 'react-router-dom';
import { apiClient } from '../api';
import { Account } from '../types';

const AccountForm: React.FC = () => {
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const isEditing = !!id;

  const [name, setName] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (isEditing && id) {
      loadAccount(id);
    }
  }, [isEditing, id]);

  const loadAccount = async (accountId: string) => {
    try {
      setLoading(true);
      const account = await apiClient.getAccount(accountId);
      setName(account.name);
      setError('');
    } catch (err) {
      setError('Failed to load account');
      console.error('Error loading account:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!name.trim()) {
      setError('Account name is required');
      return;
    }

    try {
      setLoading(true);
      setError('');

      if (isEditing && id) {
        await apiClient.updateAccount(id, { name: name.trim() });
      } else {
        await apiClient.createAccount({ name: name.trim() });
      }

      navigate('/');
    } catch (err) {
      setError(`Failed to ${isEditing ? 'update' : 'create'} account`);
      console.error(`Error ${isEditing ? 'updating' : 'creating'} account:`, err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="account-form">
      <div className="page-header">
        <h1>{isEditing ? 'Edit Account' : 'New Account'}</h1>
        <Link to="/" className="btn btn-secondary">
          Back to Accounts
        </Link>
      </div>

      {error && (
        <div className="error-message">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="form">
        <div className="form-group">
          <label htmlFor="name" className="form-label">
            Account Name
          </label>
          <input
            type="text"
            id="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="form-input"
            placeholder="Enter account name"
            disabled={loading}
            maxLength={100}
          />
        </div>

        <div className="form-actions">
          <button
            type="submit"
            disabled={loading || !name.trim()}
            className="btn btn-primary"
          >
            {loading ? 'Saving...' : (isEditing ? 'Update Account' : 'Create Account')}
          </button>
          <Link to="/" className="btn btn-secondary">
            Cancel
          </Link>
        </div>
      </form>
    </div>
  );
};

export default AccountForm;