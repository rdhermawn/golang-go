package search

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"golang-refine/internal/config"
	"golang-refine/internal/refine"
)

const maxLineBytes = 64 * 1024

type Options struct {
	File            string
	Result          string
	Contains        string
	Pattern         string
	MinLevel        int
	MaxLevel        int
	Limit           int
	compiledPattern *regexp.Regexp
}

func Run(cfg *config.Config, args []string) int {
	defaultFile := ""
	defaultBuffer := 64 * 1024
	if cfg != nil {
		defaultFile = strings.TrimSpace(cfg.LogFile)
		if cfg.MaxBuffer > defaultBuffer {
			defaultBuffer = cfg.MaxBuffer
		}
	}

	opts := Options{
		File:     defaultFile,
		MinLevel: -1,
		MaxLevel: -1,
	}

	fs := flag.NewFlagSet("search-refine", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.File, "file", opts.File, "Path file log refine")
	fs.StringVar(&opts.Result, "result", "", "Filter result: SUCCESS, FAILURE, RESET, DOWNGRADED")
	fs.StringVar(&opts.Contains, "contains", "", "Keyword substring pada summary atau line GBK yang sudah didecode")
	fs.StringVar(&opts.Pattern, "pattern", "", "Regex pada summary atau line GBK yang sudah didecode")
	fs.IntVar(&opts.MinLevel, "min-level", opts.MinLevel, "Filter level minimum")
	fs.IntVar(&opts.MaxLevel, "max-level", opts.MaxLevel, "Filter level maksimum")
	fs.IntVar(&opts.Limit, "limit", 0, "Batasi jumlah hasil, 0 = tanpa batas")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	opts.File = strings.TrimSpace(opts.File)
	opts.Result = strings.ToUpper(strings.TrimSpace(opts.Result))
	opts.Contains = strings.TrimSpace(opts.Contains)
	opts.Pattern = strings.TrimSpace(opts.Pattern)
	if opts.File == "" {
		fmt.Fprintln(os.Stderr, "Missing --file or config log_file")
		return 1
	}
	if opts.MinLevel >= 0 && opts.MaxLevel >= 0 && opts.MinLevel > opts.MaxLevel {
		fmt.Fprintln(os.Stderr, "Invalid level range: min-level cannot be greater than max-level")
		return 1
	}
	if opts.Pattern != "" {
		compiled, err := regexp.Compile(opts.Pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid --pattern: %v\n", err)
			return 1
		}
		opts.compiledPattern = compiled
	}

	f, err := os.Open(opts.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open search file: %v\n", err)
		return 1
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, defaultBuffer)
	lineNo := 0
	matched := 0

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > maxLineBytes {
			fmt.Fprintf(os.Stderr, "Search: skipped oversized line %d (%d bytes)\n", lineNo+1, len(line))
			if err == nil {
				continue
			}
			break
		}
		if len(line) > 0 {
			lineNo++
			rawLine := bytes.TrimRight(line, "\r\n")
			if parsed, ok := refine.ParseRefineLine(rawLine); ok && opts.matches(parsed) {
				fmt.Printf("%d: %s\n", lineNo, opts.buildMessage(parsed))
				matched++
				if opts.Limit > 0 && matched >= opts.Limit {
					break
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Search failed at line %d: %v\n", lineNo+1, err)
			return 1
		}
	}

	fmt.Printf("Found %d matching refine entries in %s\n", matched, opts.File)
	return 0
}

func (o Options) matches(parsed refine.ParsedRefineLine) bool {
	if o.Result != "" && parsed.Result != o.Result {
		return false
	}
	if o.MinLevel >= 0 && parsed.LevelBefore < o.MinLevel {
		return false
	}
	if o.MaxLevel >= 0 && parsed.LevelBefore > o.MaxLevel {
		return false
	}
	haystack := o.buildMessage(parsed)
	if parsed.DecodedLine != "" {
		haystack += "\n" + parsed.DecodedLine
	}
	if o.Contains != "" && !strings.Contains(strings.ToLower(haystack), strings.ToLower(o.Contains)) {
		return false
	}
	if o.compiledPattern != nil && !o.compiledPattern.MatchString(haystack) {
		return false
	}
	return true
}

func (o Options) buildMessage(parsed refine.ParsedRefineLine) string {
	return refine.BuildRefineLogMessage(
		fmt.Sprintf("role %s", parsed.RoleID),
		parsed.Result,
		refine.GetItemName(parsed.ItemID),
		parsed.LevelBefore,
		refine.CalculateLevelAfter(parsed.Result, parsed.LevelBefore),
		parsed.StoneID,
	)
}
