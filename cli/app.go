package cli

import (
	"github.com/brunoscheufler/gopherconuk25/proxy"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

type CLIOptions struct {
	Theme string
}

// RunCLI starts the CLI application with the given stores, telemetry, and options
func RunCLI(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, deploymentController *proxy.DeploymentController, options CLIOptions) error {
	// Create and setup CLI app
	cliApp := NewCLIApp(accountStore, noteStore, tel, deploymentController, options)
	cliApp.Setup()

	// Start the CLI
	return cliApp.Start()
}
