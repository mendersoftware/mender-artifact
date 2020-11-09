// Copyright 2020 Northern.tech AS
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

package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringsMatchingWildcards(t *testing.T) {
	testCases := []struct {
		provides []string
		clearsProvides []string
		expected []string
	}{
		{
			provides: []string{
				"rootfs-image.version",
			},
			clearsProvides: []string{
				"rootfs-image.*",
			},
			expected: []string{
				"rootfs-image.version",
			},
		},
		{
			provides: []string{
				"rootfs-image.version",
				"rootfs-image.single-file.version",
			},
			clearsProvides: []string{
				"rootfs-image.*",
			},
			expected: []string{
				"rootfs-image.version",
				"rootfs-image.single-file.version",
			},
		},
		{
			provides: []string{
				"rootfs-image.v",
				"rootfs-image.version",
				"rootfs-image.single-file.version",
			},
			clearsProvides: []string{
				"rootfs-image.",
			},
			expected: []string{},
		},
		{
			provides: []string{
				"rootfs-image.version",
				"rootfs-image.single-file.version",
			},
			clearsProvides: []string{},
			expected: []string{},
		},
		{
			provides: []string{
				"rootfs-image.version",
				"rootfs-image.single-file.version",
			},
			clearsProvides: nil,
			expected: []string{},
		},
		{
			provides: []string{
				"rootfs-image.version",
				"rootfs-image.single-file.version",
			},
			clearsProvides: []string{
				"rootfs-image.version",
			},
			expected: []string{
				"rootfs-image.version",
			},
		},
		{
			provides: []string{},
			clearsProvides: []string{
				"rootfs-image.version",
			},
			expected: []string{},
		},
		{
			provides: nil,
			clearsProvides: []string{
				"rootfs-image.version",
			},
			expected: []string{},
		},
		{
			provides: []string{
				"rootfs-image.*",
				"rootfs-image.version",
			},
			clearsProvides: []string{
				"rootfs-image.\\*",
			},
			expected: []string{
				"rootfs-image.*",
			},
		},
	}

	for n, tc := range testCases {
		t.Run(fmt.Sprintf("Test case %d", n), func(t *testing.T) {
			result, err := StringsMatchingWildcards(tc.provides, tc.clearsProvides)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestStringsMatchingWildcardsError(t *testing.T) {
	_, err := StringsMatchingWildcards(nil, []string{"\\"})
	assert.Error(t, err)
}
