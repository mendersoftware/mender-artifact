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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSizeLimit_ValidInputs(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		// Bare numbers (bytes)
		{"100", 100},
		{"5000000", 5000000},

		// With units (uppercase)
		{"5B", 5},
		{"5KB", 5000},
		{"5MB", 5000000},
		{"5GB", 5000000000},
		{"5TB", 5000000000000},

		// With units (lowercase)
		{"5kb", 5000},
		{"5mb", 5000000},
		{"5gb", 5000000000},
		{"5tb", 5000000000000},

		// With units (mixed case)
		{"5Mb", 5000000},
		{"5Gb", 5000000000},
		{"5Kb", 5000},
		{"5Tb", 5000000000000},

		// Decimal values
		{"1.5MB", 1500000},
		{"0.5GB", 500000000},
		{"2.25GB", 2250000000},
		{"10.5KB", 10500},

		// Edge cases
		{"0", 0},
		{"0B", 0},
		{"0MB", 0},
		{"1", 1},
		{"1B", 1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseSizeLimit(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSizeLimit_InvalidInputs(t *testing.T) {
	tests := []string{
		"",          // Empty string
		"invalid",   // No number
		"5XB",       // Invalid unit
		"5MiB",      // Binary units not supported
		"MB5",       // Wrong order
		"-5MB",      // Negative not allowed
		"5 MB",      // Space in middle
		" ",         // Just whitespace
		"5.5.5MB",   // Multiple decimal points
		"abc123",    // Invalid format
		"5PB",       // Unsupported unit (petabytes)
		"5EB",       // Unsupported unit (exabytes)
		"KB",        // No number
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseSizeLimit(input)
			assert.Error(t, err, "Expected error for input: %q", input)
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		// Bytes
		{0, "0 B"},
		{100, "100 B"},
		{999, "999 B"},

		// Kilobytes
		{1000, "1.0 KB"},
		{5000, "5.0 KB"},
		{5500, "5.5 KB"},
		{999999, "1000.0 KB"},

		// Megabytes
		{1000000, "1.0 MB"},
		{5000000, "5.0 MB"},
		{5500000, "5.5 MB"},
		{999999999, "1000.0 MB"},

		// Gigabytes
		{1000000000, "1.0 GB"},
		{5000000000, "5.0 GB"},
		{1500000000, "1.5 GB"},
		{999999999999, "1000.0 GB"},

		// Terabytes
		{1000000000000, "1.0 TB"},
		{5000000000000, "5.0 TB"},
		{1500000000000, "1.5 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSizeLimit_WithWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"  5MB  ", 5000000}, // Leading and trailing whitespace
		{" 100 ", 100},       // Whitespace around number
		{"  5GB  ", 5000000000},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseSizeLimit(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSizeLimit_LargeValues(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1000TB", 1000000000000000},
		{"999GB", 999000000000},
		{"100000MB", 100000000000},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseSizeLimit(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSize_Boundaries(t *testing.T) {
	// Test boundary values between units
	assert.Equal(t, "999 B", FormatSize(999))
	assert.Equal(t, "1.0 KB", FormatSize(1000))

	assert.Equal(t, "999.9 KB", FormatSize(999900))
	assert.Equal(t, "1.0 MB", FormatSize(1000000))

	assert.Equal(t, "999.9 MB", FormatSize(999900000))
	assert.Equal(t, "1.0 GB", FormatSize(1000000000))

	assert.Equal(t, "999.9 GB", FormatSize(999900000000))
	assert.Equal(t, "1.0 TB", FormatSize(1000000000000))
}
