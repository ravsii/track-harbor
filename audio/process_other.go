//go:build !unix

package audio

import "os/exec"

func configureCancellation(cmd *exec.Cmd) {}
