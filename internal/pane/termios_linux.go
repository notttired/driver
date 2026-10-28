package pane

import "golang.org/x/sys/unix"

// The termios ioctls (x/term keeps its own copies unexported).
const (
	getTermios = unix.TCGETS
	setTermios = unix.TCSETS
)
