package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang-refine/internal/config"
	"golang-refine/internal/monitor"
	"golang-refine/internal/refine"
)

var Client = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
	},
}

type RefineEvent struct {
	Sequence      int64
	ObservedAt    time.Time
	RoleID        string
	ItemID        string
	Result        string
	LevelBefore   int
	LevelAfter    int
	StoneID       string
	PlayerName    string
	ItemName      string
	StoneEmoticon string
	IconPath      string
}

func DrainAndClose(resp *http.Response) {
	if resp == nil {
		return
	}
	io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close()
}

func SendEmbed(webhookURL, footer, playerName, result, itemID, itemName string, levelBefore, levelAfter int, stoneID, stoneEmoticon, iconPath string) (int, float64, error) {
	var color int
	switch result {
	case "SUCCESS":
		color = 0x00FF00
	case "FAILURE":
		color = 0xFF0000
	case "RESET":
		color = 0xFFA500
	case "DOWNGRADED":
		color = 0xFF4500
	default:
		color = 0x808080
	}

	levelText := fmt.Sprintf("+%d \u2192 +%d", levelBefore, levelAfter)
	authorName := refine.GetItemDisplayName(itemID)
	if levelAfter > 0 {
		authorName = fmt.Sprintf("%s +%d", authorName, levelAfter)
	}

	embed := map[string]interface{}{
		"color":  color,
		"author": map[string]interface{}{"name": authorName},
		"description": fmt.Sprintf("**%s** refine %s from %s using %s %s",
			playerName, strings.ToLower(result), levelText, refine.GetStoneName(stoneID), stoneEmoticon),
		"footer":    map[string]interface{}{"text": footer},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	payload := map[string]interface{}{"embeds": []map[string]interface{}{embed}}
	jsonData, _ := json.Marshal(payload)

	if iconPath == "" {
		resp, err := Client.Post(webhookURL, "application/json", bytes.NewReader(jsonData))
		if err != nil {
			return 0, 0, fmt.Errorf("error sending embed to Discord: %w", err)
		}
		defer DrainAndClose(resp)
		if resp.StatusCode == 429 {
			retryAfter, _ := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64)
			return 429, retryAfter, fmt.Errorf("discord rate limited 429")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return resp.StatusCode, 0, fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
		}
		return resp.StatusCode, 0, nil
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	filename := filepath.Base(iconPath)
	embed["author"].(map[string]interface{})["icon_url"] = "attachment://" + filename
	jsonData, _ = json.Marshal(payload)

	jsonPart, _ := writer.CreateFormField("payload_json")
	jsonPart.Write(jsonData)

	filePart, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return 0, 0, fmt.Errorf("error creating form file: %w", err)
	}

	f, err := os.Open(iconPath)
	if err != nil {
		return 0, 0, fmt.Errorf("error opening icon file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(filePart, f); err != nil {
		return 0, 0, fmt.Errorf("error copying icon file: %w", err)
	}
	writer.Close()

	resp, err := Client.Post(webhookURL, writer.FormDataContentType(), &buf)
	if err != nil {
		return 0, 0, fmt.Errorf("error sending embed to Discord: %w", err)
	}
	defer DrainAndClose(resp)
	if resp.StatusCode == 429 {
		retryAfter, _ := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64)
		return 429, retryAfter, fmt.Errorf("discord rate limited 429")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, 0, fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}
	return resp.StatusCode, 0, nil
}

func sourceFileForDiscordError(err error, event RefineEvent, logFile string) string {
	msg := err.Error()
	if strings.Contains(msg, "form file") || strings.Contains(msg, "icon file") {
		if event.IconPath != "" {
			return event.IconPath
		}
	}
	if strings.TrimSpace(logFile) != "" {
		return strings.TrimSpace(logFile)
	}
	return monitor.LogPath
}

func BuildRefineEvent(sequence int64, observedAt time.Time, roleID, itemID, result string, levelBefore int, stoneID string, lookupRole func(int) (string, error)) RefineEvent {
	levelAfter := refine.CalculateLevelAfter(result, levelBefore)

	playerName := strings.TrimSpace(roleID)
	if playerName == "" {
		playerName = "Unknown Player"
	}
	if name, err := lookupRole(refine.MustAtoi(roleID)); err == nil && name != "" {
		playerName = name
	}

	itemName := refine.GetItemDisplayName(itemID)
	materialDisplay := refine.GetStoneEmoticon(stoneID)
	iconPath := refine.GetItemIconPath(itemID)

	event := RefineEvent{
		Sequence:      sequence,
		ObservedAt:    observedAt,
		RoleID:        roleID,
		ItemID:        itemID,
		Result:        result,
		LevelBefore:   levelBefore,
		LevelAfter:    levelAfter,
		StoneID:       stoneID,
		PlayerName:    playerName,
		ItemName:      itemName,
		StoneEmoticon: materialDisplay,
		IconPath:      iconPath,
	}

	return event
}

func ProcessRefineEvent(event RefineEvent, cfg *config.Config, msgCh chan<- RefineEvent) bool {
	if !cfg.Discord.ShouldSend(event.Result, event.LevelBefore, event.LevelAfter) {
		return false
	}

	fmt.Printf("[SEND] %s **%s** %s **%s** +%d\u2192+%d %s\n",
		refine.ResultEmojis[event.Result], event.PlayerName, event.Result, event.ItemName, event.LevelBefore, event.LevelAfter, event.StoneEmoticon)
	msgCh <- event
	return true
}

func StartSender(cfg *config.Config, msgCh <-chan RefineEvent) {
	const maxRateLimitWait = 60 * time.Second

	for event := range msgCh {
		webhook := cfg.Discord.GetWebhook(event.Result)
		attempt := 1
		for {
			statusCode, retryAfter, err := SendEmbed(
				webhook, cfg.Discord.Footer,
				event.PlayerName, event.Result, event.ItemID, event.ItemName,
				event.LevelBefore, event.LevelAfter,
				event.StoneID, event.StoneEmoticon, event.IconPath,
			)
			if err == nil {
				if attempt > 1 {
					fmt.Printf("[OK] %s sent to Discord after %d attempts\n", event.Result, attempt)
				} else {
					fmt.Printf("[OK] %s sent to Discord\n", event.Result)
				}
				break
			}

			if statusCode == 429 {
				wait := 2 * time.Second
				if retryAfter > 0 {
					wait = time.Duration(retryAfter * float64(time.Second))
				}
				if wait > maxRateLimitWait {
					wait = maxRateLimitWait
				}
				fmt.Printf("Rate limited, waiting %.1fs (attempt %d)\n", wait.Seconds(), attempt)
				time.Sleep(wait)
				attempt++
				continue
			}

			fmt.Printf("[ERROR] %v\n", err)
			RecordError(err.Error(), sourceFileForDiscordError(err, event, cfg.LogFile))
			break
		}
	}
}
