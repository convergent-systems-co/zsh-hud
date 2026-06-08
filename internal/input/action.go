package input

// Mode is the interpreter's current input mode.
type Mode int

const (
	ModeNormal Mode = iota // bytes forwarded to the shell
	ModeCopy               // bytes captured for scrollback/selection
)

// Action is a semantic HUD command produced from input.
type Action int

const (
	ScrollPageUp Action = iota
	ScrollPageDown
	ScrollLineUp
	ScrollLineDown
	EnterCopyMode
	ExitCopyMode
	CopyMoveUp
	CopyMoveDown
	CopyMoveLeft
	CopyMoveRight
	CopyToggleSelect
	CopyYank
)

// Result is the outcome of feeding bytes: Forward goes to the pty; Actions are
// applied by main in order.
type Result struct {
	Forward []byte
	Actions []Action
}
