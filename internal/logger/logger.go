package logger

import (
	"context"
	"io"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Init(directory string) error {
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	files, err := newWriter(directory)
	if err != nil {
		return err
	}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())

	// stderr is unbuffered. Expose only Write so Sync does not fsync a terminal.
	console := zapcore.Lock(zapcore.AddSync(struct{ io.Writer }{os.Stderr}))
	zap.ReplaceGlobals(zap.New(zapcore.NewTee(
		zapcore.NewCore(encoder, console, zap.InfoLevel),
		zapcore.NewCore(encoder, &files, zap.InfoLevel),
	), zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)))

	return nil
}

func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	zap.L().WithOptions(zap.AddCallerSkip(1)).Debug(msg, fields...)
}

func Info(ctx context.Context, msg string, fields ...zap.Field) {
	zap.L().WithOptions(zap.AddCallerSkip(1)).Info(msg, fields...)
}

func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	zap.L().WithOptions(zap.AddCallerSkip(1)).Warn(msg, fields...)
}

func Err(ctx context.Context, msg string, fields ...zap.Field) {
	zap.L().WithOptions(zap.AddCallerSkip(1)).Error(msg, fields...)
}

// Fatal logs the message and exits the process with status 1.
func Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	zap.L().WithOptions(zap.AddCallerSkip(1)).Fatal(msg, fields...)
}
