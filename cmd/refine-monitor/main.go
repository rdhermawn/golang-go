package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang-refine/internal/backfill"
	"golang-refine/internal/config"
	"golang-refine/internal/discord"
	"golang-refine/internal/game"
	"golang-refine/internal/monitor"
	"golang-refine/internal/refine"
	"golang-refine/internal/search"
	"golang-refine/internal/tail"
	"golang-refine/webui"
)

func main() {
	refine.Init()

	cfg, err := config.Load("./configs/config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "search-refine":
			os.Exit(search.Run(cfg, os.Args[2:]))
		case "backfill-discord":
			api := NewAPIClient(cfg)
			os.Exit(backfill.Run(cfg, os.Args[2:], api.LookupRoleBase))
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	api := NewAPIClient(cfg)
	gameLogFile := strings.TrimSpace(cfg.LogFile)
	if gameLogFile == "" {
		fmt.Fprintln(os.Stderr, "Missing required config: log_file")
		os.Exit(1)
	}

	if err := monitor.OpenLog(time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open monitor log: %v\n", err)
	} else {
		fmt.Printf("Monitor log: %s\n", monitor.LogPath)
	}

	hub := webui.NewHub(cfg.GetWebRecentBufferSize())
	lastSequence, err := hub.LoadMonitorHistory(monitor.LogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed web history: %v\n", err)
	}
	var sequence int64 = lastSequence

	if cfg.IsWebEnabled() {
		webAddr := cfg.GetWebAddr()
		listener, err := webui.Listen(webAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Web UI disabled: %v\n", formatWebListenError(webAddr, err))
		} else {
			go func() {
				if err := webui.Start(ctx, listener, hub); err != nil && !errors.Is(err, context.Canceled) {
					fmt.Fprintf(os.Stderr, "Web UI stopped: %v\n", err)
				}
			}()
			fmt.Printf("Web UI: http://%s\n", listener.Addr().String())
		}
	}
	fmt.Printf("Starting refine monitor...\n")
	fmt.Printf("Log file: %s\n", gameLogFile)
	fmt.Printf("Server online: %v\n", api.game.ServerOnline())

	lineCh := make(chan []byte, 100)
	msgCh := make(chan discord.RefineEvent, 100)

	go monitor.StartRotation(ctx.Done())
	go discord.StartHourlySummary(cfg, ctx.Done())
	go discord.StartSender(cfg, msgCh)
	go tail.Start(gameLogFile, lineCh, ctx.Done())

	fmt.Println("Monitoring refines...")

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nShutting down gracefully...")
			close(msgCh)
			time.Sleep(2 * time.Second)
			return
		case rawLine, ok := <-lineCh:
			if !ok {
				return
			}
			parsed, match := refine.ParseRefineLine(rawLine)
			if !match {
				continue
			}
			currentSequence := atomic.AddInt64(&sequence, 1)
			observedAt := time.Now()
			if parsedTimestamp, ok := refine.ParseLineTimestamp(rawLine); ok {
				observedAt = parsedTimestamp
			}
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
				monitor.LogToFileAt(event.ObservedAt, "[OK] %s", refine.BuildRefineMonitorLogMessage(
					event.RoleID,
					event.PlayerName,
					event.Result,
					event.ItemName,
					event.LevelBefore,
					event.LevelAfter,
					event.StoneID,
				))
				hub.Publish(webui.NewFeedEvent(event))
				discord.ProcessRefineEvent(event, cfg, msgCh)
			}(parsed, currentSequence, observedAt)
		}
	}
}

type apiClient struct {
	game *game.API
}

func NewAPIClient(cfg *config.Config) *apiClient {
	return &apiClient{game: game.NewAPI(cfg)}
}

func (a *apiClient) LookupRoleBase(roleID int) (string, error) {
	return a.game.GetRoleBase(roleID)
}

func formatWebListenError(addr string, err error) error {
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("cannot bind %s: address already in use; free the port or change \"web_addr\" in configs/config.json", addr)
	}

	var addrErr *net.AddrError
	if errors.As(err, &addrErr) {
		return fmt.Errorf("invalid web address %q: %w", addr, err)
	}

	return err
}
