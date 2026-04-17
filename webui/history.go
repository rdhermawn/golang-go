package webui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang-refine/internal/refine"
)

const monitorTimeLayout = "2006-01-02 15:04:05"

func (h *Hub) LoadMonitorHistory(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	sequence := int64(0)
	events := make([]FeedEvent, 0, h.size)

	for scanner.Scan() {
		event, ok := ParseMonitorLogLine(sequence+1, scanner.Text())
		if !ok {
			continue
		}
		sequence++
		event.Sequence = sequence
		event.ID = fmt.Sprintf("%d-history", event.Sequence)
		events = append(events, event)
		if len(events) > h.size {
			events = append([]FeedEvent(nil), events[len(events)-h.size:]...)
		}
	}
	if err := scanner.Err(); err != nil {
		return sequence, err
	}

	h.SetRecent(events)
	return sequence, nil
}

func ParseMonitorLogLine(sequence int64, line string) (FeedEvent, bool) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "[") {
		return FeedEvent{}, false
	}

	timestampEnd := strings.Index(line, "]")
	if timestampEnd <= 1 {
		return FeedEvent{}, false
	}
	timestampText := line[1:timestampEnd]
	timestamp, err := time.ParseInLocation(monitorTimeLayout, timestampText, time.Local)
	if err != nil {
		return FeedEvent{}, false
	}

	remainder := strings.TrimSpace(line[timestampEnd+1:])
	if !strings.HasPrefix(remainder, "[") {
		return FeedEvent{}, false
	}
	statusEnd := strings.Index(remainder, "]")
	if statusEnd <= 1 {
		return FeedEvent{}, false
	}
	status := remainder[1:statusEnd]
	if status != "OK" {
		return FeedEvent{}, false
	}

	message := strings.TrimSpace(remainder[statusEnd+1:])
	roleID, playerName, result, itemName, levelBefore, levelAfter, stoneName, ok := parseMonitorMessage(message)
	if !ok {
		return FeedEvent{}, false
	}
	itemID := refine.LookupItemIDByName(itemName)
	iconURL := ""
	if itemID != "" && refine.GetItemIconPath(itemID) != "" {
		iconURL = "/api/icons/" + itemID + ".png"
	}

	return FeedEvent{
		ID:          fmt.Sprintf("%d-history", sequence),
		Sequence:    sequence,
		Timestamp:   timestamp.UTC().Format(time.RFC3339),
		Status:      status,
		RoleID:      roleID,
		PlayerName:  playerName,
		ItemID:      itemID,
		ItemName:    itemName,
		IconURL:     iconURL,
		Result:      strings.ToUpper(result),
		LevelBefore: levelBefore,
		LevelAfter:  levelAfter,
		StoneName:   stoneName,
		Message:     message,
		StreakKey:   buildStreakKey(roleID, itemID, playerName, itemName),
	}, true
}

func parseMonitorMessage(message string) (string, string, string, string, int, int, string, bool) {
	refinedIndex := strings.Index(message, " refined ")
	if refinedIndex <= 0 {
		return "", "", "", "", 0, 0, "", false
	}

	roleID, playerName := parseMonitorActor(message[:refinedIndex])
	remainder := message[refinedIndex+len(" refined "):]

	var result string
	for _, candidate := range []string{"success", "failure", "reset", "downgraded"} {
		prefix := candidate + " "
		if strings.HasPrefix(remainder, prefix) {
			result = candidate
			remainder = remainder[len(prefix):]
			break
		}
	}
	if result == "" {
		return "", "", "", "", 0, 0, "", false
	}

	fromIndex := strings.LastIndex(remainder, " from +")
	if fromIndex <= 0 {
		return "", "", "", "", 0, 0, "", false
	}
	itemName := strings.TrimSpace(remainder[:fromIndex])
	itemName = refine.NormalizeItemDisplayName(itemName)
	levelSection := strings.TrimSpace(remainder[fromIndex+len(" from +"):])

	stoneName := ""
	usingIndex := strings.LastIndex(levelSection, " using ")
	if usingIndex >= 0 {
		stoneName = strings.TrimSpace(levelSection[usingIndex+len(" using "):])
		levelSection = strings.TrimSpace(levelSection[:usingIndex])
	}

	levels := strings.Split(levelSection, " -> +")
	if len(levels) != 2 {
		return "", "", "", "", 0, 0, "", false
	}

	levelBefore, err := strconv.Atoi(strings.TrimSpace(levels[0]))
	if err != nil {
		return "", "", "", "", 0, 0, "", false
	}
	levelAfter, err := strconv.Atoi(strings.TrimSpace(levels[1]))
	if err != nil {
		return "", "", "", "", 0, 0, "", false
	}

	return roleID, playerName, result, itemName, levelBefore, levelAfter, stoneName, true
}

func parseMonitorActor(actor string) (string, string) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "", ""
	}

	if !strings.HasSuffix(actor, ")") {
		return "", actor
	}

	markerIndex := -1
	markerLength := 0
	for _, marker := range []string{" (role ", " ("} {
		if idx := strings.LastIndex(actor, marker); idx > 0 {
			markerIndex = idx
			markerLength = len(marker)
			break
		}
	}
	if markerIndex <= 0 || markerLength == 0 {
		return "", actor
	}

	roleID := strings.TrimSpace(actor[markerIndex+markerLength : len(actor)-1])
	if roleID == "" {
		return "", actor
	}
	for _, ch := range roleID {
		if ch < '0' || ch > '9' {
			return "", actor
		}
	}

	playerName := strings.TrimSpace(actor[:markerIndex])
	if playerName == "" {
		return "", actor
	}

	return roleID, playerName
}
