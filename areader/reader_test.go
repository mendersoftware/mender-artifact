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

package areader

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/stretchr/testify/assert"
)

func WriteRootfsImageArchive(dir string, version int, signed bool) error {
	if err := artifact.MakeFakeUpdateDir(dir,
		[]artifact.TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my first update"),
				IsDir:   false,
			},
		}); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(dir, "artifact.mender"))
	if err != nil {
		return err
	}
	defer f.Close()

	var aw *awriter.Writer
	if !signed {
		aw = awriter.NewWriter(f)
	} else {
		aw = awriter.NewWriterSigned(f, new(artifact.DummySigner))
	}
	var u artifact.Composer
	switch version {
	case 1:
		u = handlers.NewRootfsV1(filepath.Join(dir, "update.ext4"))
	case 2:
		u = handlers.NewRootfsV2(filepath.Join(dir, "update.ext4"))
	}
	updates := &artifact.Updates{U: []artifact.Composer{u}}
	return aw.WriteArtifact("mender", version, []string{"vexpress"},
		"mender-1.1", updates)
}

func TestReadArtifact(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := WriteRootfsImageArchive(updateTestDir, 1, false)
	assert.NoError(t, err)

	// open archive file
	f, err := os.Open(filepath.Join(updateTestDir, "artifact.mender"))
	assert.NoError(t, err)
	assert.NotNil(t, f)

	defer f.Close()

	// open file to write data to
	df, err := os.Create(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)

	aReader := NewReader(f)

	h := handlers.NewRootfsInstaller()
	h.InstallHandler = func(r io.Reader, f *artifact.File) error {
		_, cErr := io.Copy(df, r)
		return cErr
	}
	aReader.RegisterHandler(h)

	err = aReader.ReadArtifact()
	assert.NoError(t, err)

	df.Close()
	inst := aReader.GetHandlers()
	assert.Len(t, inst, 1)
	_, ok := inst[0].(*handlers.Rootfs)
	assert.True(t, ok)
	assert.Len(t, aReader.GetCompatibleDevices(), 1)
	assert.Equal(t, "vexpress", aReader.GetCompatibleDevices()[0])

	data, err := ioutil.ReadFile(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)
	assert.Equal(t, "my first update", string(data))
	assert.Equal(t, "vexpress", aReader.GetCompatibleDevices()[0])
}

func TestReadArtifactV2(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := WriteRootfsImageArchive(updateTestDir, 2, false)
	assert.NoError(t, err)

	// open archive file
	f, err := os.Open(filepath.Join(updateTestDir, "artifact.mender"))
	assert.NoError(t, err)
	assert.NotNil(t, f)

	defer f.Close()

	// open file to write data to
	df, err := os.Create(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)

	aReader := NewReader(f)

	h := handlers.NewRootfsInstaller()
	h.InstallHandler = func(r io.Reader, f *artifact.File) error {
		_, cErr := io.Copy(df, r)
		return cErr
	}
	aReader.RegisterHandler(h)

	err = aReader.ReadArtifact()
	assert.NoError(t, err)

	df.Close()
	inst := aReader.GetHandlers()
	assert.Len(t, inst, 1)
	_, ok := inst[0].(*handlers.Rootfs)
	assert.True(t, ok)
	assert.Len(t, aReader.GetCompatibleDevices(), 1)
	assert.Equal(t, "vexpress", aReader.GetCompatibleDevices()[0])

	data, err := ioutil.ReadFile(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)
	assert.Equal(t, "my first update", string(data))
	assert.Equal(t, "vexpress", aReader.GetCompatibleDevices()[0])
}

func TestReadArtifactSignedV2(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := WriteRootfsImageArchive(updateTestDir, 2, true)
	assert.NoError(t, err)

	// open archive file
	f, err := os.Open(filepath.Join(updateTestDir, "artifact.mender"))
	assert.NoError(t, err)
	assert.NotNil(t, f)

	defer f.Close()

	// open file to write data to
	df, err := os.Create(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)

	aReader := NewReader(f)

	h := handlers.NewRootfsInstaller()
	h.InstallHandler = func(r io.Reader, f *artifact.File) error {
		_, cErr := io.Copy(df, r)
		return cErr
	}
	aReader.RegisterHandler(h)

	v := new(artifact.DummySigner)
	aReader.VerifySignatureCallback = v.Verify

	err = aReader.ReadArtifact()
	assert.NoError(t, err)

	df.Close()
	inst := aReader.GetHandlers()
	assert.Len(t, inst, 1)
	_, ok := inst[0].(*handlers.Rootfs)
	assert.True(t, ok)
	assert.Len(t, aReader.GetCompatibleDevices(), 1)
	assert.Equal(t, "vexpress", aReader.GetCompatibleDevices()[0])

	data, err := ioutil.ReadFile(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)
	assert.Equal(t, "my first update", string(data))
	assert.Equal(t, "vexpress", aReader.GetCompatibleDevices()[0])
}
