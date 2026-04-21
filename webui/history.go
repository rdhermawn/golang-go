package webui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang-refine/internal/craft"
	"golang-refine/internal/refine"
)

func (h *Hub) LoadAllLogsFromDir(dir string, parser func(int64, string) (FeedEvent, bool)) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var txtFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".txt") {
			txtFiles = append(txtFiles, filepath.Join(dir, name))
		}
	}

	sort.Strings(txtFiles)

	var sequence int64
	for _, path := range txtFiles {
		seq, err := h.loadHistoryFromPath(path, parser, sequence)
		if err != nil {
			return sequence, err
		}
		sequence = seq
	}
	return sequence, nil
}

const monitorTimeLayout = "2006-01-02 15:04:05"

func (h *Hub) LoadRefineHistory(path string) (int64, error) {
	return h.loadHistoryFromPath(path, ParseRefineLogLine, 0)
}

func (h *Hub) LoadPickupHistory(path string, baseSequence int64) (int64, error) {
	return h.loadHistoryFromPath(path, ParsePickupLogLine, baseSequence)
}

func (h *Hub) LoadCraftHistory(path string, baseSequence int64) (int64, error) {
	return h.loadHistoryFromPath(path, ParseCraftLogLine, baseSequence)
}

func (h *Hub) LoadSendmailHistory(path string, baseSequence int64) (int64, error) {
	return h.loadHistoryFromPath(path, ParseSendmailLogLine, baseSequence)
}

func (h *Hub) LoadMonitorHistory(path string) (int64, error) {
	return h.LoadRefineHistory(path)
}

func (h *Hub) loadHistoryFromPath(path string, parser func(int64, string) (FeedEvent, bool), baseSequence int64) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return baseSequence, nil
		}
		return baseSequence, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	sequence := baseSequence
	events := make([]FeedEvent, 0, h.size)

	for scanner.Scan() {
		event, ok := parser(sequence+1, scanner.Text())
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

	h.mergeEvents(events)
	return sequence, nil
}

func loadAllEventsFromDir(dir string, parser func(int64, string) (FeedEvent, bool)) ([]FeedEvent, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var txtFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".txt") {
			txtFiles = append(txtFiles, filepath.Join(dir, name))
		}
	}

	sort.Strings(txtFiles)

	var sequence int64
	var allEvents []FeedEvent

	for _, path := range txtFiles {
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			event, ok := parser(sequence+1, scanner.Text())
			if !ok {
				continue
			}
			sequence++
			event.Sequence = sequence
			event.ID = fmt.Sprintf("%d-history", event.Sequence)
			allEvents = append(allEvents, event)
		}
		f.Close()

		if err := scanner.Err(); err != nil {
			return allEvents, err
		}
	}

	return allEvents, nil
}

func ParseRefineLogLine(sequence int64, line string) (FeedEvent, bool) {
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

	rawMessage := strings.TrimSpace(remainder[statusEnd+1:])
	roleID, playerName, result, itemName, levelBefore, levelAfter, stoneName, ok := parseMonitorMessage(rawMessage)
	if !ok {
		return FeedEvent{}, false
	}
	itemID := refine.LookupItemIDByName(itemName)
	iconURL := ""
	if itemID != "" && refine.GetItemIconPath(itemID) != "" {
		iconURL = "/api/icons/" + itemID + ".png"
	}

	// Build new format message without result word (success/failure/reset/downgraded)
	actor := strings.TrimSpace(playerName)
	if roleID != "" && actor != roleID {
		actor = fmt.Sprintf("%s (%s)", actor, roleID)
	}
	message := fmt.Sprintf("%s refined %s from +%d -> +%d", actor, itemName, levelBefore, levelAfter)
	if stoneName != "" {
		message = fmt.Sprintf("%s using %s", message, stoneName)
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

func ParsePickupLogLine(sequence int64, line string) (FeedEvent, bool) {
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
	if status != "PICKUP" {
		return FeedEvent{}, false
	}

	message := strings.TrimSpace(remainder[statusEnd+1:])
	roleID, playerName, itemName, count, ok := parsePickupMessage(message)
	if !ok {
		return FeedEvent{}, false
	}
	itemID := refine.LookupItemIDByName(itemName)
	iconURL := ""
	if itemID != "" && refine.GetItemIconPath(itemID) != "" {
		iconURL = "/api/icons/" + itemID + ".png"
	}

	return FeedEvent{
		ID:         fmt.Sprintf("%d-history", sequence),
		Sequence:   sequence,
		Timestamp:  timestamp.UTC().Format(time.RFC3339),
		Status:     status,
		RoleID:     roleID,
		PlayerName: playerName,
		ItemID:     itemID,
		ItemName:   itemName,
		IconURL:    iconURL,
		Result:     "PICKUP",
		Count:      count,
		Message:    fmt.Sprintf("%s acquired %d x %s", playerName, count, itemName),
		StreakKey:  buildStreakKey(roleID, itemID, playerName, itemName),
	}, true
}

func ParseMonitorLogLine(sequence int64, line string) (FeedEvent, bool) {
	return ParseRefineLogLine(sequence, line)
}

func ParseCraftLogLine(sequence int64, line string) (FeedEvent, bool) {
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
	if status != "CRAFT" {
		return FeedEvent{}, false
	}

	message := strings.TrimSpace(remainder[statusEnd+1:])
	roleID, playerName, craftedItemName, craftCount, _, _, ok := parseCraftMessage(message)
	if !ok {
		return FeedEvent{}, false
	}
	itemID := refine.LookupItemIDByName(craftedItemName)
	iconURL := ""
	if itemID != "" && refine.GetItemIconPath(itemID) != "" {
		iconURL = "/api/icons/" + itemID + ".png"
	}

	return FeedEvent{
		ID:         fmt.Sprintf("%d-history", sequence),
		Sequence:   sequence,
		Timestamp:  timestamp.UTC().Format(time.RFC3339),
		Status:     status,
		RoleID:     roleID,
		PlayerName: playerName,
		ItemID:     itemID,
		ItemName:   craftedItemName,
		IconURL:    iconURL,
		Result:     "CRAFT",
		Count:      craftCount,
		Message:    fmt.Sprintf("%s.png %s x%d manufactured by %s", itemID, craftedItemName, craftCount, playerName),
		StreakKey:  buildStreakKey(roleID, itemID, playerName, craftedItemName),
	}, true
}

func parsePickupMessage(message string) (string, string, string, int, bool) {
	// Format: "[PICKUP] PlayerName picked up Count x ItemName"
	pickedUpIndex := strings.Index(message, " picked up ")
	if pickedUpIndex <= 0 {
		return "", "", "", 0, false
	}

	playerName := strings.TrimSpace(message[:pickedUpIndex])
	remainder := message[pickedUpIndex+len(" picked up "):]

	// Parse "Count x ItemName"
	xIndex := strings.Index(remainder, " x ")
	if xIndex <= 0 {
		return "", playerName, "", 0, false
	}

	countStr := strings.TrimSpace(remainder[:xIndex])
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return "", playerName, "", 0, false
	}

	itemName := strings.TrimSpace(remainder[xIndex+len(" x "):])
	itemName = refine.NormalizeItemDisplayName(itemName)

	// Try to extract roleID from playerName if it contains " (role ID)"
	roleID := ""
	if idx := strings.LastIndex(playerName, " ("); idx > 0 {
		if strings.HasSuffix(playerName, ")") {
			possibleRole := playerName[idx+2 : len(playerName)-1]
			if strings.HasPrefix(possibleRole, "role ") {
				roleID = strings.TrimPrefix(possibleRole, "role ")
				playerName = strings.TrimSpace(playerName[:idx])
			}
		}
	}

	return roleID, playerName, itemName, count, true
}

func parseCraftMessage(message string) (string, string, string, int, string, int, bool) {
	// Format: "CraftedItemName xCount manufactured by PlayerName"
	// Or: "Icon item CraftedItemName xCount manufactured by RoleID (PlayerName)"
	// Or legacy: "PlayerName crafted Count x CraftedItemName (used MaterialCount x MaterialName)"
	// Or legacy short: "PlayerName crafted Count x CraftedItemName"

	if strings.Contains(message, " manufactured by ") {
		return parseNewCraftMessage(message)
	}
	return parseLegacyCraftMessage(message)
}

func parseNewCraftMessage(message string) (string, string, string, int, string, int, bool) {
	// "CraftedItemName xCount manufactured by PlayerName"
	// Or: "Icon item CraftedItemName xCount manufactured by RoleID (PlayerName)"
	manufacturedIndex := strings.Index(message, " manufactured by ")
	if manufacturedIndex <= 0 {
		return "", "", "", 0, "", 0, false
	}

	prefix := strings.TrimSpace(message[:manufacturedIndex])
	remainder := strings.TrimSpace(message[manufacturedIndex+len(" manufactured by "):])

	// Strip "Icon item " prefix if present
	prefix = strings.TrimPrefix(prefix, "Icon item ")

	// Parse "CraftedItemName xCount"
	xIndex := strings.LastIndex(prefix, " x")
	if xIndex <= 0 {
		return "", "", "", 0, "", 0, false
	}

	craftedItemName := strings.TrimSpace(prefix[:xIndex])
	craftedItemName = craft.NormalizeItemDisplayName(craftedItemName)
	countStr := strings.TrimSpace(prefix[xIndex+len(" x"):])
	craftCount, err := strconv.Atoi(countStr)
	if err != nil {
		return "", "", "", 0, "", 0, false
	}

	// Parse "RoleID (PlayerName)" or just "PlayerName"
	roleID := ""
	playerName := remainder
	if idx := strings.LastIndex(remainder, " ("); idx > 0 {
		if strings.HasSuffix(remainder, ")") {
			roleID = strings.TrimSpace(remainder[:idx])
			playerName = strings.TrimSpace(remainder[idx+2 : len(remainder)-1])
		}
	}

	return roleID, playerName, craftedItemName, craftCount, "", 0, true
}

func parseLegacyCraftMessage(message string) (string, string, string, int, string, int, bool) {
	// Format: "PlayerName crafted Count x CraftedItemName (used MaterialCount x MaterialName)"
	// Or: "PlayerName crafted Count x CraftedItemName"
	craftedIndex := strings.Index(message, " crafted ")
	if craftedIndex <= 0 {
		return "", "", "", 0, "", 0, false
	}

	playerName := strings.TrimSpace(message[:craftedIndex])
	remainder := message[craftedIndex+len(" crafted "):]

	// Parse "Count x CraftedItemName" with optional "(used MaterialCount x MaterialName)"
	xIndex := strings.Index(remainder, " x ")
	if xIndex <= 0 {
		return "", playerName, "", 0, "", 0, false
	}

	countStr := strings.TrimSpace(remainder[:xIndex])
	craftCount, err := strconv.Atoi(countStr)
	if err != nil {
		return "", playerName, "", 0, "", 0, false
	}

	usedIndex := strings.Index(remainder, " (used ")
	if usedIndex > 0 {
		craftedItemName := strings.TrimSpace(remainder[xIndex+len(" x "):usedIndex])
		craftedItemName = craft.NormalizeItemDisplayName(craftedItemName)
		materialSection := remainder[usedIndex+len(" (used "):]

		matXIndex := strings.Index(materialSection, " x ")
		if matXIndex <= 0 {
			return "", playerName, "", 0, "", 0, false
		}

		matCountStr := strings.TrimSpace(materialSection[:matXIndex])
		materialCount, err := strconv.Atoi(matCountStr)
		if err != nil {
			return "", playerName, "", 0, "", 0, false
		}

		materialName := strings.TrimSpace(materialSection[matXIndex+len(" x "):])
		materialName = strings.TrimSuffix(materialName, ")")
		materialName = craft.NormalizeItemDisplayName(materialName)

		roleID := ""
		if idx := strings.LastIndex(playerName, " ("); idx > 0 {
			if strings.HasSuffix(playerName, ")") {
				possibleRole := playerName[idx+2 : len(playerName)-1]
				if strings.HasPrefix(possibleRole, "role ") {
					roleID = strings.TrimPrefix(possibleRole, "role ")
					playerName = strings.TrimSpace(playerName[:idx])
				}
			}
		}

		return roleID, playerName, craftedItemName, craftCount, materialName, materialCount, true
	}

	craftedItemName := strings.TrimSpace(remainder[xIndex+len(" x "):])
	craftedItemName = craft.NormalizeItemDisplayName(craftedItemName)

	roleID := ""
	if idx := strings.LastIndex(playerName, " ("); idx > 0 {
		if strings.HasSuffix(playerName, ")") {
			possibleRole := playerName[idx+2 : len(playerName)-1]
			if strings.HasPrefix(possibleRole, "role ") {
				roleID = strings.TrimPrefix(possibleRole, "role ")
				playerName = strings.TrimSpace(playerName[:idx])
			}
		}
	}

	return roleID, playerName, craftedItemName, craftCount, "", 0, true
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
	if result == "" {
		result = inferRefineResult(levelBefore, levelAfter)
	}

	return roleID, playerName, result, itemName, levelBefore, levelAfter, stoneName, true
}

func inferRefineResult(levelBefore, levelAfter int) string {
	switch {
	case levelAfter > levelBefore:
		return "success"
	case levelAfter == 0 && levelBefore > 0:
		return "reset"
	case levelAfter < levelBefore:
		return "downgraded"
	default:
		return "failure"
	}
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

func ParseSendmailLogLine(sequence int64, line string) (FeedEvent, bool) {
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
	if status != "SENDMAIL" {
		return FeedEvent{}, false
	}

	rawMessage := strings.TrimSpace(remainder[statusEnd+1:])
	srcRoleID, dstRoleID, srcPlayerName, dstPlayerName, itemName, money, count, ok := parseSendmailMessage(rawMessage)
	if !ok {
		return FeedEvent{}, false
	}
	if money == 0 {
		return FeedEvent{}, false
	}
	itemID := refine.LookupItemIDByName(itemName)
	iconURL := ""
	if itemID != "" && refine.GetItemIconPath(itemID) != "" {
		iconURL = "/api/icons/" + itemID + ".png"
	}

	// Build new format message without "sent"
	var message string
	if money > 0 && itemID != "" && itemID != "0" && count > 0 {
		message = fmt.Sprintf("%s -> %s: %d gold + %d x %s", srcPlayerName, dstPlayerName, money, count, itemName)
	} else if money > 0 {
		message = fmt.Sprintf("%s -> %s: %d silver", srcPlayerName, dstPlayerName, money)
	} else if itemID != "" && itemID != "0" && count > 0 {
		message = fmt.Sprintf("%s -> %s: %d x %s", srcPlayerName, dstPlayerName, count, itemName)
	} else {
		message = fmt.Sprintf("%s -> %s: mail", srcPlayerName, dstPlayerName)
	}

	return FeedEvent{
		ID:            fmt.Sprintf("%d-history", sequence),
		Sequence:      sequence,
		Timestamp:     timestamp.UTC().Format(time.RFC3339),
		Status:        status,
		RoleID:        srcRoleID,
		PlayerName:    srcPlayerName,
		ItemID:        itemID,
		ItemName:      itemName,
		IconURL:       iconURL,
		Result:        "SENDMAIL",
		Count:         count,
		Message:       message,
		StreakKey:     buildStreakKey(srcRoleID, itemID, srcPlayerName, itemName),
		SrcRoleID:     srcRoleID,
		DstRoleID:     dstRoleID,
		SrcPlayerName: srcPlayerName,
		DstPlayerName: dstPlayerName,
		Money:         money,
	}, true
}

func parseSendmailMessage(message string) (string, string, string, string, string, int64, int, bool) {
	srcRoleID := ""
	dstRoleID := ""
	srcPlayerName := ""
	dstPlayerName := ""
	itemName := ""
	money := int64(0)
	count := 0

	sentIndex := strings.Index(message, " sent ")
	if sentIndex <= 0 {
		return "", "", "", "", "", 0, 0, false
	}

	actor := strings.TrimSpace(message[:sentIndex])
	remainder := message[sentIndex+len(" sent "):]

	srcRoleID, srcPlayerName = parseMonitorActor(actor)
	if srcPlayerName == "" {
		srcPlayerName = actor
	}

	toIndex := strings.LastIndex(remainder, " to ")
	if toIndex <= 0 {
		return "", "", srcPlayerName, "", "", 0, 0, false
	}

	content := strings.TrimSpace(remainder[:toIndex])
	dst := strings.TrimSpace(remainder[toIndex+len(" to "):])

	dstRoleID, dstPlayerName = parseMonitorActor(dst)
	if dstPlayerName == "" {
		dstPlayerName = dst
	}

	if strings.Contains(content, " gold + ") && strings.Contains(content, " x ") {
		parts := strings.Split(content, " gold + ")
		if len(parts) == 2 {
			moneyStr := strings.TrimSpace(parts[0])
			money, _ = strconv.ParseInt(moneyStr, 10, 64)

			itemPart := strings.TrimSpace(parts[1])
			xIdx := strings.Index(itemPart, " x ")
			if xIdx > 0 {
				countStr := strings.TrimSpace(itemPart[:xIdx])
				count, _ = strconv.Atoi(countStr)
				itemName = strings.TrimSpace(itemPart[xIdx+len(" x "):])
				itemName = refine.NormalizeItemDisplayName(itemName)
			}
		}
	} else if strings.Contains(content, " gold ") && !strings.Contains(content, " x ") {
		moneyStr := strings.TrimSuffix(content, " gold")
		moneyStr = strings.TrimSpace(moneyStr)
		money, _ = strconv.ParseInt(moneyStr, 10, 64)
	} else if strings.Contains(content, " x ") {
		xIdx := strings.Index(content, " x ")
		if xIdx > 0 {
			countStr := strings.TrimSpace(content[:xIdx])
			count, _ = strconv.Atoi(countStr)
			itemName = strings.TrimSpace(content[xIdx+len(" x "):])
			itemName = refine.NormalizeItemDisplayName(itemName)
		}
	}

	return srcRoleID, dstRoleID, srcPlayerName, dstPlayerName, itemName, money, count, true
}
