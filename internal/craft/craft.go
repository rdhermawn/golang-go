package craft

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
	craftRegex = regexp.MustCompile(`锟矫伙拷(\d+)锟斤拷锟斤拷锟斤拷.*?(\d+)锟斤拷(\d+),\s*锟戒方\d+,\s*锟斤拷锟斤拷锟斤拷锟斤拷(\d+),\s*锟斤拷锟斤拷(\d+)`)

	itemCache        map[string]string
	itemDisplayCache map[string]string
	itemLookupCache  map[string]string
	itemIDLookup     map[string]string
	craftItemSet     map[string]bool
)

type ParsedCraftLine struct {
	RoleID         string
	CraftCount     int
	CraftedItemID  string
	MaterialItemID string
	MaterialCount  int
	DecodedLine    string
	Timestamp      time.Time
}

func Init(tabFilePath, craftItemConfPath string) error {
	itemCache = make(map[string]string)
	itemDisplayCache = make(map[string]string)
	itemLookupCache = make(map[string]string)
	itemIDLookup = make(map[string]string)
	craftItemSet = make(map[string]bool)

	if err := loadItemNames(tabFilePath); err != nil {
		return fmt.Errorf("failed to load item names: %w", err)
	}

	if err := loadCraftItems(craftItemConfPath); err != nil {
		return fmt.Errorf("failed to load craft items: %w", err)
	}

	fmt.Printf("[Craft] Loaded %d items from TAB file, %d craft items from config\n",
		len(itemCache), len(craftItemSet))
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
			registerItemLookup(itemID, name, displayName)
		}
	}
	return scanner.Err()
}

func registerItemLookup(itemID, name, displayName string) {
	key := normalizeLookupKey(name)
	if key == "" {
		return
	}
	if _, exists := itemLookupCache[key]; exists {
		if _, hasID := itemIDLookup[key]; !hasID {
			itemIDLookup[key] = itemID
		}
		return
	}
	itemLookupCache[key] = displayName
	itemIDLookup[key] = itemID
}

func normalizeLookupKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func LookupItemIDByName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	return itemIDLookup[normalizeLookupKey(trimmed)]
}

func NormalizeItemDisplayName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return trimmed
	}
	if display, ok := itemLookupCache[normalizeLookupKey(trimmed)]; ok {
		return display
	}
	return trimmed
}

func loadCraftItems(craftItemConfPath string) error {
	f, err := os.Open(craftItemConfPath)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", craftItemConfPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		itemID := strings.TrimSpace(scanner.Text())
		if itemID != "" && !strings.HasPrefix(itemID, "#") {
			craftItemSet[itemID] = true
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

func IsCraftItem(itemID string) bool {
	return craftItemSet[itemID]
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

func IsCraftLine(rawLine []byte) bool {
	craftFingerprints := [][]byte{
		{0xef, 0xbf, 0xbd, 0xc3, 0xbb, 0xef, 0xbf, 0xbd},
	}

	for _, fp := range craftFingerprints {
		if bytes.Contains(rawLine, fp) {
			if bytes.Contains(rawLine, []byte{0xef, 0xbf, 0xbd, 0xe4, 0xb7, 0xbd}) {
				return true
			}
		}
	}

	decoded := convertToUTF8(rawLine)
	if craftRegex.MatchString(decoded) {
		return true
	}

	return false
}

func ParseCraftLine(rawLine []byte) (ParsedCraftLine, bool) {
	if !IsCraftLine(rawLine) {
		return ParsedCraftLine{}, false
	}

	decodedLine := convertToUTF8(rawLine)
	matches := craftRegex.FindStringSubmatch(decodedLine)
	if matches == nil {
		return ParsedCraftLine{}, false
	}

	roleID := matches[1]
	craftCount, _ := strconv.Atoi(matches[2])
	craftedItemID := matches[3]
	materialItemID := matches[4]
	materialCount, _ := strconv.Atoi(matches[5])

	if !IsCraftItem(craftedItemID) {
		return ParsedCraftLine{}, false
	}

	timestamp, _ := ParseLineTimestamp(rawLine)

	return ParsedCraftLine{
		RoleID:         roleID,
		CraftCount:     craftCount,
		CraftedItemID:  craftedItemID,
		MaterialItemID: materialItemID,
		MaterialCount:  materialCount,
		DecodedLine:    decodedLine,
		Timestamp:      timestamp,
	}, true
}

func MustAtoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
