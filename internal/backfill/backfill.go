package backfill

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang-refine/internal/config"
	"golang-refine/internal/discord"
	"golang-refine/internal/refine"
)

const (
	maxLineBytes          = 64 * 1024
	defaultBatchSize      = 2000
	maxBatchPayloadBytes  = 6 * 1024 * 1024
	defaultCheckpointPath = "./logs/backfill-discord.checkpoint.json"
)

type LookupRoleFunc func(roleID int) (string, error)

type Options struct {
	File       string
	MinLevel   int
	MaxLevel   int
	Result     string
	BatchSize  int
	Checkpoint string
	Resume     bool
	Webhook    string
}

type checkpoint struct {
	File      string `json:"file"`
	Offset    int64  `json:"offset"`
	MinLevel  int    `json:"min_level"`
	MaxLevel  int    `json:"max_level"`
	Result    string `json:"result"`
	UpdatedAt string `json:"updated_at"`
}

type batchEntry struct {
	line   string
	offset int64
}

func Run(cfg *config.Config, args []string, lookupRole LookupRoleFunc) int {
	if cfg == nil {
		fmt.Fprintln(os.Stderr, "Missing config")
		return 1
	}

	opts := Options{
		File:       strings.TrimSpace(cfg.LogFile),
		MinLevel:   11,
		MaxLevel:   -1,
		BatchSize:  defaultBatchSize,
		Checkpoint: defaultCheckpointPath,
		Resume:     true,
		Webhook:    cfg.Discord.GetSummaryWebhook(),
	}

	fs := flag.NewFlagSet("backfill-discord", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.File, "file", opts.File, "Path file log refine")
	fs.IntVar(&opts.MinLevel, "min-level", opts.MinLevel, "Filter level minimum")
	fs.IntVar(&opts.MaxLevel, "max-level", opts.MaxLevel, "Filter level maksimum")
	fs.StringVar(&opts.Result, "result", "", "Filter result: SUCCESS, FAILURE, RESET, DOWNGRADED")
	fs.IntVar(&opts.BatchSize, "batch-size", opts.BatchSize, "Jumlah entries per file batch")
	fs.StringVar(&opts.Checkpoint, "checkpoint", opts.Checkpoint, "Path file checkpoint")
	fs.BoolVar(&opts.Resume, "resume", opts.Resume, "Lanjutkan dari checkpoint terakhir")
	fs.StringVar(&opts.Webhook, "webhook", opts.Webhook, "Discord webhook untuk file backfill")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	opts.File = strings.TrimSpace(opts.File)
	opts.Result = strings.ToUpper(strings.TrimSpace(opts.Result))
	opts.Checkpoint = strings.TrimSpace(opts.Checkpoint)
	opts.Webhook = strings.TrimSpace(opts.Webhook)

	if opts.File == "" {
		fmt.Fprintln(os.Stderr, "Missing --file or config log_file")
		return 1
	}
	if opts.BatchSize <= 0 {
		fmt.Fprintln(os.Stderr, "Invalid --batch-size, must be > 0")
		return 1
	}
	if opts.MinLevel < 0 {
		fmt.Fprintln(os.Stderr, "Invalid --min-level, must be >= 0")
		return 1
	}
	if opts.MaxLevel >= 0 && opts.MaxLevel < opts.MinLevel {
		fmt.Fprintln(os.Stderr, "Invalid level range: max-level cannot be lower than min-level")
		return 1
	}
	if opts.Webhook == "" {
		fmt.Fprintln(os.Stderr, "Missing Discord webhook for backfill")
		return 1
	}

	file, err := os.Open(opts.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open backfill file: %v\n", err)
		return 1
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stat backfill file: %v\n", err)
		return 1
	}
	fileSize := fileInfo.Size()

	startOffset := int64(0)
	if opts.Resume {
		startOffset = loadCheckpoint(opts)
		if startOffset < 0 {
			startOffset = 0
		}
		if startOffset > fileSize {
			fmt.Printf("Checkpoint offset %d exceeds file size %d, restarting from 0\n", startOffset, fileSize)
			startOffset = 0
		}
	}
	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seek backfill file: %v\n", err)
		return 1
	}

	readerSize := 64 * 1024
	if cfg.MaxBuffer > readerSize {
		readerSize = cfg.MaxBuffer
	}
	reader := bufio.NewReaderSize(file, readerSize)

	roleCache := map[int]string{}
	currentOffset := startOffset
	processedLines := 0
	matchedEntries := 0
	sentBatches := 0
	skippedLongLines := 0
	var batch []batchEntry

	flush := func(force bool) error {
		if len(batch) == 0 {
			return nil
		}
		if !force && len(batch) < opts.BatchSize && batchTotalBytes(batch) < maxBatchPayloadBytes {
			return nil
		}

		batchNumber := sentBatches + 1
		payload := buildBatchPayload(batch)
		start := batch[0].offset - int64(len(batch[0].line))
		end := batch[len(batch)-1].offset
		filename := fmt.Sprintf("backfill_min%d_batch_%05d.txt", opts.MinLevel, batchNumber)
		content := fmt.Sprintf(
			"Backfill refine min-level >= %d\nResult filter: %s\nSource: %s\nOffset: %d - %d\nEntries: %d\nBatch: %d\n",
			opts.MinLevel, formatResultFilter(opts.Result), opts.File, start, end, len(batch), batchNumber,
		)

		if err := sendBatchWithRetry(opts.Webhook, filename, payload, content); err != nil {
			return err
		}

		if err := saveCheckpoint(opts, end); err != nil {
			return err
		}
		sentBatches++
		fmt.Printf("Batch %d sent (%d entries, offset %d)\n", batchNumber, len(batch), end)
		batch = batch[:0]
		return nil
	}

	for {
		line, readErr := reader.ReadBytes('\n')
		lineLen := len(line)
		if lineLen > 0 {
			currentOffset += int64(lineLen)
			processedLines++
		}

		if lineLen > maxLineBytes {
			skippedLongLines++
			if readErr == nil {
				continue
			}
		}

		if lineLen > 0 {
			rawLine := bytes.TrimRight(line, "\r\n")
			parsed, ok := refine.ParseRefineLine(rawLine)
			if ok && matchFilter(parsed, opts) {
				displayName := resolveDisplayName(parsed.RoleID, lookupRole, roleCache)
				message := refine.BuildRefineLogMessage(
					displayName,
					parsed.Result,
					refine.GetItemName(parsed.ItemID),
					parsed.LevelBefore,
					refine.CalculateLevelAfter(parsed.Result, parsed.LevelBefore),
					parsed.StoneID,
				)
				batch = append(batch, batchEntry{
					line:   fmt.Sprintf("role=%s | %s", parsed.RoleID, message),
					offset: currentOffset,
				})
				matchedEntries++
				if err := flush(false); err != nil {
					fmt.Fprintf(os.Stderr, "Backfill failed while sending batch: %v\n", err)
					return 1
				}
			}
		}

		if processedLines > 0 && processedLines%200000 == 0 {
			fmt.Printf("Progress: lines=%d matched=%d offset=%d/%d\n", processedLines, matchedEntries, currentOffset, fileSize)
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Backfill read error after %d lines: %v\n", processedLines, readErr)
			return 1
		}
	}

	if err := flush(true); err != nil {
		fmt.Fprintf(os.Stderr, "Backfill failed while sending final batch: %v\n", err)
		return 1
	}

	if err := saveCheckpoint(opts, currentOffset); err != nil {
		fmt.Fprintf(os.Stderr, "Backfill finished but failed to save checkpoint: %v\n", err)
		return 1
	}

	fmt.Printf(
		"Backfill complete: lines=%d matched=%d batches=%d skipped_long=%d final_offset=%d\n",
		processedLines, matchedEntries, sentBatches, skippedLongLines, currentOffset,
	)
	return 0
}

func matchFilter(parsed refine.ParsedRefineLine, opts Options) bool {
	if opts.Result != "" && parsed.Result != opts.Result {
		return false
	}
	if parsed.LevelBefore < opts.MinLevel {
		return false
	}
	if opts.MaxLevel >= 0 && parsed.LevelBefore > opts.MaxLevel {
		return false
	}
	return true
}

func resolveDisplayName(roleID string, lookupRole LookupRoleFunc, cache map[int]string) string {
	roleNum := refine.MustAtoi(roleID)
	defaultName := fmt.Sprintf("role %s", roleID)
	if roleNum <= 0 || lookupRole == nil {
		return defaultName
	}
	if cached, ok := cache[roleNum]; ok {
		return cached
	}
	name, err := lookupRole(roleNum)
	if err != nil || strings.TrimSpace(name) == "" {
		cache[roleNum] = defaultName
		return defaultName
	}
	cache[roleNum] = name
	return name
}

func batchTotalBytes(batch []batchEntry) int {
	total := 0
	for _, item := range batch {
		total += len(item.line) + 1
	}
	return total
}

func buildBatchPayload(batch []batchEntry) string {
	var b strings.Builder
	for _, item := range batch {
		b.WriteString(item.line)
		b.WriteByte('\n')
	}
	return b.String()
}

func sendBatchWithRetry(webhookURL, fileName, payload, content string) error {
	const maxRateLimitWait = 60 * time.Second

	for attempt := 1; attempt <= 10; attempt++ {
		statusCode, retryAfter, err := sendBatchFile(webhookURL, fileName, payload, content)
		if err == nil {
			return nil
		}

		if statusCode == http.StatusTooManyRequests {
			if retryAfter <= 0 {
				retryAfter = 2
			}
			wait := time.Duration(retryAfter * float64(time.Second))
			if wait > maxRateLimitWait {
				wait = maxRateLimitWait
			}
			fmt.Printf("Rate limited (429), waiting %.1fs before retrying batch\n", wait.Seconds())
			time.Sleep(wait)
			continue
		}

		wait := time.Duration(attempt*2) * time.Second
		fmt.Printf("Discord send failed (status=%d): %v. Retrying in %s\n", statusCode, err, wait)
		time.Sleep(wait)
	}
	return fmt.Errorf("max retries reached while sending batch %s", fileName)
}

func sendBatchFile(webhookURL, fileName, payload, content string) (int, float64, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	payloadJSON, _ := json.Marshal(map[string]string{
		"content": content,
	})
	payloadPart, err := writer.CreateFormField("payload_json")
	if err != nil {
		return 0, 0, fmt.Errorf("failed creating payload_json field: %w", err)
	}
	if _, err := payloadPart.Write(payloadJSON); err != nil {
		return 0, 0, fmt.Errorf("failed writing payload_json field: %w", err)
	}

	filePart, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed creating file field: %w", err)
	}
	if _, err := io.Copy(filePart, strings.NewReader(payload)); err != nil {
		return 0, 0, fmt.Errorf("failed writing file payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return 0, 0, fmt.Errorf("failed closing multipart writer: %w", err)
	}

	resp, err := discord.Client.Post(webhookURL, writer.FormDataContentType(), &buf)
	if err != nil {
		return 0, 0, fmt.Errorf("failed posting batch to Discord: %w", err)
	}
	defer discord.DrainAndClose(resp)

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, 0, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		if retryAfter <= 0 {
			var parsed struct {
				RetryAfter float64 `json:"retry_after"`
			}
			if err := json.Unmarshal(body, &parsed); err == nil && parsed.RetryAfter > 0 {
				retryAfter = parsed.RetryAfter
			}
		}
		return resp.StatusCode, retryAfter, fmt.Errorf("discord rate limited 429")
	}
	return resp.StatusCode, 0, fmt.Errorf("discord webhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func parseRetryAfter(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

func formatResultFilter(result string) string {
	if result == "" {
		return "ALL"
	}
	return result
}

func loadCheckpoint(opts Options) int64 {
	if opts.Checkpoint == "" {
		return 0
	}
	data, err := os.ReadFile(opts.Checkpoint)
	if err != nil {
		return 0
	}
	var cp checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return 0
	}
	if cp.File != opts.File || cp.MinLevel != opts.MinLevel || cp.MaxLevel != opts.MaxLevel || cp.Result != opts.Result {
		return 0
	}
	return cp.Offset
}

func saveCheckpoint(opts Options, offset int64) error {
	if opts.Checkpoint == "" {
		return nil
	}
	dir := filepath.Dir(opts.Checkpoint)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	cp := checkpoint{
		File:      opts.File,
		Offset:    offset,
		MinLevel:  opts.MinLevel,
		MaxLevel:  opts.MaxLevel,
		Result:    opts.Result,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(cp, "", "  ")
	return os.WriteFile(opts.Checkpoint, data, 0o644)
}
