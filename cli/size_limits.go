// Copyright 2025 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package cli

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	KB = 1000
	MB = 1000 * 1000
	GB = 1000 * 1000 * 1000
	TB = 1000 * 1000 * 1000 * 1000
)

// ParseSizeLimit parses size strings like "5MB", "1.5GB", "100"
// Supports: B, KB, MB, GB, TB (decimal, base 1000)
// Case-insensitive, allows decimals (1.5MB), bare numbers (5000000)
func ParseSizeLimit(sizeStr string) (int64, error) {
	if sizeStr == "" {
		return 0, errors.New("size string is empty")
	}

	// Regex: optional decimal number + optional unit
	// Examples: "5", "5MB", "1.5GB", "100kb"
	re := regexp.MustCompile(`^([0-9]+\.?[0-9]*)([a-zA-Z]*)$`)
	matches := re.FindStringSubmatch(strings.TrimSpace(sizeStr))

	if matches == nil {
		return 0, errors.Errorf("invalid size format: %s", sizeStr)
	}

	// Parse number part
	numStr := matches[1]
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid number: %s", numStr)
	}

	// Parse unit part (case-insensitive)
	unit := strings.ToUpper(matches[2])

	var multiplier int64
	switch unit {
	case "", "B":
		multiplier = 1
	case "KB":
		multiplier = KB
	case "MB":
		multiplier = MB
	case "GB":
		multiplier = GB
	case "TB":
		multiplier = TB
	default:
		return 0, errors.Errorf(
			"unsupported unit: %s (supported: B, KB, MB, GB, TB)",
			unit,
		)
	}

	result := int64(num * float64(multiplier))
	return result, nil
}

// FormatSize formats bytes into human-readable format
// Examples: 5000000 -> "5.0 MB", 1500000000 -> "1.5 GB"
func FormatSize(bytes int64) string {
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// CheckArtifactSize checks the Artifact file size against configured limits
// Returns error if max limit exceeded (and deletes file)
// Logs warning if warn limit exceeded (keeps file)
func CheckArtifactSize(outputPath string, ctx *cli.Context) error {
	// Get file info
	fi, err := os.Stat(outputPath)
	if err != nil {
		return errors.Wrapf(err, "failed to stat Artifact file: %s", outputPath)
	}

	size := fi.Size()

	// Check hard limit (--max-artifact-size)
	if maxSize := ctx.String("max-artifact-size"); maxSize != "" {
		limit, err := ParseSizeLimit(maxSize)
		if err != nil {
			return errors.Wrap(err, "invalid --max-artifact-size")
		}

		if size > limit {
			// Delete the artifact file
			if rmErr := os.Remove(outputPath); rmErr != nil {
				Log.Warnf("Failed to delete Artifact file: %v", rmErr)
			}

			return fmt.Errorf(
				"Artifact size %s exceeds maximum allowed size %s;"+
					" file deleted",
				FormatSize(size),
				maxSize,
			)
		}
	}

	// Check soft limit (--warn-artifact-size)
	if warnSize := ctx.String("warn-artifact-size"); warnSize != "" {
		limit, err := ParseSizeLimit(warnSize)
		if err != nil {
			return errors.Wrap(err, "invalid --warn-artifact-size")
		}

		if size > limit {
			Log.Warnf(
				"Artifact size %s exceeds specified limit %s",
				FormatSize(size),
				warnSize,
			)
		}
	}

	return nil
}
