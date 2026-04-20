package pickup

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
	pickupRegex = regexp.MustCompile(`用户(\d+)拣起(\d+)个(\d+)`)

	itemCache        map[string]string
	itemDisplayCache map[string]string
	rareItemSet      map[string]bool
)

type ParsedPickupLine struct {
	RoleID      string
	Count       int
	ItemID      string
	DecodedLine string
	Timestamp   time.Time
}

func Init(tabFilePath, rareItemConfPath string) error {
	itemCache = make(map[string]string)
	itemDisplayCache = make(map[string]string)
	rareItemSet = make(map[string]bool)

	if err := loadItemNames(tabFilePath); err != nil {
		return fmt.Errorf("failed to load item names: %w", err)
	}

	if err := loadRareItems(rareItemConfPath); err != nil {
		return fmt.Errorf("failed to load rare items: %w", err)
	}

	fmt.Printf("[Pickup] Loaded %d items from TAB file, %d rare items from config\n",
		len(itemCache), len(rareItemSet))
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

func loadRareItems(rareItemConfPath string) error {
	f, err := os.Open(rareItemConfPath)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", rareItemConfPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		itemID := strings.TrimSpace(scanner.Text())
		if itemID != "" && !strings.HasPrefix(itemID, "#") {
			rareItemSet[itemID] = true
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

func IsRareItem(itemID string) bool {
	return rareItemSet[itemID]
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

func IsPickupLine(rawLine []byte) bool {
	if bytes.Contains(rawLine, []byte{0xD3, 0xC3, 0xA7, 0xB7}) &&
		bytes.Contains(rawLine, []byte{0xBC, 0xF0, 0xC6, 0xF0}) {
		return true
	}

	decoded := convertToUTF8(rawLine)
	if pickupRegex.MatchString(decoded) {
		return true
	}

	return pickupRegex.MatchString(string(rawLine))
}

func ParsePickupLine(rawLine []byte) (ParsedPickupLine, bool) {
	if !IsPickupLine(rawLine) {
		return ParsedPickupLine{}, false
	}

	decodedLine := convertToUTF8(rawLine)
	matches := pickupRegex.FindStringSubmatch(decodedLine)
	if matches == nil {
		matches = pickupRegex.FindStringSubmatch(string(rawLine))
	}
	if matches == nil {
		return ParsedPickupLine{}, false
	}

	roleID := matches[1]
	count, _ := strconv.Atoi(matches[2])
	itemID := matches[3]

	if !IsRareItem(itemID) {
		return ParsedPickupLine{}, false
	}

	timestamp, _ := ParseLineTimestamp(rawLine)

	return ParsedPickupLine{
		RoleID:      roleID,
		Count:       count,
		ItemID:      itemID,
		DecodedLine: decodedLine,
		Timestamp:   timestamp,
	}, true
}

func MustAtoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
