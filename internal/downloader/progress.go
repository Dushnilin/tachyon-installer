package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ProgressCallback receives the current download stats.
type ProgressCallback func(downloaded int64, total int64, speedBytesPerSec int64)

// ProgressReader wraps an io.Reader and reports live download progress.
type ProgressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	chunkSize  int64
	lastReport int64
	lastTime   time.Time
	startTime  time.Time
	callback   ProgressCallback
}

// NewProgressReader creates a new progress reader.
func NewProgressReader(r io.Reader, total int64, callback ProgressCallback) *ProgressReader {
	now := time.Now()
	return &ProgressReader{
		reader:    r,
		total:     total,
		chunkSize: 128 * 1024, // report every 128 KB
		lastTime:  now,
		startTime: now,
		callback:  callback,
	}
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)

	now := time.Now()
	if pr.downloaded-pr.lastReport >= pr.chunkSize || err == io.EOF || now.Sub(pr.lastTime) >= 300*time.Millisecond {
		elapsed := now.Sub(pr.startTime).Seconds()
		var speed int64
		if elapsed > 0 {
			speed = int64(float64(pr.downloaded) / elapsed)
		}
		pr.lastReport = pr.downloaded
		pr.lastTime = now

		if pr.callback != nil {
			pr.callback(pr.downloaded, pr.total, speed)
		}
	}

	return n, err
}

// ParseSHA256Sums parses lines from a sha256sums.txt file.
// Format: "<hash>  <filename>" or "<hash> *<filename>"
func ParseSHA256Sums(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			hash := strings.ToLower(strings.TrimSpace(fields[0]))
			filename := strings.TrimSpace(fields[1])
			filename = strings.TrimPrefix(filename, "*")
			// Only valid 64-char hex hashes
			if len(hash) == 64 {
				result[filename] = hash
			}
		}
	}
	return result
}

// VerifyFileSHA256 computes the SHA256 hash of a file and verifies it against expectedHash.
func VerifyFileSHA256(filePath, expectedHash string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file for checksum: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("read file for checksum: %w", err)
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}
	return nil
}
