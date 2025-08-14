import React from 'react';
import { BrowserRouter as Router, Routes, Route, Link } from 'react-router-dom';
import AccountList from './components/AccountList';
import AccountForm from './components/AccountForm';
import NoteList from './components/NoteList';
import NoteForm from './components/NoteForm';

function App() {
  return (
    <Router>
      <div className="app">
        <nav className="nav">
          <div className="nav-brand">
            <Link to="/" className="nav-title">Notely</Link>
          </div>
          <div className="nav-links">
            <Link to="/" className="nav-link">Accounts</Link>
          </div>
        </nav>

        <main className="main">
          <Routes>
            <Route path="/" element={<AccountList />} />
            <Route path="/accounts/new" element={<AccountForm />} />
            <Route path="/accounts/:id/edit" element={<AccountForm />} />
            <Route path="/accounts/:accountId/notes" element={<NoteList />} />
            <Route path="/accounts/:accountId/notes/new" element={<NoteForm />} />
            <Route path="/accounts/:accountId/notes/:noteId/edit" element={<NoteForm />} />
          </Routes>
        </main>
      </div>
    </Router>
  );
}

export default App;