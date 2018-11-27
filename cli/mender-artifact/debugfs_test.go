// Copyright 2018 Northern.tech AS
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

package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFsck(t *testing.T) {
	err := debugfsRunFsck("mender_test.img.broken")
	assert.Error(t, err)

	err = debugfsRunFsck("mender_test.img")
	assert.NoError(t, err)
}

func TestExecuteCommand(t *testing.T) {
	tests := map[string]struct {
		cmd      string
		expected string
	}{
		"non-existing directory": {
			cmd:      "cd /mender",
			expected: "File not found by ext2_lookup",
		},
	}

	for name, test := range tests {
		err := executeCommand(test.cmd, "mender_test.img")
		t.Log(name)
		fmt.Fprintf(os.Stderr, "err: %s\n", err)
		assert.Contains(t, err.Error(), test.expected, "Unexpected error")
	}
}
