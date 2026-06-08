// Package session owns the interactive state layered over the shell's grid:
// the scrollback offset (and, in a later plan, copy-mode cursor/selection). It
// builds a render.Frame each time the screen needs drawing, combining the
// compositor's three-region layout with the hud bars. It is pure: the shell
// grid is read through compositor.GridSource, so session is unit-testable.
package session

import (
	"terminal-hud/internal/compositor"
	"terminal-hud/internal/hud"
	"terminal-hud/internal/input"
	"terminal-hud/internal/render"
)

// Session holds interactive view state over a GridSource.
type Session struct {
	src        compositor.GridSource
	rows, cols int // full screen dimensions (including both bars)
	scrollOff  int
	deps       hud.Deps
}

// New returns a Session for a rows x cols screen reading from src (live view).
func New(src compositor.GridSource, rows, cols int) *Session {
	return &Session{src: src, rows: rows, cols: cols}
}

// Resize updates the full-screen dimensions. The caller resizes the engine to
// the middle (rows-2 x cols) separately.
func (s *Session) Resize(rows, cols int) {
	s.rows, s.cols = rows, cols
	s.clampScroll()
}

// SetDeps sets the HUD segment strings used for the next Frame.
func (s *Session) SetDeps(d hud.Deps) { s.deps = d }

// ScrollOffset reports the current scrollback offset (0 = live).
func (s *Session) ScrollOffset() int { return s.scrollOff }

// midHeight is the number of shell-view rows between the two bars.
func (s *Session) midHeight() int {
	if s.rows < 2 {
		return 0
	}
	return s.rows - 2
}

// Apply mutates session state for a HUD action. Scroll actions move the
// scrollback offset (clamped); ExitCopyMode/CopyYank return to the live view.
// Copy-mode cursor/selection actions are accepted but not yet acted on (Plan 7b).
func (s *Session) Apply(a input.Action) {
	page := s.midHeight() - 1
	if page < 1 {
		page = 1
	}
	switch a {
	case input.ScrollLineUp:
		s.scrollOff++
	case input.ScrollLineDown:
		s.scrollOff--
	case input.ScrollPageUp:
		s.scrollOff += page
	case input.ScrollPageDown:
		s.scrollOff -= page
	case input.ExitCopyMode, input.CopyYank:
		s.scrollOff = 0
	default:
		// EnterCopyMode, CopyMove*, CopyToggleSelect: no-op until Plan 7b
	}
	s.clampScroll()
}

func (s *Session) clampScroll() {
	if s.scrollOff < 0 {
		s.scrollOff = 0
	}
	if max := s.src.ScrollbackLen(); s.scrollOff > max {
		s.scrollOff = max
	}
}

// Frame composes the current screen: top/bottom HUD bars and the shell view at
// the current scroll offset.
func (s *Session) Frame() *render.Frame {
	top := hud.TopBar(s.cols, s.deps)
	bottom := hud.BottomBar(s.cols, s.deps)
	return compositor.Compose(s.rows, s.cols, s.src, s.scrollOff, top, bottom)
}
