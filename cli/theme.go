package cli

// This file contains theme-related utilities for the bubbletea CLI implementation.
// The old tview-specific theme code has been removed and replaced with lipgloss-based
// theming in the bubbletea.go file.

// GetBubbleTeaTheme returns the appropriate BubbleTeaTheme based on theme name
func GetBubbleTeaTheme(themeName string) BubbleTeaTheme {
	switch themeName {
	case "light":
		return lightTheme
	case "dark":
		fallthrough
	default:
		return darkTheme
	}
}