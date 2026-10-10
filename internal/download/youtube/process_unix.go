//go:build unix

package youtube

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// yt-dlp starts FFmpeg/JS children; cancel the whole isolated process group.
func configureCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
