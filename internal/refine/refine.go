package refine

import (
	"bufio"
	"bytes"
	"encoding/hex"
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

const MaxConcurrentRefines = 50

var WorkerSem = make(chan struct{}, MaxConcurrentRefines)

var (
	tabFilePath = "./data/RAE_Exported_Table.tab"
	iconsDir    = "./data/Icons/"

	itemCache        map[string]string
	itemDisplayCache map[string]string
	itemLookupCache  map[string]string
	itemIDLookup     map[string]string
)

var (
	refineRegexClean     = regexp.MustCompile(`用户(\d+)精炼物品(\d+)\[(成功|材料消失|属性爆掉|属性降低一级)\]，精炼前级别(\d+) 消耗幻仙石(\d+) 概率物品(-1|\d+)`)
	refineRegexCorrupted = regexp.MustCompile(`(\d+)\D+?(\d+)\[([^\]]+)\]\D+?(\d+)\D+?(\d+)\D+?(-?\d+)\s*$`)
	stripStarsRegex      = regexp.MustCompile(`^[^\p{L}\p{N}]+`)

	chineseToPortuguese = map[string]string{
		"\u6210\u529f":                         "SUCCESS",
		"\u6750\u6599\u6d88\u5931":             "FAILURE",
		"\u5c5e\u6027\u7206\u6389":             "RESET",
		"\u5c5e\u6027\u964d\u4f4e\u4e00\u7ea7": "DOWNGRADED",
	}

	resultFingerprints = map[string]string{
		"c9b9":     "SUCCESS",
		"caa7":     "FAILURE",
		"d4b1":     "RESET",
		"d4bdd2bb": "DOWNGRADED",
	}

	stoneEmoticons = map[string]string{
		"15049": "<:15049:1444078544522051696>",
		"12751": "<:12751:1397026138760679484>",
		"12750": "<:12750:1397206621804822598>",
		"15692": "<:15692:1465936632321413293>",
		"12980": "<:12980:1397026136353144842>",
		"11208": "<:11208:1465934861842780312>",
		"15693": "<:15693:1465936520413450374>",
		"15048": "<:15048:1465936765239169231>",
		"15047": "<:15047:1465936982747381792>",
		"15042": "<:15042:1472399227710865550>",
	}

	stoneNames = map[string]string{}

	ResultEmojis = map[string]string{
		"SUCCESS":    "\u2705",
		"FAILURE":    "\u274c",
		"RESET":      "\U0001f4a5",
		"DOWNGRADED": "\u2b07\ufe0f",
	}
)

type ParsedRefineLine struct {
	RoleID      string
	ItemID      string
	Result      string
	LevelBefore int
	StoneID     string
	DecodedLine string
}

func Init() {
	itemCache = make(map[string]string)
	itemDisplayCache = make(map[string]string)
	itemLookupCache = make(map[string]string)
	itemIDLookup = make(map[string]string)
	f, err := os.Open(tabFilePath)
	if err != nil {
		fmt.Printf("Warning: cannot open %s: %v\n", tabFilePath, err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		columns := strings.Split(strings.TrimSpace(line), "\t")
		if len(columns) >= 3 {
			displayName := strings.TrimSpace(columns[2])
			name := stripStarsRegex.ReplaceAllString(displayName, "")
			name = strings.TrimSpace(name)
			if name == "" {
				name = displayName
			}
			if displayName == "" {
				displayName = fmt.Sprintf("Item %s", columns[0])
			}
			if name == "" {
				name = fmt.Sprintf("Item %s", columns[0])
			}
			itemCache[columns[0]] = name
			itemDisplayCache[columns[0]] = displayName
			registerItemLookup(columns[0], name, displayName)
			registerItemLookup(columns[0], displayName, displayName)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("Warning: cannot scan %s: %v\n", tabFilePath, err)
	}
	fmt.Printf("Loaded %d items from TAB file\n", len(itemCache))
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

func LookupItemIDByName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	return itemIDLookup[normalizeLookupKey(trimmed)]
}

func GetStoneEmoticon(stoneID string) string {
	if stoneID == "-1" {
		return ""
	}
	if emote, ok := stoneEmoticons[stoneID]; ok {
		return emote
	}
	return fmt.Sprintf(":%s:", stoneID)
}

func GetStoneName(stoneID string) string {
	if stoneID == "" || stoneID == "-1" {
		return ""
	}
	if name, ok := stoneNames[stoneID]; ok {
		return name
	}
	if name, ok := itemCache[stoneID]; ok {
		return name
	}
	return fmt.Sprintf("Material %s", stoneID)
}

func GetItemIconPath(itemID string) string {
	iconPath := filepath.Join(iconsDir, itemID+".png")
	if _, err := os.Stat(iconPath); err == nil {
		return iconPath
	}
	return ""
}

func CalculateLevelAfter(result string, levelBefore int) int {
	if result == "SUCCESS" {
		return levelBefore + 1
	}
	if result == "FAILURE" {
		return levelBefore
	}
	if result == "RESET" {
		return 0
	}
	if result == "DOWNGRADED" {
		return levelBefore - 1
	}
	return levelBefore
}

func BuildRefineLogMessage(playerName, result, itemName string, levelBefore, levelAfter int, stoneID string) string {
	message := fmt.Sprintf("%s refined %s from +%d -> +%d",
		playerName, NormalizeItemDisplayName(itemName), levelBefore, levelAfter)
	stoneName := GetStoneName(stoneID)
	if stoneName == "" {
		return message
	}
	return fmt.Sprintf("%s using %s", message, stoneName)
}

func BuildRefineMonitorLogMessage(roleID, playerName, result, itemName string, levelBefore, levelAfter int, stoneID string) string {
	actor := strings.TrimSpace(playerName)
	roleID = strings.TrimSpace(roleID)
	if actor == "" {
		actor = roleID
	}
	if actor == "" {
		actor = "Unknown Player"
	}
	if roleID != "" && actor != roleID {
		actor = fmt.Sprintf("%s (%s)", actor, roleID)
	}

	return BuildRefineLogMessage(actor, result, itemName, levelBefore, levelAfter, stoneID)
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

func MustAtoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func IsRefineLine(rawLine []byte) bool {
	bracketStart := bytes.IndexByte(rawLine, '[')
	bracketEnd := bytes.IndexByte(rawLine, ']')
	if bracketStart < 0 || bracketEnd <= bracketStart+1 {
		return false
	}
	bracketContent := rawLine[bracketStart+1 : bracketEnd]

	surviving := bytes.ReplaceAll(bracketContent, []byte{0xEF, 0xBF, 0xBD}, nil)
	if len(surviving) > 0 {
		fingerprint := hex.EncodeToString(surviving)
		if _, ok := resultFingerprints[fingerprint]; ok {
			return true
		}
	}

	gbkKeywords := [][]byte{
		{0xb3, 0xc9, 0xb9, 0xa6},
		{0xb2, 0xc4, 0xc1, 0xcf, 0xcf, 0xfb, 0xca, 0xa7},
		{0xca, 0xf4, 0xd0, 0xd4, 0xb1, 0xac, 0xb5, 0xf4},
		{0xca, 0xf4, 0xd0, 0xd4, 0xbd, 0xb5, 0xb5, 0xcd, 0xd2, 0xbb, 0xbc, 0xb6},
	}
	for _, kw := range gbkKeywords {
		if bytes.Contains(bracketContent, kw) {
			return true
		}
	}

	utf8Keywords := [][]byte{
		[]byte("\u6210\u529f"),
		[]byte("\u6750\u6599\u6d88\u5931"),
		[]byte("\u5c5e\u6027\u7206\u6389"),
		[]byte("\u5c5e\u6027\u964d\u4f4e\u4e00\u7ea7"),
	}
	for _, kw := range utf8Keywords {
		if bytes.Contains(bracketContent, kw) {
			return true
		}
	}

	return false
}

func detectResultFromFingerprint(rawLine []byte, bracketStart, bracketEnd int) string {
	bracketContent := rawLine[bracketStart+1 : bracketEnd]
	surviving := bytes.ReplaceAll(bracketContent, []byte{0xEF, 0xBF, 0xBD}, nil)
	fingerprint := hex.EncodeToString(surviving)
	if result, ok := resultFingerprints[fingerprint]; ok {
		return result
	}
	return ""
}

func convertChineseToPortuguese(text string) string {
	for k, v := range chineseToPortuguese {
		text = strings.ReplaceAll(text, k, v)
	}
	return text
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

func parseCleanRefineLine(line string) (ParsedRefineLine, bool) {
	matches := refineRegexClean.FindStringSubmatch(line)
	if matches == nil {
		return ParsedRefineLine{}, false
	}
	level, _ := strconv.Atoi(matches[4])
	stoneID := matches[6]
	if stoneID == "-1" {
		stoneID = "11208"
	}
	return ParsedRefineLine{
		RoleID:      matches[1],
		ItemID:      matches[2],
		Result:      convertChineseToPortuguese(matches[3]),
		LevelBefore: level,
		StoneID:     stoneID,
		DecodedLine: line,
	}, true
}

func parseCorruptedRefineLine(rawLine []byte, decodedLine string) (ParsedRefineLine, bool) {
	bracketStart := bytes.IndexByte(rawLine, '[')
	bracketEnd := bytes.IndexByte(rawLine, ']')
	if bracketStart < 0 || bracketEnd < 0 || bracketEnd <= bracketStart {
		return ParsedRefineLine{}, false
	}
	result := detectResultFromFingerprint(rawLine, bracketStart, bracketEnd)
	if result == "" {
		return ParsedRefineLine{}, false
	}
	matches := refineRegexCorrupted.FindStringSubmatch(string(rawLine))
	if matches == nil {
		return ParsedRefineLine{}, false
	}
	level, _ := strconv.Atoi(matches[4])
	stoneID := matches[6]
	if stoneID == "-1" {
		stoneID = "11208"
	}
	return ParsedRefineLine{
		RoleID:      matches[1],
		ItemID:      matches[2],
		Result:      result,
		LevelBefore: level,
		StoneID:     stoneID,
		DecodedLine: decodedLine,
	}, true
}

func ParseRefineLine(rawLine []byte) (ParsedRefineLine, bool) {
	if !IsRefineLine(rawLine) {
		return ParsedRefineLine{}, false
	}
	decodedLine := convertToUTF8(rawLine)
	if parsed, ok := parseCleanRefineLine(decodedLine); ok {
		return parsed, true
	}
	return parseCorruptedRefineLine(rawLine, decodedLine)
}
