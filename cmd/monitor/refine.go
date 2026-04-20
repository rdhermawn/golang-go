package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"golang-refine/internal/config"
	"golang-refine/internal/discord"
	"golang-refine/internal/monitor"
	"golang-refine/internal/refine"
	"golang-refine/webui"
)

func initRefineLogger(cfg *config.Config) (*monitor.Logger, error) {
	refineLogPath := cfg.GetRefineLogPath()
	refineLogger := monitor.NewLoggerWithLegacy(refineLogPath, "./logs/monitor-log.txt")
	if err := refineLogger.Open(time.Now()); err != nil {
		return nil, err
	}
	fmt.Printf("Refine log: %s\n", refineLogger.Path())
	return refineLogger, nil
}

func processRefineEvent(
	parsed refine.ParsedRefineLine,
	sequence int64,
	observedAt time.Time,
	api *apiClient,
	refineLogger *monitor.Logger,
	hub *webui.Hub,
	cfg *config.Config,
	msgCh chan<- discord.RefineEvent,
) {
	refine.WorkerSem <- struct{}{}
	go func(parsed refine.ParsedRefineLine, seq int64, eventTime time.Time) {
		defer func() { <-refine.WorkerSem }()
		event := discord.BuildRefineEvent(
			seq,
			eventTime,
			parsed.RoleID,
			parsed.ItemID,
			parsed.Result,
			parsed.LevelBefore,
			parsed.StoneID,
			api.LookupRoleBase,
		)
		refineLogger.LogAt(event.ObservedAt, "[OK] %s", refine.BuildRefineMonitorLogMessage(
			event.RoleID,
			event.PlayerName,
			event.Result,
			event.ItemName,
			event.LevelBefore,
			event.LevelAfter,
			event.StoneID,
		))
		if cfg.Discord.ShouldSend(event.Result, event.LevelBefore, event.LevelAfter) {
			hub.Publish(webui.NewFeedEvent(event))
			discord.ProcessRefineEvent(event, cfg, msgCh)
		}
	}(parsed, sequence, observedAt)
}

func handleRefineLine(
	rawLine []byte,
	sequence *int64,
	api *apiClient,
	refineLogger *monitor.Logger,
	hub *webui.Hub,
	cfg *config.Config,
	msgCh chan<- discord.RefineEvent,
) bool {
	parsed, match := refine.ParseRefineLine(rawLine)
	if !match {
		return false
	}

	currentSequence := atomic.AddInt64(sequence, 1)
	observedAt := time.Now()
	if parsedTimestamp, ok := refine.ParseLineTimestamp(rawLine); ok {
		observedAt = parsedTimestamp
	}

	processRefineEvent(parsed, currentSequence, observedAt, api, refineLogger, hub, cfg, msgCh)
	return true
}
