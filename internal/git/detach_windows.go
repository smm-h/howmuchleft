//go:build windows

package git

import (
	"os/exec"
	"syscall"
)

// detachChild starts the refresher without a console window, so it does not
// flash one over the terminal Claude Code is drawing on. Windows processes are
// not in a process group the way Unix ones are, so nothing else is needed for
// the child to outlive the render.
func detachChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
