package console

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// progress.go renders terminal progress bars for long-running operations.

// ProgressBar renders a simple single-line progress bar to stdout.
type ProgressBar struct {
	label string
	total int
	width int
	out   io.Writer
	mu    sync.Mutex
}

// NewProgressBar creates a new progress bar with the given label and total.
func NewProgressBar(label string, total int) *ProgressBar {
	if total < 1 {
		total = 1
	}
	return &ProgressBar{
		label: label,
		total: total,
		width: 28,
		out:   os.Stderr,
	}
}

// Update redraws the progress bar with the provided completed count.
func (p *ProgressBar) Update(completed int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if completed < 0 {
		completed = 0
	}
	if completed > p.total {
		completed = p.total
	}

	percent := float64(completed) / float64(p.total)
	filled := int(percent * float64(p.width))
	if filled > p.width {
		filled = p.width
	}

	bar := strings.Repeat("=", filled) + strings.Repeat(" ", p.width-filled)
	percentInt := int(percent * 100.0)

	fmt.Fprintf(p.out, "\r%s [%s] %3d%% (%d/%d)", p.label, bar, percentInt, completed, p.total)
	if completed == p.total {
		fmt.Fprintln(p.out)
	}
}
