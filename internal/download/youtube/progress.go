package youtube

import (
	"encoding/json"
	"strings"
	"time"

	"meshvault/internal/download"
)

const progressPrefix = "meshvault-progress:"
const progressTemplate = `download:meshvault-progress:{"downloaded":%(progress.downloaded_bytes|0)s,"total":%(progress.total_bytes,progress.total_bytes_estimate|0)s,"speed":%(progress.speed|0)s,"eta":%(progress.eta|0)s,"status":%(progress.status)j}`

// Consume subprocess stdout incrementally without exposing URLs or raw output.
// exec.Cmd serializes calls to this writer and joins them before Run returns.
type progressWriter struct {
	line       []byte
	dropping   bool
	onProgress func(download.DownloadProgress)
}

func (w *progressWriter) Write(data []byte) (int, error) {
	for _, b := range data {
		if b == '\n' || b == '\r' {
			w.flush()
			w.dropping = false
		} else if !w.dropping {
			if len(w.line) >= 16*1024 {
				w.line = nil
				w.dropping = true
			} else {
				w.line = append(w.line, b)
			}
		}
	}
	return len(data), nil
}

func (w *progressWriter) flush() {
	line := string(w.line)
	w.line = w.line[:0]
	if w.dropping || !strings.HasPrefix(line, progressPrefix) {
		return
	}
	var event struct {
		Downloaded int64   `json:"downloaded"`
		Total      int64   `json:"total"`
		Speed      float64 `json:"speed"`
		ETA        float64 `json:"eta"`
		Status     string  `json:"status"`
	}
	if json.Unmarshal([]byte(strings.TrimPrefix(line, progressPrefix)), &event) != nil {
		return
	}
	if event.Total <= 0 {
		event.Total = -1
	}
	w.onProgress(download.DownloadProgress{
		DownloadedBytes: max(0, event.Downloaded), TotalBytes: event.Total,
		BytesPerSecond: max(0, event.Speed), ETA: time.Duration(max(0, event.ETA) * float64(time.Second)),
		Converting: event.Status == "finished",
	})
}
