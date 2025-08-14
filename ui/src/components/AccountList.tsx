import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { apiClient } from '../api';
import { Account } from '../types';

const AccountList: React.FC = () => {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>('');

  useEffect(() => {
    loadAccounts();
  }, []);

  const loadAccounts = async () => {
    try {
      setLoading(true);
      const data = await apiClient.getAccounts();
      setAccounts(data);
      setError('');
    } catch (err) {
      setError('Failed to load accounts');
      console.error('Error loading accounts:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleTriggerDeploy = async () => {
    try {
      await apiClient.triggerDeploy();
      alert('Deployment triggered successfully');
    } catch (err) {
      alert('Failed to trigger deployment');
      console.error('Error triggering deployment:', err);
    }
  };

  if (loading) return <div className="loading">Loading accounts...</div>;

  return (
    <div className="account-list">
      <div className="page-header">
        <h1>Accounts</h1>
        <div className="page-actions">
          <button 
            onClick={handleTriggerDeploy}
            className="btn btn-secondary"
          >
            Trigger Deploy
          </button>
          <Link to="/accounts/new" className="btn btn-primary">
            New Account
          </Link>
        </div>
      </div>

      {error && (
        <div className="error-message">
          {error}
          <button onClick={loadAccounts} className="btn btn-link">
            Retry
          </button>
        </div>
      )}

      <div className="accounts-grid">
        {accounts.map((account) => (
          <div key={account.id} className="account-card">
            <div className="account-info">
              <h3 className="account-name">{account.name}</h3>
              <div className="account-meta">
                <span className="account-id">ID: {account.id}</span>
                {account.isMigrating && (
                  <span className="migration-status">Migrating</span>
                )}
                {account.shard && (
                  <span className="shard-info">Shard: {account.shard}</span>
                )}
              </div>
            </div>
            <div className="account-actions">
              <Link 
                to={`/accounts/${account.id}/notes`}
                className="btn btn-primary btn-sm"
              >
                View Notes
              </Link>
              <Link 
                to={`/accounts/${account.id}/edit`}
                className="btn btn-secondary btn-sm"
              >
                Edit
              </Link>
            </div>
          </div>
        ))}
      </div>

      {accounts.length === 0 && !loading && (
        <div className="empty-state">
          <p>No accounts found.</p>
          <Link to="/accounts/new" className="btn btn-primary">
            Create your first account
          </Link>
        </div>
      )}
    </div>
  );
};

export default AccountList;