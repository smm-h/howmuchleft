//go:build !windows

package git

import (
	"os/exec"
	"syscall"
)

// detachChild puts the refresher in its own session, so it is not in the
// render's process group: the terminal's signals do not reach it and it keeps
// running after the render exits.
func detachChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
