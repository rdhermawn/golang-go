package tail

import (
	"bufio"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

const maxLineBytes = 64 * 1024

func Start(filePath string, lineCh chan<- []byte, done <-chan struct{}) {
	var prevSize int64
	initialized := false

	for {
		select {
		case <-done:
			return
		default:
		}

		f, err := os.Open(filePath)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		stat, err := f.Stat()
		if err != nil {
			f.Close()
			time.Sleep(1 * time.Second)
			continue
		}

		if stat.Size() < prevSize {
			prevSize = 0
		}

		if !initialized {
			prevSize = stat.Size()
			initialized = true
			f.Close()
			time.Sleep(1 * time.Second)
			continue
		}

		if stat.Size() > prevSize {
			_, err = f.Seek(prevSize, io.SeekStart)
			if err == nil {
				reader := bufio.NewReader(f)
				for {
					line, readErr := reader.ReadString('\n')
					if len(line) > maxLineBytes {
						log.Printf("tail: skipped oversized line (%d bytes), possible corrupt file", len(line))
						line = ""
					}
					if len(line) > 0 {
						lineCh <- []byte(strings.TrimRight(line, "\r\n"))
					}
					if readErr != nil {
						if readErr != io.EOF {
							log.Printf("tail: read error: %v", readErr)
						}
						break
					}
				}
			}
			newStat, statErr := f.Stat()
			if statErr == nil {
				prevSize = newStat.Size()
			} else {
				prevSize = stat.Size()
			}
		}

		f.Close()
		time.Sleep(500 * time.Millisecond)
	}
}
