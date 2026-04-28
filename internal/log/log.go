package log

import (
	"log/slog"
	"os"
	"sync"
)

var (
	mu              sync.Mutex
	file            *os.File
	path            string
	logger          *slog.Logger
	rotateThreshold int64 = 10 * 1024 * 1024
)

func Init(p string) error {
	mu.Lock()
	defer mu.Unlock()
	path = p
	if err := rotateIfNeededLocked(); err != nil {
		return err
	}
	return openLocked()
}

func openLocked() error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	file = f
	logger = slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return nil
}

func rotateIfNeededLocked() error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() < rotateThreshold {
		return nil
	}
	if file != nil {
		_ = file.Close()
		file = nil
	}
	return os.Rename(path, path+".1")
}

func rotateAfterWriteLocked() {
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() < rotateThreshold {
		return
	}
	if file != nil {
		_ = file.Close()
		file = nil
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return
	}
	_ = openLocked()
}

func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
}

func Info(msg string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if logger == nil {
		return
	}
	logger.Info(msg, args...)
	rotateAfterWriteLocked()
}

func Warn(msg string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if logger == nil {
		return
	}
	logger.Warn(msg, args...)
	rotateAfterWriteLocked()
}

func Error(msg string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if logger == nil {
		return
	}
	logger.Error(msg, args...)
	rotateAfterWriteLocked()
}
