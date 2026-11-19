package main

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/razoreater/trnr/internal/garmin"
)

// Colors per activity type. AdaptiveColor picks Light or Dark depending on
// the terminal's background, so the text stays readable on both.
var typeColors = map[string]lipgloss.AdaptiveColor{
	"running": {Light: "#C2410C", Dark: "#FB923C"}, // orange
	"cycling": {Light: "#1D4ED8", Dark: "#60A5FA"}, // blue
	"walking": {Light: "#15803D", Dark: "#4ADE80"}, // green
}

var borderColor = lipgloss.AdaptiveColor{Light: "#A1A1AA", Dark: "#52525B"} // grey

// theme holds the styles for one output. The renderer checks whether that
// output is a terminal: if it isn't (a pipe, a file, a test buffer), the
// styles render without colors.
type theme struct {
	cell   lipgloss.Style
	header lipgloss.Style
	border lipgloss.Style
	title  lipgloss.Style
	box    lipgloss.Style
}

func newTheme(out io.Writer) theme {
	r := lipgloss.NewRenderer(out)
	cell := r.NewStyle().Padding(0, 1)
	return theme{
		cell:   cell,
		header: cell.Bold(true),
		border: r.NewStyle().Foreground(borderColor),
		title:  cell.Bold(true),
		box: r.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor),
	}
}

// rightAligned reports which columns hold numbers. Those are right-aligned so
// the digits line up.
func rightAligned(headers []string, col int) bool {
	switch headers[col] {
	case "KM", "TIME", "PACE", "HR", "TE", "COUNT":
		return true
	}
	return false
}

// newTable sets up a table with the shared look: rounded grey border, bold
// header, numbers right-aligned. rowStyle can add a style per data row; it
// may be nil.
func newTable(th theme, headers []string, rows [][]string, rowStyle func(row int, s lipgloss.Style) lipgloss.Style) *table.Table {
	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(th.border).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := th.cell
			if row == table.HeaderRow {
				s = th.header
			} else if rowStyle != nil {
				s = rowStyle(row, s)
			}
			if rightAligned(headers, col) {
				s = s.Align(lipgloss.Right)
			}
			return s
		})
}

func printTable(out io.Writer, activities []garmin.Activity) {
	th := newTheme(out)
	t := newTable(th, activityHeaders, activityRows(activities),
		func(row int, s lipgloss.Style) lipgloss.Style {
			if color, ok := typeColors[activities[row].Type.TypeKey]; ok {
				return s.Foreground(color)
			}
			return s
		})
	fmt.Fprintln(out, t.Render())
}

// printWeeks prints the weekly totals as a small table inside a box.
func printWeeks(out io.Writer, totals map[string]*weekTotal) {
	th := newTheme(out)
	// Inside the box, only the line under the header is drawn.
	t := newTable(th, weekHeaders, weekRows(totals), nil).
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false)
	fmt.Fprintln(out, th.box.Render(lipgloss.JoinVertical(lipgloss.Left,
		th.title.Render("Weekly totals"),
		t.Render(),
	)))
}
