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

package reader

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/mendersoftware/artifacts/parser"
	"github.com/mendersoftware/artifacts/writer"
	"github.com/stretchr/testify/assert"

	. "github.com/mendersoftware/artifacts/test_utils"
)

var dirStructOK = []TestDirEntry{
	{Path: "0000", IsDir: true},
	{Path: "0000/data", IsDir: true},
	{Path: "0000/data/update.ext4", Content: []byte("my first update"), IsDir: false},
	{Path: "0000/type-info",
		Content: []byte(`{"type": "rootfs-image"}`),
		IsDir:   false},
	{Path: "0000/meta-data",
		Content: []byte(`{"DeviceType": "vexpress-qemu", "ImageID": "core-image-minimal-201608110900"}`),
		IsDir:   false},
	{Path: "0000/signatures", IsDir: true},
	{Path: "0000/signatures/update.sig", IsDir: false},
	{Path: "0000/scripts", IsDir: true},
	{Path: "0000/scripts/pre", IsDir: true},
	{Path: "0000/scripts/pre/my_script", IsDir: false},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},
}

func writeArchive(dir string) (string, error) {

	err := MakeFakeUpdateDir(dir, dirStructOK)
	if err != nil {
		return "", err
	}

	aw := writer.NewArtifactsWriter("artifact.tar.gz", dir, "mender", 1)
	rp := parser.NewRootfsParser("", nil)
	aw.Register(rp, "rootfs-image")
	err = aw.Write()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "artifact.tar.gz"), nil
}

func TestReadArchive(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	archive, err := writeArchive(updateTestDir)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	// open archive file
	f, err := os.Open(archive)
	defer f.Close()
	assert.NoError(t, err)
	assert.NotNil(t, f)

	df, err := os.Create(path.Join(updateTestDir, "my_update"))

	aReader := NewArtifactsReader(f)
	rp := parser.NewRootfsParser("", df)
	aReader.Register(rp, "rootfs-image")
	err = aReader.Read()
	assert.NoError(t, err)
	assert.NotNil(t, df)
	df.Close()

	data, err := ioutil.ReadFile(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)
	assert.Equal(t, "my first update", string(data))
}

func TestReadSequence(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	archive, err := writeArchive(updateTestDir)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	// open archive file
	f, err := os.Open(archive)
	defer f.Close()
	assert.NoError(t, err)
	assert.NotNil(t, f)

	aReader := NewArtifactsReader(f)
	defer aReader.Close()
	rp := parser.NewRootfsParser("", nil)
	aReader.Register(rp, "rootfs-image")

	upd, err := aReader.GetUpdates()
	assert.NoError(t, err)
	assert.NotNil(t, upd)

	upd, err = aReader.ReadHeader()
	assert.NoError(t, err)
	assert.NotNil(t, upd)

	err = aReader.ProcessUpdateFiles()
	assert.NoError(t, err)
}
