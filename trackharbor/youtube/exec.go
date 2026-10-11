package youtube

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Run executes yt-dlp with explicit routing. Passing an empty proxy address
// forces a direct connection instead of inheriting environment proxies.
// Caller interprets stderr without ever logging signed URLs or cookies.
func Run(ctx context.Context, executable, proxyAddress, progressTemplate string, stdout io.Writer, args ...string) ([]byte, []byte, error) {
	options := []string{"--ignore-config", "--no-progress", "--no-cache-dir", "--socket-timeout", "15", "--retries", "0", "--fragment-retries", "0", "--extractor-retries", "0", "--file-access-retries", "0"}
	if stdout != nil {
		options = append(options, "--progress", "--newline", "--progress-delta", "0.5", "--progress-template", progressTemplate)
	}
	options = append(options, "--proxy", proxyAddress)
	cmd := exec.CommandContext(ctx, executable, append(options, args...)...)
	cmd.Env = make([]string, 0, len(os.Environ()))
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		switch strings.ToUpper(key) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			continue
		}
		cmd.Env = append(cmd.Env, env)
	}
	cmd.WaitDelay = time.Second
	configureCancellation(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if stdout != nil {
		cmd.Stdout = stdout
		err := cmd.Run()
		return nil, stderr.Bytes(), err
	}
	output, err := cmd.Output()
	return output, stderr.Bytes(), err
}
