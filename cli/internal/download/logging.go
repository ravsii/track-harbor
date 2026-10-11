package download

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewFileLogger(path string) (*zap.Logger, func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, err
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return nil, nil, err
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.Lock(zapcore.AddSync(file)), zap.InfoLevel)
	logger := zap.New(core).With(zap.String("run_id", fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())))
	return logger, func() error { return errors.Join(logger.Sync(), file.Close()) }, nil
}

func (d *Downloader) WithLogger(logger *zap.Logger) *Downloader {
	if logger == nil {
		logger = zap.NewNop()
	}
	d.logger = logger
	return d
}
