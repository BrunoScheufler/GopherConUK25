package cli

import (
	"github.com/brunoscheufler/gopherconuk25/proxy"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	tea "github.com/charmbracelet/bubbletea"
)

// AppConfig groups common application dependencies to reduce parameter lists
type AppConfig struct {
	AccountStore         store.AccountStore
	NoteStore            store.NoteStore
	DeploymentController *proxy.DeploymentController
	Telemetry            *telemetry.Telemetry
}

type CLIOptions struct {
	Theme string
}

// RunCLI starts the CLI application with the given stores, telemetry, and options
func RunCLI(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, deploymentController *proxy.DeploymentController, options CLIOptions) error {
	appConfig := &AppConfig{
		AccountStore:         accountStore,
		NoteStore:            noteStore,
		DeploymentController: deploymentController,
		Telemetry:            tel,
	}
	
	// Create new bubbletea model
	model := NewBubbleTeaModel(appConfig, options)
	
	// Start the bubbletea program
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}
