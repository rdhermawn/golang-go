package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang-refine/internal/config"
)

type errorEntry struct {
	Key   string
	Count int
}

type errorSnapshot struct {
	WindowStart time.Time
	WindowEnd   time.Time
	TotalCount  int
	TopMessages []errorEntry
	TopFile     errorEntry
	HasTopFile  bool
}

type errorCollector struct {
	mu           sync.Mutex
	windowStart  time.Time
	totalCount   int
	messageCount map[string]int
	fileCount    map[string]int
}

var collector = newCollector(time.Now())

func newCollector(now time.Time) *errorCollector {
	return &errorCollector{
		windowStart:  now.Truncate(time.Hour),
		messageCount: make(map[string]int),
		fileCount:    make(map[string]int),
	}
}

func RecordError(message, sourceFile string) {
	if collector == nil {
		return
	}
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if message == "" {
		message = "unknown error"
	}
	sourceFile = strings.TrimSpace(sourceFile)
	if sourceFile == "" {
		sourceFile = "(unknown)"
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.windowStart.IsZero() {
		collector.windowStart = time.Now().Truncate(time.Hour)
	}
	collector.totalCount++
	collector.messageCount[message]++
	collector.fileCount[sourceFile]++
}

func (c *errorCollector) snapshotAndReset(now time.Time) errorSnapshot {
	if c == nil {
		return errorSnapshot{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	windowEnd := now.Truncate(time.Hour)
	if c.windowStart.IsZero() {
		c.windowStart = windowEnd
	}
	snapshot := errorSnapshot{
		WindowStart: c.windowStart,
		WindowEnd:   windowEnd,
		TotalCount:  c.totalCount,
		TopMessages: topEntries(c.messageCount, 5),
	}
	if topFiles := topEntries(c.fileCount, 1); len(topFiles) > 0 {
		snapshot.TopFile = topFiles[0]
		snapshot.HasTopFile = true
	}
	c.windowStart = windowEnd
	c.totalCount = 0
	c.messageCount = make(map[string]int)
	c.fileCount = make(map[string]int)
	return snapshot
}

func topEntries(counts map[string]int, limit int) []errorEntry {
	entries := make([]errorEntry, 0, len(counts))
	for key, count := range counts {
		entries = append(entries, errorEntry{Key: key, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count == entries[j].Count {
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Count > entries[j].Count
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

func truncateText(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func formatTopMessages(entries []errorEntry) string {
	if len(entries) == 0 {
		return "-"
	}
	lines := make([]string, 0, len(entries))
	currentLen := 0
	for i, entry := range entries {
		line := fmt.Sprintf("%d. %s (%d)", i+1, truncateText(entry.Key, 180), entry.Count)
		if currentLen+len(line)+1 > 1000 {
			break
		}
		lines = append(lines, line)
		currentLen += len(line) + 1
	}
	if len(lines) == 0 {
		return "-"
	}
	return strings.Join(lines, "\n")
}

func sendHourlySummary(webhookURL, footer string, snapshot errorSnapshot) error {
	if webhookURL == "" {
		return fmt.Errorf("missing webhook for hourly error summary")
	}
	description := fmt.Sprintf("Period: %s - %s\nTotal error count: **%d**",
		snapshot.WindowStart.Format("2006-01-02 15:04"),
		snapshot.WindowEnd.Format("2006-01-02 15:04"),
		snapshot.TotalCount,
	)
	if snapshot.HasTopFile {
		description += fmt.Sprintf("\nFile with most errors: %s (%d)",
			truncateText(snapshot.TopFile.Key, 200), snapshot.TopFile.Count)
	} else {
		description += "\nFile with most errors: -"
	}

	embed := map[string]interface{}{
		"title":       "Hourly Error Summary",
		"color":       0xFF5555,
		"description": description,
		"fields": []map[string]interface{}{
			{"name": "Top 5 Error Messages", "value": formatTopMessages(snapshot.TopMessages), "inline": false},
		},
		"footer":    map[string]interface{}{"text": footer},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	payload := map[string]interface{}{"embeds": []map[string]interface{}{embed}}
	jsonData, _ := json.Marshal(payload)

	resp, err := Client.Post(webhookURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("error sending hourly summary to Discord: %w", err)
	}
	defer DrainAndClose(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hourly summary webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func StartHourlySummary(done <-chan struct{}) {
	for {
		nextHour := time.Now().Truncate(time.Hour).Add(time.Hour)
		select {
		case <-done:
			return
		case <-time.After(time.Until(nextHour)):
		}
		snapshot := collector.snapshotAndReset(time.Now())
		if snapshot.TotalCount == 0 {
			continue
		}
		cfg := config.Current()
		if err := sendHourlySummary(cfg.Discord.GetSummaryWebhook(), cfg.Discord.Footer, snapshot); err != nil {
			fmt.Printf("[ERROR] failed to send hourly error summary: %v\n", err)
			continue
		}
	}
}
