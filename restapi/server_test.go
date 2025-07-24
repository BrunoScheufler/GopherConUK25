package restapi

import (
	"context"
	"testing"

	"github.com/brunoscheufler/gopherconuk25/proxy"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing
type mockAccountStore struct{}
func (m *mockAccountStore) ListAccounts(ctx context.Context) ([]store.Account, error) { return nil, nil }
func (m *mockAccountStore) CreateAccount(ctx context.Context, account store.Account) error { return nil }
func (m *mockAccountStore) UpdateAccount(ctx context.Context, account store.Account) error { return nil }
func (m *mockAccountStore) HealthCheck(ctx context.Context) error { return nil }
func (m *mockAccountStore) Close() error { return nil }

type mockNoteStore struct{}
func (m *mockNoteStore) ListNotes(ctx context.Context, accountID uuid.UUID) ([]store.Note, error) { return nil, nil }
func (m *mockNoteStore) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) { return nil, nil }
func (m *mockNoteStore) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error { return nil }
func (m *mockNoteStore) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error { return nil }
func (m *mockNoteStore) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error { return nil }
func (m *mockNoteStore) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) { return 0, nil }
func (m *mockNoteStore) GetTotalNotes(ctx context.Context) (int, error) { return 0, nil }
func (m *mockNoteStore) HealthCheck(ctx context.Context) error { return nil }
func (m *mockNoteStore) Close() error { return nil }

func TestNewServer_Options(t *testing.T) {
	mockAccountStore := &mockAccountStore{}
	mockNoteStore := &mockNoteStore{}
	mockTelemetry := telemetry.New()
	defer mockTelemetry.StatsCollector.Stop()
	mockDeployment := proxy.NewDeploymentController(mockTelemetry)
	defer mockDeployment.Close()
	
	// Test with individual options
	server := NewServer(
		WithAccountStore(mockAccountStore),
		WithNoteStore(mockNoteStore),
		WithTelemetry(mockTelemetry),
		WithDeploymentController(mockDeployment),
	)
	
	require.NotNil(t, server, "Server should be created")
	require.Equal(t, mockAccountStore, server.accountStore, "Account store should be set")
	require.Equal(t, mockNoteStore, server.noteStore, "Note store should be set")
	require.Equal(t, mockTelemetry, server.telemetry, "Telemetry should be set")
	require.Equal(t, mockDeployment, server.deploymentController, "Deployment controller should be set")
	require.NotNil(t, server.logger, "Logger should be set from telemetry")
	
	// Test with AppConfig option
	appConfig := &AppConfig{
		AccountStore:         mockAccountStore,
		NoteStore:            mockNoteStore,
		DeploymentController: mockDeployment,
		Telemetry:            mockTelemetry,
	}
	
	configServer := NewServer(WithAppConfig(appConfig))
	
	require.NotNil(t, configServer, "Config server should be created")
	require.Equal(t, mockAccountStore, configServer.accountStore, "Config account store should be set")
	require.Equal(t, mockNoteStore, configServer.noteStore, "Config note store should be set")
	
	// Test backward compatibility with NewServerFromConfig
	compatServer := NewServerFromConfig(appConfig)
	
	require.NotNil(t, compatServer, "Compat server should be created")
	require.Equal(t, mockAccountStore, compatServer.accountStore, "Compat account store should be set")
}