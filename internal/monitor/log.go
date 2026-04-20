package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logDateLayout = "2006-01-02"
	logTimeLayout = "2006-01-02 15:04:05"
	maxArchive    = 100
)

// Legacy constants for backward compatibility
const (
	LogDir        = "./logs"
	LogPath       = "./logs/monitor-log.txt"
)

var legacyLogPath = "./monitor-log.txt"

// Logger manages a single log file with rotation
type Logger struct {
	path       string
	legacyPath string
	logFile    *os.File
	logDate    string
	mu         sync.Mutex
}

var (
	logFile *os.File
	logDate string
	mu      sync.Mutex
)

// NewLogger creates a new logger for the given log path
func NewLogger(logPath string) *Logger {
	return &Logger{
		path:       logPath,
		legacyPath: "",
	}
}

// NewLoggerWithLegacy creates a new logger with a legacy path for migration
func NewLoggerWithLegacy(logPath, legacyPath string) *Logger {
	return &Logger{
		path:       logPath,
		legacyPath: legacyPath,
	}
}

// Open opens the log file, creating directories if needed
func (l *Logger) Open(now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.openLocked(now)
}

// Log writes a formatted message to the log file
func (l *Logger) Log(format string, args ...interface{}) {
	now := time.Now()
	l.LogAt(now, format, args...)
}

// LogAt writes a formatted message with a specific timestamp
func (l *Logger) LogAt(now time.Time, format string, args ...interface{}) {
	if now.IsZero() {
		now = time.Now()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.rotateIfNeededLocked(now); err != nil {
		return
	}
	if l.logFile == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	l.logFile.WriteString(fmt.Sprintf("[%s] %s\n", now.Format(logTimeLayout), msg))
	l.logFile.Sync()
}

// StartRotation starts a goroutine that rotates logs at midnight
func (l *Logger) StartRotation(done <-chan struct{}) {
	for {
		now := time.Now()
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		select {
		case <-done:
			return
		case <-time.After(time.Until(nextMidnight)):
		}
		l.mu.Lock()
		_ = l.rotateIfNeededLocked(time.Now())
		l.mu.Unlock()
	}
}

// Path returns the log file path
func (l *Logger) Path() string {
	return l.path
}

func (l *Logger) nextArchivePath(date string) (string, error) {
	ext := filepath.Ext(l.path)
	name := strings.TrimSuffix(l.path, ext)
	archivePath := fmt.Sprintf("%s-%s%s", name, date, ext)

	if _, err := os.Stat(archivePath); err == nil {
	} else if os.IsNotExist(err) {
		return archivePath, nil
	} else {
		return "", err
	}

	for i := 1; i < maxArchive; i++ {
		candidate := fmt.Sprintf("%s-%s-%d%s", name, date, i, ext)
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if os.IsNotExist(err) {
			return candidate, nil
		} else {
			return "", err
		}
	}
	return "", fmt.Errorf("too many archive files for %s-%s", name, date)
}

func (l *Logger) ensureDir() error {
	dir := filepath.Dir(l.path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

func (l *Logger) migrateLegacy() error {
	if l.legacyPath == "" || l.legacyPath == l.path {
		return nil
	}
	if _, err := os.Stat(l.legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(l.path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(l.legacyPath, l.path)
}

func (l *Logger) archiveLocked(date string) error {
	if l.logFile != nil {
		if err := l.logFile.Close(); err != nil {
			return err
		}
		l.logFile = nil
	}
	if date == "" {
		date = time.Now().Format(logDateLayout)
	}
	if _, err := os.Stat(l.path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	archivePath, err := l.nextArchivePath(date)
	if err != nil {
		return err
	}
	return os.Rename(l.path, archivePath)
}

func (l *Logger) openLocked(now time.Time) error {
	currentDate := now.Format(logDateLayout)
	if err := l.ensureDir(); err != nil {
		return err
	}
	if err := l.migrateLegacy(); err != nil {
		return err
	}
	if info, err := os.Stat(l.path); err == nil {
		fileDate := info.ModTime().Format(logDateLayout)
		if fileDate != currentDate {
			if err := l.archiveLocked(fileDate); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if l.logFile != nil {
		l.logDate = currentDate
		return nil
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	l.logFile = f
	l.logDate = currentDate
	return nil
}

func (l *Logger) rotateIfNeededLocked(now time.Time) error {
	currentDate := now.Format(logDateLayout)
	if l.logFile == nil {
		return l.openLocked(now)
	}
	if l.logDate == currentDate {
		return nil
	}
	if err := l.archiveLocked(l.logDate); err != nil {
		return err
	}
	return l.openLocked(now)
}

func OpenLog(now time.Time) error {
	mu.Lock()
	defer mu.Unlock()
	return openLogLocked(now)
}

func LogToFile(format string, args ...interface{}) {
	now := time.Now()
	LogToFileAt(now, format, args...)
}

func LogToFileAt(now time.Time, format string, args ...interface{}) {
	if now.IsZero() {
		now = time.Now()
	}
	mu.Lock()
	defer mu.Unlock()
	if err := rotateIfNeededLocked(now); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to prepare monitor log: %v\n", err)
		return
	}
	if logFile == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	logFile.WriteString(fmt.Sprintf("[%s] %s\n", now.Format(logTimeLayout), msg))
	logFile.Sync()
}

func StartRotation(done <-chan struct{}) {
	for {
		now := time.Now()
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		select {
		case <-done:
			return
		case <-time.After(time.Until(nextMidnight)):
		}
		mu.Lock()
		err := rotateIfNeededLocked(time.Now())
		mu.Unlock()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to rotate monitor log: %v\n", err)
		}
	}
}

func nextArchivePath(basePath, date string) (string, error) {
	ext := filepath.Ext(basePath)
	name := strings.TrimSuffix(basePath, ext)
	archivePath := fmt.Sprintf("%s-%s%s", name, date, ext)

	if _, err := os.Stat(archivePath); err == nil {
	} else if os.IsNotExist(err) {
		return archivePath, nil
	} else {
		return "", err
	}

	for i := 1; i < maxArchive; i++ {
		candidate := fmt.Sprintf("%s-%s-%d%s", name, date, i, ext)
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if os.IsNotExist(err) {
			return candidate, nil
		} else {
			return "", err
		}
	}
	return "", fmt.Errorf("too many archive files for %s-%s", name, date)
}

func ensureDir() error {
	return os.MkdirAll(LogDir, 0755)
}

func migrateLegacy() error {
	if legacyLogPath == LogPath {
		return nil
	}
	if _, err := os.Stat(legacyLogPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(LogPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(legacyLogPath, LogPath)
}

func archiveLocked(date string) error {
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			return err
		}
		logFile = nil
	}
	if date == "" {
		date = time.Now().Format(logDateLayout)
	}
	if _, err := os.Stat(LogPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	archivePath, err := nextArchivePath(LogPath, date)
	if err != nil {
		return err
	}
	return os.Rename(LogPath, archivePath)
}

func openLogLocked(now time.Time) error {
	currentDate := now.Format(logDateLayout)
	if err := ensureDir(); err != nil {
		return err
	}
	if err := migrateLegacy(); err != nil {
		return err
	}
	if info, err := os.Stat(LogPath); err == nil {
		fileDate := info.ModTime().Format(logDateLayout)
		if fileDate != currentDate {
			if err := archiveLocked(fileDate); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if logFile != nil {
		logDate = currentDate
		return nil
	}
	f, err := os.OpenFile(LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	logFile = f
	logDate = currentDate
	return nil
}

func rotateIfNeededLocked(now time.Time) error {
	currentDate := now.Format(logDateLayout)
	if logFile == nil {
		return openLogLocked(now)
	}
	if logDate == currentDate {
		return nil
	}
	if err := archiveLocked(logDate); err != nil {
		return err
	}
	return openLogLocked(now)
}
