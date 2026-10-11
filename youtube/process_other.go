//go:build !unix

package youtube

import "os/exec"

func configureCancellation(cmd *exec.Cmd) {}
