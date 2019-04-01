// Copyright 2019 Northern.tech AS
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

	"github.com/mendersoftware/mender-artifact/utils"
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
		_, err := executeCommand(test.cmd, "mender_test.img")
		t.Log(name)
		fmt.Fprintf(os.Stderr, "err: %s\n", err)
		assert.Contains(t, err.Error(), test.expected, "Unexpected error")
	}
}

func TestExternalBinaryDependency(t *testing.T) {
	// Set the PATH variable to be empty for the test.
	origPATH := os.Getenv("PATH")
	// "/usr/sbin", "/sbin", "/usr/local/sbin" also needs to be unset
	utils.ExternalBinaryPaths = []string{}
	defer func() {
		os.Setenv("PATH", origPATH)
	}()
	os.Setenv("PATH", "")
	_, err := debugfsCopyFile("foo", "bar")
	assert.EqualError(t, err, debugfsMissingErr)

	_, err = executeCommand("foobar", "bash")
	assert.EqualError(t, err, debugfsMissingErr)

	_, err = processSdimg("foobar")
	assert.EqualError(t, err, "`parted` binary not found on the system")

	_, err = imgFilesystemType("foobar")
	assert.EqualError(t, err, "`blkid` binary not found on the system")
}
