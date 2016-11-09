// Copyright 2016 Mender Software AS
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
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"

	"github.com/mendersoftware/artifacts/parser"
	. "github.com/mendersoftware/artifacts/test_utils"
	"github.com/mendersoftware/artifacts/writer"
)

var (
	lastExitCode = 0
	fakeOsExiter = func(rc int) {
		lastExitCode = rc
	}
	fakeErrWriter = &bytes.Buffer{}
)

func init() {
	cli.OsExiter = fakeOsExiter
	cli.ErrWriter = fakeErrWriter
}

func WriteRootfsImageArchive(dir string, dirStruct []TestDirEntry) (path string, err error) {
	err = MakeFakeUpdateDir(dir, dirStruct)
	if err != nil {
		return
	}

	aw := awriter.NewWriter("mender", 1, []string{"vexpress"}, "mender-1.1")
	rp := &parser.RootfsParser{}
	aw.Register(rp)

	path = filepath.Join(dir, "artifact.tar.gz")
	err = aw.Write(dir, path)
	return
}

func TestArtifactsWrite(t *testing.T) {
	os.Args = []string{"artifacts", "write"}
	err := run()
	// should output help message and no error
	assert.NoError(t, err)

	os.Args = []string{"artifacts", "write", "rootfs-image"}
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

	// store named file
	os.Args = []string{"artifacts", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender")}
	err = run()
	assert.NoError(t, err)

	fs, err := os.Stat(filepath.Join(updateTestDir, "art.mender"))
	assert.NoError(t, err)
	assert.False(t, fs.IsDir())
}

func TestArtifactsValidate(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	archive, err := WriteRootfsImageArchive(updateTestDir, RootfsImageStructOK)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	os.Args = []string{"artifacts", "validate",
		filepath.Join(updateTestDir, "artifact.tar.gz")}
	err = run()
	assert.NoError(t, err)

	os.Args = []string{"artifacts", "validate", "non-existing"}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, 1, lastExitCode)
	assert.Equal(t, "Pathspec 'non-existing' does not match any files.\n",
		fakeErrWriter.String())
}

func TestArtifactsRead(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	archive, err := WriteRootfsImageArchive(updateTestDir, RootfsImageStructOK)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	os.Args = []string{"artifacts", "read", "artifact",
		filepath.Join(updateTestDir, "artifact.tar.gz")}
	err = run()
	assert.NoError(t, err)

	os.Args = []string{"artifacts", "validate", "non-existing"}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, 1, lastExitCode)
	assert.Equal(t, "Pathspec 'non-existing' does not match any files.\n",
		fakeErrWriter.String())
}
