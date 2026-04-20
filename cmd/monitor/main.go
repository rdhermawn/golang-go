package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang-refine/internal/backfill"
	"golang-refine/internal/config"
	"golang-refine/internal/craft"
	"golang-refine/internal/discord"
	"golang-refine/internal/game"
	"golang-refine/internal/monitor"
	"golang-refine/internal/pickup"
	"golang-refine/internal/refine"
	"golang-refine/internal/search"
	"golang-refine/internal/sendmail"
	"golang-refine/internal/tail"
	"golang-refine/webui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	refine.Init()

	cfg, err := config.Load("./configs/config.json")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Discord.PickupEnabled {
		tabPath := "./data/RAE_Exported_Table.tab"
		rareConfPath := "/home/gamed/config/rare_item.conf"
		if err := pickup.Init(tabPath, rareConfPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to init pickup: %v\n", err)
		}
	}

	if cfg.Discord.CraftEnabled {
		tabPath := "./data/RAE_Exported_Table.tab"
		craftConfPath := "./configs/craft_items.conf"
		if err := craft.Init(tabPath, craftConfPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to init craft: %v\n", err)
		}
	}

	if cfg.Discord.SendmailEnabled {
		tabPath := "./data/RAE_Exported_Table.tab"
		if err := sendmail.Init(tabPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to init sendmail: %v\n", err)
		}
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
		return fmt.Errorf("missing required config: log_file")
	}

	refineLogger, err := initRefineLogger(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize refine logger: %w", err)
	}

	pickupLogger, err := initPickupLogger(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize pickup logger: %w", err)
	}

	craftLogger, err := initCraftLogger(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize craft logger: %w", err)
	}

	sendmailLogger, err := initSendmailLogger(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize sendmail logger: %w", err)
	}

	hub := webui.NewHub(cfg.GetWebRecentBufferSize())
	refineLogDir := filepath.Dir(refineLogger.Path())
	lastSequence, err := hub.LoadAllFromDirAndParser(refineLogDir, webui.ParseRefineLogLine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed refine history: %v\n", err)
	}
	if pickupLogger != nil {
		seq, err := hub.LoadPickupHistory(pickupLogger.Path(), lastSequence)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to seed pickup history: %v\n", err)
		} else {
			lastSequence = seq
		}
	}
	if craftLogger != nil {
		seq, err := hub.LoadCraftHistory(craftLogger.Path(), lastSequence)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to seed craft history: %v\n", err)
		} else {
			lastSequence = seq
		}
	}
	if sendmailLogger != nil {
		seq, err := hub.LoadSendmailHistory(sendmailLogger.Path(), lastSequence)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to seed sendmail history: %v\n", err)
		} else {
			lastSequence = seq
		}
	}

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

	formatLogFile := strings.TrimSpace(cfg.FormatLogPath)
	if formatLogFile != "" {
		fmt.Printf("Format log: %s\n", formatLogFile)
	}
	fmt.Printf("Server online: %v\n", api.game.ServerOnline())

	lineCh := make(chan []byte, 100)
	formatLineCh := make(chan []byte, 100)
	msgCh := make(chan discord.RefineEvent, 100)
	pickupCh := make(chan discord.PickupEvent, 100)
	craftCh := make(chan discord.CraftEvent, 100)
	sendmailCh := make(chan discord.SendmailEvent, 100)

	go refineLogger.StartRotation(ctx.Done())
	if pickupLogger != nil {
		go pickupLogger.StartRotation(ctx.Done())
	}
	if craftLogger != nil {
		go craftLogger.StartRotation(ctx.Done())
	}
	if sendmailLogger != nil {
		go sendmailLogger.StartRotation(ctx.Done())
	}
	go discord.StartHourlySummary(cfg, ctx.Done())
	go discord.StartSender(cfg, msgCh)
	go discord.StartPickupSender(cfg, pickupCh)
	go discord.StartCraftSender(cfg, craftCh)
	go discord.StartSendmailSender(cfg, sendmailCh)
	go tail.Start(gameLogFile, lineCh, ctx.Done())
	if formatLogFile != "" {
		go tail.Start(formatLogFile, formatLineCh, ctx.Done())
	}

	fmt.Println("Monitoring refines...")

	return runEventLoop(ctx, lineCh, formatLineCh, msgCh, pickupCh, craftCh, sendmailCh, cfg, api, refineLogger, pickupLogger, craftLogger, sendmailLogger, hub, lastSequence)
}

func runEventLoop(
	ctx context.Context,
	lineCh <-chan []byte,
	formatLineCh <-chan []byte,
	msgCh chan<- discord.RefineEvent,
	pickupCh chan<- discord.PickupEvent,
	craftCh chan<- discord.CraftEvent,
	sendmailCh chan<- discord.SendmailEvent,
	cfg *config.Config,
	api *apiClient,
	refineLogger *monitor.Logger,
	pickupLogger *monitor.Logger,
	craftLogger *monitor.Logger,
	sendmailLogger *monitor.Logger,
	hub *webui.Hub,
	lastSequence int64,
) error {
	var sequence int64 = lastSequence

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nShutting down gracefully...")
			close(msgCh)
			close(pickupCh)
			close(craftCh)
			close(sendmailCh)
			time.Sleep(2 * time.Second)
			return nil
		case rawLine, ok := <-lineCh:
			if !ok {
				return nil
			}
			if handleRefineLine(rawLine, &sequence, api, refineLogger, hub, cfg, msgCh) {
				continue
			}
			if handlePickupLine(rawLine, &sequence, api, pickupLogger, hub, cfg, pickupCh) {
				continue
			}
			handleCraftLine(rawLine, &sequence, api, craftLogger, hub, cfg, craftCh)
		case rawLine, ok := <-formatLineCh:
			if !ok {
				return nil
			}
			if handleSendmailLine(rawLine, &sequence, api, sendmailLogger, hub, cfg, sendmailCh) {
				continue
			}
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
