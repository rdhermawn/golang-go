package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func StartRetentionCleaner(dirs []string, getRetentionDays func() int, done <-chan struct{}) {
	cleanAll(dirs, getRetentionDays())

	for {
		now := time.Now()
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		select {
		case <-done:
			return
		case <-time.After(time.Until(nextMidnight)):
		}
		cleanAll(dirs, getRetentionDays())
	}
}

func cleanAll(dirs []string, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, dir := range dirs {
		if err := cleanDir(dir, cutoff); err != nil {
			fmt.Fprintf(os.Stderr, "[RETENTION] failed to clean %s: %v\n", dir, err)
		}
	}
}

func cleanDir(dir string, cutoff time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, name)
			if err := os.Remove(path); err != nil {
				fmt.Fprintf(os.Stderr, "[RETENTION] failed to remove %s: %v\n", path, err)
			} else {
				fmt.Printf("[RETENTION] removed old log: %s\n", path)
			}
		}
	}
	return nil
}
