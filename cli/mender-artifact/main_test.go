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
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompressionArgumentLocations(t *testing.T) {
	app := getCliContext()

	dummyFile, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	_, err = dummyFile.Write([]byte("abcd"))
	require.NoError(t, err)
	dummyFile.Close()
	dummyName := dummyFile.Name()
	defer os.Remove(dummyName)

	menderFile, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	menderFile.Close()
	menderName := menderFile.Name()
	defer os.Remove(menderName)

	// Default
	app.Run([]string{"mender-artifact",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-t", "dummy",
		"-n", "dummy",
		"-o", menderName,
	})
	outputBytes, err := exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.gz")
	assert.NotContains(t, string(outputBytes), "header.tar.xz")
	assert.NoError(t, err)

	// Global flag
	app.Run([]string{"mender-artifact",
		"--compression", "lzma",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-t", "dummy",
		"-n", "dummy",
		"-o", menderName,
	})
	outputBytes, err = exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.xz")
	assert.NotContains(t, string(outputBytes), "header.tar.gz")
	assert.NoError(t, err)

	// Command flag
	app.Run([]string{"mender-artifact",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-t", "dummy",
		"-n", "dummy",
		"-o", menderName,
		"--compression", "lzma",
	})
	outputBytes, err = exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.xz")
	assert.NotContains(t, string(outputBytes), "header.tar.gz")
	assert.NoError(t, err)

	// Overriding with lzma
	app.Run([]string{"mender-artifact",
		"--compression", "gzip",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-t", "dummy",
		"-n", "dummy",
		"-o", menderName,
		"--compression", "lzma",
	})
	outputBytes, err = exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.xz")
	assert.NotContains(t, string(outputBytes), "header.tar.gz")
	assert.NoError(t, err)

	// Overriding with gz
	app.Run([]string{"mender-artifact",
		"--compression", "lzma",
		"write",
		"rootfs-image",
		"-f", dummyName,
		"-t", "dummy",
		"-n", "dummy",
		"-o", menderName,
		"--compression", "gzip",
	})
	outputBytes, err = exec.Command("tar", "tf", menderName).Output()
	assert.Contains(t, string(outputBytes), "header.tar.gz")
	assert.NotContains(t, string(outputBytes), "header.tar.xz")
	assert.NoError(t, err)
}
