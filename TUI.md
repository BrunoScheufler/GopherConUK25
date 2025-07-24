# Switching from rivo/tview to charmbracelet/bubbletea

## Goal

- Reimplement the current UI using charmbracelet/bubbletea
- Fix longstanding issues including responsive resizing and a wrap layout

## Libraries to use

- github.com/charmbracelet/bubbletea for the TUI library to use
- github.com/charmbracelet/lipgloss for rendering a responsive layout
- github.com/charmbracelet/bubbles for rendering tables, progress bars, and help

## Steps

- Implement a basic TUI with responsive panels including the main categories
  - API requests
  - Data store access
  - Deployments
  - Logs at the bottom
- Respond to terminal resize events to adjust the layout
- Render the content for each panel
  - Render metric in a table with columns for each field (e.g. method, status, p95 duration, count)
  - Render progress bar for deployment using the bubbles library

- Completely remove all previous CLI code that used rivo/tview
