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
	"fmt"
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
	{Path: "0000/scripts/pre/my_script", Content: []byte("my first script"), IsDir: false},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},
}

func writeArchive(dir string) (path string, err error) {

	err = MakeFakeUpdateDir(dir, dirStructOK)
	if err != nil {
		return
	}

	aw := awriter.NewWriter("mender", 1)
	defer func() {
		err = aw.Close()
	}()
	rp := parser.NewRootfsParser(nil, "")
	aw.Register(rp)

	path = filepath.Join(dir, "artifact.tar.gz")
	err = aw.Write(dir, path)

	return
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
	rp := parser.NewRootfsParser(df, "")
	defer df.Close()

	aReader := NewReader(f)
	aReader.Register(rp)
	_, err = aReader.Read()
	assert.NoError(t, err)
	assert.NotNil(t, df)
	df.Close()

	data, err := ioutil.ReadFile(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)
	assert.Equal(t, "my first update", string(data))
}

func TestReadGeneric(t *testing.T) {
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

	aReader := NewReader(f)
	_, err = aReader.Read()
	assert.NoError(t, err)
}

func TestReadKnownUpdate(t *testing.T) {
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

	df, err := os.Create(filepath.Join(updateTestDir, "my_update"))
	rp := parser.NewRootfsParser(df, "")
	defer df.Close()

	aReader := NewReader(f)
	aReader.PushWorker(rp, "0000")
	_, err = aReader.Read()
	assert.NoError(t, err)
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

	aReader := NewReader(f)
	defer aReader.Close()
	rp := parser.NewRootfsParser(nil, "")
	aReader.Register(rp)

	info, err := aReader.ReadInfo()
	assert.NoError(t, err)
	assert.NotNil(t, info)

	hInfo, err := aReader.ReadHeaderInfo()
	assert.NoError(t, err)
	assert.NotNil(t, hInfo)

	df, err := os.Create(filepath.Join(updateTestDir, "my_update"))
	defer df.Close()

	for cnt, update := range hInfo.Updates {
		if update.Type == "rootfs-image" {
			rp := parser.NewRootfsParser(df, "")
			aReader.PushWorker(rp, fmt.Sprintf("%04d", cnt))
		}
	}

	hdr, err := aReader.ReadHeader()
	assert.NoError(t, err)
	assert.NotNil(t, hdr)

	w, err := aReader.ReadData()
	assert.NoError(t, err)

	for _, p := range w {
		assert.Equal(t, "vexpress-qemu", p.GetDeviceType())
	}

	data, err := ioutil.ReadFile(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)
	assert.Equal(t, "my first update", string(data))
}
