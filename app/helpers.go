package app

import (
	"fmt"
	"math"
	"mime"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	bitSize64 = 64
	kilo      = 1024
	mega      = kilo * kilo
	giga      = kilo * mega
)

func isPiped() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fileInfo.Mode()&os.ModeCharDevice == 0
}

func parseValidateURL(in string) (string, error) {
	u, err := url.ParseRequestURI(in)
	if err != nil {
		return "", fmt.Errorf("%s %w", errInvalidURL.Error(), err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errInvalidURL
	}
	return u.String(), nil
}

func findExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "jpg"
	case "video/mp4":
		return "mp4"
	default:
		ext, err := mime.ExtensionsByType(mimeType)
		if err != nil || len(ext) == 0 {
			return "unknown"
		}
		return strings.TrimPrefix(ext[0], ".")
	}
}

// getChunks splits a byte range into N equal-ish chunks.
func getChunks(length int, chunkSize int) [][2]int {
	if length <= 0 || chunkSize <= 0 {
		return nil
	}

	// don't create more chunks than bytes
	if chunkSize > length {
		chunkSize = length
	}

	out := make([][2]int, 0, chunkSize)
	chunkLen := length / chunkSize
	remainder := length % chunkSize

	start := 0
	for range chunkSize {
		end := start + chunkLen - 1
		if remainder > 0 {
			end++
			remainder--
		}
		out = append(out, [2]int{start, end})
		start = end + 1
	}

	return out
}

// parseRate parses bandwidth rate strings like "5M", "500K", "1G".
// Returns bytes per second. 0 means unlimited.
func parseRate(s string) (int64, error) {
	if s == "" || s == "0" {
		return 0, nil
	}

	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)

	multiplier := int64(1)
	numStr := upper

	switch {
	case strings.HasSuffix(upper, "G"):
		multiplier = giga
		numStr = upper[:len(upper)-1]
	case strings.HasSuffix(upper, "M"):
		multiplier = mega
		numStr = upper[:len(upper)-1]
	case strings.HasSuffix(upper, "K"):
		multiplier = kilo
		numStr = upper[:len(upper)-1]
	default:
		// no suffix, treat as raw bytes per second
	}

	num, err := strconv.ParseFloat(numStr, bitSize64)
	if err != nil {
		return 0, fmt.Errorf("invalid rate: %s", s)
	}

	if num < 0 || math.IsNaN(num) || math.IsInf(num, 0) {
		return 0, fmt.Errorf("invalid rate value: %s", s)
	}

	result := num * float64(multiplier)
	if result > float64(math.MaxInt64) {
		return 0, fmt.Errorf("rate value too large: %s", s)
	}

	return int64(result), nil
}

// formatBytes formats byte count to human readable string.
func formatBytes(bytes int64) string {
	switch {
	case bytes >= giga:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(giga))
	case bytes >= mega:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(mega))
	case bytes >= kilo:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(kilo))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
