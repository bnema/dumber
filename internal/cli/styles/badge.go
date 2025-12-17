package styles

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Badge renders styled metadata badges.

// VisitBadge renders a visit count badge.
func (t *Theme) VisitBadge(count int) string {
	text := fmt.Sprintf("%d visits", count)
	if count == 1 {
		text = "1 visit"
	}
	return t.BadgeMuted.Render(text)
}

// TimeBadge renders a relative time badge.
func (t *Theme) TimeBadge(tm time.Time) string {
	text := RelativeTime(tm)
	return t.BadgeMuted.Render(text)
}

// DomainBadge renders a domain badge.
func (t *Theme) DomainBadge(domain string) string {
	return t.Badge.Render(domain)
}

// AccentBadge renders a badge with accent color.
func (t *Theme) AccentBadge(text string) string {
	return t.Badge.Render(text)
}

// MutedBadge renders a badge with muted colors.
func (t *Theme) MutedBadge(text string) string {
	return t.BadgeMuted.Render(text)
}

// StatusBadge renders a status badge with custom colors.
func (t *Theme) StatusBadge(text string, fg, bg lipgloss.Color) string {
	style := lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Padding(0, 1)
	return style.Render(text)
}

// RelativeTime formats a time as a human-readable relative string.
func RelativeTime(tm time.Time) string {
	now := time.Now()
	diff := now.Sub(tm)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / (24 * 7))
		if weeks == 1 {
			return "1w ago"
		}
		return fmt.Sprintf("%dw ago", weeks)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		if months == 1 {
			return "1mo ago"
		}
		return fmt.Sprintf("%dmo ago", months)
	default:
		years := int(diff.Hours() / (24 * 365))
		if years == 1 {
			return "1y ago"
		}
		return fmt.Sprintf("%dy ago", years)
	}
}

// TimeCategory returns which timeline category a time belongs to.
type TimeCategory int

const (
	TimeCategoryToday TimeCategory = iota
	TimeCategoryYesterday
	TimeCategoryThisWeek
	TimeCategoryOlder
)

// GetTimeCategory returns the category for a given time.
func GetTimeCategory(tm time.Time) TimeCategory {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	weekAgo := today.AddDate(0, 0, -7)

	switch {
	case tm.After(today) || tm.Equal(today):
		return TimeCategoryToday
	case tm.After(yesterday) || tm.Equal(yesterday):
		return TimeCategoryYesterday
	case tm.After(weekAgo):
		return TimeCategoryThisWeek
	default:
		return TimeCategoryOlder
	}
}

// TimeCategoryLabel returns the display label for a time category.
func TimeCategoryLabel(cat TimeCategory) string {
	switch cat {
	case TimeCategoryToday:
		return "Today"
	case TimeCategoryYesterday:
		return "Yesterday"
	case TimeCategoryThisWeek:
		return "This Week"
	case TimeCategoryOlder:
		return "Older"
	default:
		return "Unknown"
	}
}
