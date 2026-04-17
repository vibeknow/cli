package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func ThemeVibeKnow() *huh.Theme {
	t := huh.ThemeBase()

	purple := lipgloss.Color("#5B4CEE")
	green := lipgloss.Color("#10B981")
	red := lipgloss.Color("#EF4444")
	text := lipgloss.AdaptiveColor{Light: "#111827", Dark: "#E5E7EB"}
	subtext := lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}

	t.Focused.Base = t.Focused.Base.BorderForeground(purple)
	t.Focused.Title = t.Focused.Title.Foreground(purple).Bold(true)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(red)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(purple)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(green)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(green).SetString("✓ ")
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(subtext).SetString("• ")
	t.Focused.FocusedButton = t.Focused.FocusedButton.
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(purple).
		Bold(true)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(purple)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(subtext)
	t.Focused.Description = t.Focused.Description.Foreground(text)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())

	return t
}
