# Notely UI

A React frontend for the Notely application - a demo app for learning data migration strategies in Go.

## Features

- **Account Management**: Create, view, and edit user accounts
- **Note Management**: CRUD operations for notes within accounts
- **Migration Visualization**: Display account migration status and shard information
- **Deployment Control**: Trigger rolling deployments of data proxy instances
- **Responsive Design**: Works on desktop and mobile devices

## API Integration

The UI communicates with the Go backend API running on port 8080, with endpoints for:

- Account operations (`/accounts/*`)
- Note operations (`/accounts/{accountId}/notes/*`)
- Health checks (`/healthz`)
- Deployment triggers (`/deploy`)

## Getting Started

1. **Install dependencies:**
   ```bash
   cd ui
   npm install
   ```

2. **Start the development server:**
   ```bash
   npm run dev
   ```

3. **Make sure the Go backend is running:**
   ```bash
   cd ..
   go run . --port=8080
   ```

The UI will be available at `http://localhost:3001` and will proxy API requests to the Go server at `http://localhost:8080`.

## Project Structure

```
src/
├── components/          # React components
│   ├── AccountList.tsx  # Account listing with migration status
│   ├── AccountForm.tsx  # Account creation/editing form
│   ├── NoteList.tsx     # Note listing for an account
│   └── NoteForm.tsx     # Note creation/editing form
├── api.ts              # API client with typed methods
├── types.ts            # TypeScript type definitions
├── App.tsx             # Main app with routing
├── main.tsx            # React entry point
└── index.css           # Global styles
```

## Key Components

### AccountList
- Lists all accounts with migration status indicators
- Shows shard information where applicable
- Provides navigation to notes and editing
- Includes deployment trigger button

### NoteList
- Displays notes for a specific account
- Shows creation and update timestamps
- Provides CRUD operations for notes
- Handles empty states gracefully

### Forms
- Validation for required fields
- Loading states during API calls
- Error handling with user feedback
- Character limits matching backend validation

## Development

- Built with Vite for fast development
- TypeScript for type safety
- React Router for client-side routing
- Axios for API communication
- CSS modules for styling

## Production Build

```bash
npm run build
```

This creates optimized production files in the `dist/` directory.