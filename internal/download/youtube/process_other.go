//go:build !unix

package youtube

import "os/exec"

// CommandContext terminates yt-dlp; WaitDelay also bounds inherited pipe waits.
func configureCancellation(cmd *exec.Cmd) {}
