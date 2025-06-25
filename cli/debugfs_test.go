// Copyright 2021 Northern.tech AS
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
	"os"
	"testing"

	"github.com/mendersoftware/mender-artifact/utils"
	"github.com/stretchr/testify/assert"
)

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
		_, err := debugfsExecuteCommand(test.cmd, "mender_test.img")
		t.Log(name)
		assert.Contains(t, err.Error(), test.expected, "Unexpected error")
	}
}

func TestExternalBinaryDependency(t *testing.T) {
	// Set the PATH variable to be empty for the test.
	origPATH := os.Getenv("PATH")
	// "/usr/sbin", "/sbin", "/usr/local/sbin" also needs to be unset
	origExternalBinaryPaths := utils.ExternalBinaryPaths
	// also make sure that Mac Brew specific paths are not set
	origBrewSpecificPaths := utils.BrewSpecificPaths
	utils.ExternalBinaryPaths = []string{}
	utils.BrewSpecificPaths = []string{}
	defer func() {
		os.Setenv("PATH", origPATH)
		utils.ExternalBinaryPaths = origExternalBinaryPaths
		utils.BrewSpecificPaths = origBrewSpecificPaths
	}()
	os.Setenv("PATH", "")
	tmpdir, err := debugfsCopyFile("foo", "bar")
	assert.EqualError(t, err, debugfsMissingErr)
	defer os.RemoveAll(tmpdir)

	_, err = debugfsExecuteCommand("foobar", "bash")
	assert.EqualError(t, err, debugfsMissingErr)

	_, err = processSdimg("foobar")
	assert.Contains(t, err.Error(), "`parted` binary not found on the system")

	_, err = imgFilesystemType("foobar")
	assert.ErrorIs(t, err, errBlkidNotFound)
}
