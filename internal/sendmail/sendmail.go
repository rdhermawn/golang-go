package sendmail

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var (
	sendmailRegex = regexp.MustCompile(`formatlog:sendmail:timestamp=(\d+):src=(\d+):dst=(\d+):mid=(\d+):size=(\d+):money=(-?\d+):item=(\d+):count=(\d+):pos=(\d+)`)

	itemCache        map[string]string
	itemDisplayCache map[string]string
)

type ParsedSendmailLine struct {
	Timestamp   time.Time
	SrcRoleID   string
	DstRoleID   string
	Money       int64
	ItemID      string
	ItemCount   int
	DecodedLine string
}

func Init(tabFilePath string) error {
	itemCache = make(map[string]string)
	itemDisplayCache = make(map[string]string)

	if err := loadItemNames(tabFilePath); err != nil {
		return fmt.Errorf("failed to load item names: %w", err)
	}

	fmt.Printf("[Sendmail] Loaded %d items from TAB file\n", len(itemCache))
	return nil
}

func loadItemNames(tabFilePath string) error {
	f, err := os.Open(tabFilePath)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", tabFilePath, err)
	}
	defer f.Close()

	stripStarsRegex := regexp.MustCompile(`^[^\p{L}\p{N}]+`)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		columns := strings.Split(strings.TrimSpace(line), "\t")
		if len(columns) >= 3 {
			itemID := strings.TrimSpace(columns[0])
			displayName := strings.TrimSpace(columns[2])
			name := stripStarsRegex.ReplaceAllString(displayName, "")
			name = strings.TrimSpace(name)
			if name == "" {
				name = displayName
			}
			if displayName == "" {
				displayName = fmt.Sprintf("Item %s", itemID)
			}
			if name == "" {
				name = fmt.Sprintf("Item %s", itemID)
			}
			itemCache[itemID] = name
			itemDisplayCache[itemID] = displayName
		}
	}
	return scanner.Err()
}

func GetItemName(itemID string) string {
	if cached, ok := itemCache[itemID]; ok {
		return cached
	}
	return fmt.Sprintf("Item %s", itemID)
}

func GetItemDisplayName(itemID string) string {
	if cached, ok := itemDisplayCache[itemID]; ok {
		return cached
	}
	return GetItemName(itemID)
}

func GetItemIconPath(itemID string) string {
	iconPath := filepath.Join("./data/Icons/", itemID+".png")
	if _, err := os.Stat(iconPath); err == nil {
		return iconPath
	}
	return ""
}

func convertToUTF8(data []byte) string {
	decoder := simplifiedchinese.GBK.NewDecoder()
	reader := transform.NewReader(bytes.NewReader(data), decoder)
	result, err := io.ReadAll(reader)
	if err != nil {
		return string(data)
	}
	return string(result)
}

func ParseLineTimestamp(rawLine []byte) (time.Time, bool) {
	if len(rawLine) < len("2006-01-02 15:04:05") {
		return time.Time{}, false
	}
	timestampText := string(rawLine[:len("2006-01-02 15:04:05")])
	timestamp, err := time.ParseInLocation("2006-01-02 15:04:05", timestampText, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return timestamp, true
}

func IsSendmailLine(rawLine []byte) bool {
	return bytes.Contains(rawLine, []byte("formatlog:sendmail:"))
}

func ParseSendmailLine(rawLine []byte) (ParsedSendmailLine, bool) {
	if !IsSendmailLine(rawLine) {
		return ParsedSendmailLine{}, false
	}

	decodedLine := convertToUTF8(rawLine)
	matches := sendmailRegex.FindStringSubmatch(decodedLine)
	if matches == nil {
		return ParsedSendmailLine{}, false
	}

	srcRoleID := matches[2]
	dstRoleID := matches[3]
	money, _ := strconv.ParseInt(matches[6], 10, 64)
	itemID := matches[7]
	itemCount, _ := strconv.Atoi(matches[8])

	if money == 0 && (itemID == "0" || itemCount == 0) {
		return ParsedSendmailLine{}, false
	}

	timestamp, _ := ParseLineTimestamp(rawLine)

	return ParsedSendmailLine{
		Timestamp:   timestamp,
		SrcRoleID:   srcRoleID,
		DstRoleID:   dstRoleID,
		Money:       money,
		ItemID:      itemID,
		ItemCount:   itemCount,
		DecodedLine: decodedLine,
	}, true
}

func BuildMonitorMessage(srcPlayerName, dstPlayerName, srcRoleID, dstRoleID string, money int64, itemID string, itemCount int) string {
	actor := strings.TrimSpace(srcPlayerName)
	if actor == "" {
		actor = srcRoleID
	}
	if actor == "" {
		actor = "Unknown Player"
	}
	if srcRoleID != "" && actor != srcRoleID {
		actor = fmt.Sprintf("%s (%s)", actor, srcRoleID)
	}

	dst := strings.TrimSpace(dstPlayerName)
	if dst == "" {
		dst = dstRoleID
	}
	if dst == "" {
		dst = "Unknown Player"
	}
	if dstRoleID != "" && dst != dstRoleID {
		dst = fmt.Sprintf("%s (%s)", dst, dstRoleID)
	}

	if money > 0 && itemID != "0" && itemCount > 0 {
		itemName := GetItemDisplayName(itemID)
		return fmt.Sprintf("%s sent %d gold + %d x %s to %s", actor, money, itemCount, itemName, dst)
	}
	if money > 0 {
		return fmt.Sprintf("%s sent %d gold to %s", actor, money, dst)
	}
	if itemID != "0" && itemCount > 0 {
		itemName := GetItemDisplayName(itemID)
		return fmt.Sprintf("%s sent %d x %s to %s", actor, itemCount, itemName, dst)
	}
	return fmt.Sprintf("%s sent mail to %s", actor, dst)
}

func MustAtoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
