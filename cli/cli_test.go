// Copyright 2022 Northern.tech AS
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
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompressionArgumentLocations(t *testing.T) {
	app := getCliContext()

	dummyFile, err := os.CreateTemp("", "")
	require.NoError(t, err)
	_, err = dummyFile.Write([]byte("abcd"))
	require.NoError(t, err)
	dummyFile.Close()
	dummyName := dummyFile.Name()
	defer os.Remove(dummyName)

	menderFile, err := os.CreateTemp("", "")
	require.NoError(t, err)
	menderFile.Close()
	menderName := menderFile.Name()
	defer os.Remove(menderName)

	// Default
	err = app.Run([]string{"mender-artifact",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-c", "dummy",
		"-n", "dummy",
		"-o", menderName,
	})
	assert.NoError(t, err)
	outputBytes, err := exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.gz")
	assert.NotContains(t, string(outputBytes), "header.tar.xz")
	assert.NoError(t, err)

	// Global flag
	err = app.Run([]string{"mender-artifact",
		"--compression", "lzma",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-c", "dummy",
		"-n", "dummy",
		"-o", menderName,
	})
	assert.NoError(t, err)
	outputBytes, err = exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.xz")
	assert.NotContains(t, string(outputBytes), "header.tar.gz")
	assert.NoError(t, err)

	// Command flag
	err = app.Run([]string{"mender-artifact",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-c", "dummy",
		"-n", "dummy",
		"-o", menderName,
		"--compression", "lzma",
	})
	assert.NoError(t, err)
	outputBytes, err = exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.xz")
	assert.NotContains(t, string(outputBytes), "header.tar.gz")
	assert.NoError(t, err)

	// Overriding with lzma
	err = app.Run([]string{"mender-artifact",
		"--compression", "gzip",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-c", "dummy",
		"-n", "dummy",
		"-o", menderName,
		"--compression", "lzma",
	})
	assert.NoError(t, err)
	outputBytes, err = exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.xz")
	assert.NotContains(t, string(outputBytes), "header.tar.gz")
	assert.NoError(t, err)

	// Overriding with gz
	err = app.Run([]string{"mender-artifact",
		"--compression", "lzma",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-c", "dummy",
		"-n", "dummy",
		"-o", menderName,
		"--compression", "gzip",
	})
	assert.NoError(t, err)
	outputBytes, err = exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.gz")
	assert.NotContains(t, string(outputBytes), "header.tar.xz")
	assert.NoError(t, err)

	// write module-image now requires 'device-type' to be set
	assert.Error(t, app.Run([]string{"mender-artifact",
		"write",
		"module-image",
		"-T", "script",
		"-f", dummyName,
		"-n", "dummy",
		"-o", menderName,
	}))
}

func TestModuleImageWithoutPayload(t *testing.T) {
	app := getCliContext()

	menderFile, err := os.CreateTemp("", "")
	require.NoError(t, err)
	menderFile.Close()
	menderName := menderFile.Name()
	defer os.Remove(menderName)

	// Default
	err = app.Run([]string{"mender-artifact",
		"write",
		"module-image",
		"-c", "dummy",
		"-n", "dummy",
		"-T", "dummy",
		"-o", menderName,
	})
	assert.NoError(t, err)
	outputBytes, err := exec.Command("bash", "-c", fmt.Sprintf("tar xOf %s data/0000.tar.gz | tar tz", menderName)).Output()
	assert.NoError(t, err)
	assert.Empty(t, string(outputBytes))
}

func TestWriteBootstrapArtifact(t *testing.T) {
	app := getCliContext()

	menderFile, err := os.CreateTemp("", "")
	require.NoError(t, err)
	menderFile.Close()
	menderName := menderFile.Name()
	defer os.Remove(menderName)

	// Default
	err = app.Run([]string{"mender-artifact",
		"write",
		"bootstrap-artifact",
		"-c", "dummy",
		"-n", "dummy",
		"-G", "dep_gr",
		"-g", "pr_gr",
		"--clears-provides", "cl_pr",
		"-d", "dep:val",
		"-p", "pr:val",
		"-o", menderName,
	})
	assert.NoError(t, err)
	outputBytes, err := exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.gz")
	assert.NotContains(t, string(outputBytes), "header.tar.xz")
	assert.NoError(t, err)
}
