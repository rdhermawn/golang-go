package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang-refine/internal/config"
	"golang-refine/internal/discord"
	"golang-refine/internal/game"
	"golang-refine/internal/monitor"
	"golang-refine/internal/refine"
	"golang-refine/internal/search"
	"golang-refine/internal/tail"
)

func main() {
	refine.Init()

	cfg, err := config.Load("./configs/config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 && os.Args[1] == "search-refine" {
		os.Exit(search.Run(cfg, os.Args[2:]))
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
			refine.WorkerSem <- struct{}{}
			go func() {
				defer func() { <-refine.WorkerSem }()
				discord.ProcessRefineEvent(parsed.RoleID, parsed.ItemID, parsed.Result, parsed.LevelBefore, parsed.StoneID, api.LookupRoleBase, cfg, msgCh)
			}()
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
