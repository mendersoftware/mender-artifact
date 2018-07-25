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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRootfsChecksum(t *testing.T) {
	// Create a rootfs image
	tdir, err := ioutil.TempDir("", "mendertmp")
	require.Nil(t, err)
	require.Nil(t, copyFile("mender_test.img", filepath.Join(tdir, "mender_test.img")))
	testimg := filepath.Join(tdir, "mender_test.img")
	defer os.RemoveAll(tdir)
	// Check the Checksum
	checksum, err := getRootfsChecksum(testimg)
	require.Nil(t, err)
	assert.Equal(t, "dc66c40bc3e52e1d0d3f46f417cbb8e12a86bc63b2a9b3be91ee77aa0fd680b0", checksum, "getRootfsChecksum calculates the wrong checksum")
	// No file
	checksum, err = getRootfsChecksum("foobarimg")
}

func TestArtifactsWrite(t *testing.T) {
	os.Args = []string{"mender-artifact", "write"}
	err := run()
	// should output help message and no error
	assert.NoError(t, err)

	fakeErrWriter.Reset()

	os.Args = []string{"mender-artifact", "write", "rootfs-image"}
	err = run()
	assert.Error(t, err)
	assert.Equal(t, 1, lastExitCode)
	assert.Equal(t, "must provide `device-type`, `artifact-name` and `update`\n",
		fakeErrWriter.String())

	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err = MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my update"),
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	// no whitespace allowed in artifact-name
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1. 1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender")}
	err = run()
	assert.Equal(t, "whitespace is not allowed in the artifact-name", err.Error())

	// store named file
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender")}
	err = run()
	assert.NoError(t, err)

	fs, err := os.Stat(filepath.Join(updateTestDir, "art.mender"))
	assert.NoError(t, err)
	assert.False(t, fs.IsDir())

	// store named file
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender"), "-v", "3"}
	err = run()
	assert.Error(t, err)
}

func TestWriteDelta(t *testing.T) {
}

func TestWithScripts(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my update"),
				IsDir:   false,
			},
			{
				Path:    "ArtifactInstall_Enter_99",
				Content: []byte("this is first enter script"),
				IsDir:   false,
			},
			{
				Path:    "ArtifactInstall_Leave_01",
				Content: []byte("this is leave script"),
				IsDir:   false,
			},
			{
				Path:    "script-dir",
				Content: []byte(""),
				IsDir:   true,
			},
			{
				Path:    "script-dir/ArtifactReboot_Enter_99",
				Content: []byte("this is reboot enter script"),
				IsDir:   false,
			},
			{
				Path:    "script-dir/ArtifactReboot_Leave_01",
				Content: []byte("this is reboot leave script"),
				IsDir:   false,
			},
			{
				Path:    "InvalidScript",
				Content: []byte("this is invalid script"),
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	// write artifact
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Enter_99"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Leave_01"),
		"-s", filepath.Join(updateTestDir, "script-dir")}
	err = run()
	assert.NoError(t, err)

	// read artifact
	os.Args = []string{"mender-artifact", "read",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.NoError(t, err)

	// write artifact vith invalid version
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Enter_99"),
		"-v", "1"}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "can not use scripts artifact with version 1\n",
		fakeErrWriter.String())

	// write artifact vith invalid script name
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "InvalidScript")}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "scripter: invalid script: InvalidScript\n",
		fakeErrWriter.String())
}
