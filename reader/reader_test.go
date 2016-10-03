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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/mendersoftware/artifacts/parser"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	. "github.com/mendersoftware/artifacts/test_utils"
	"github.com/mendersoftware/artifacts/writer"
)

func WriteRootfsImageArchive(dir string, dirStruct []TestDirEntry) (path string, err error) {
	err = MakeFakeUpdateDir(dir, dirStruct)
	if err != nil {
		return
	}

	aw := awriter.NewWriter("mender", 1)
	rp := &parser.RootfsParser{}
	aw.Register(rp)

	path = filepath.Join(dir, "artifact.tar.gz")
	err = aw.Write(dir, path)
	return
}

func TestReadArchive(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	archive, err := WriteRootfsImageArchive(updateTestDir, RootfsImageStructOK)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	// open archive file
	f, err := os.Open(archive)
	defer f.Close()
	assert.NoError(t, err)
	assert.NotNil(t, f)

	df, err := os.Create(path.Join(updateTestDir, "my_update"))
	rp := &parser.RootfsParser{W: df}
	defer df.Close()

	aReader := NewReader(f)
	aReader.Register(rp)
	p, err := aReader.Read()
	assert.NoError(t, err)
	assert.NotNil(t, df)
	df.Close()
	assert.Len(t, p, 1)
	rp, ok := p["0000"].(*parser.RootfsParser)
	assert.True(t, ok)
	assert.Equal(t, "vexpress-qemu", rp.GetDeviceType())
	assert.Equal(t, "core-image-minimal-201608110900", rp.GetImageID())

	data, err := ioutil.ReadFile(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)
	assert.Equal(t, "my first update", string(data))
}

func TestReadArchiveCustomHandler(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	archive, err := WriteRootfsImageArchive(updateTestDir, RootfsImageStructOK)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	// open archive file
	f, err := os.Open(archive)
	defer f.Close()
	assert.NoError(t, err)
	assert.NotNil(t, f)

	var called bool
	rp := &parser.RootfsParser{
		DataFunc: func(r io.Reader, dt string, uf parser.UpdateFile) error {
			called = true
			assert.Equal(t, "vexpress-qemu", dt)
			assert.Equal(t, "update.ext4", uf.Name)

			b := bytes.Buffer{}

			n, err := io.Copy(&b, r)
			assert.NoError(t, err)
			assert.Equal(t, uf.Size, n)
			assert.Equal(t, []byte("my first update"), b.Bytes())
			return nil
		},
	}

	aReader := NewReader(f)
	aReader.Register(rp)
	_, err = aReader.Read()
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestReadArchiveCustomHandlerError(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	archive, err := WriteRootfsImageArchive(updateTestDir, RootfsImageStructOK)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	// open archive file
	f, err := os.Open(archive)
	defer f.Close()
	assert.NoError(t, err)
	assert.NotNil(t, f)

	var called bool
	rp := &parser.RootfsParser{
		DataFunc: func(r io.Reader, dt string, uf parser.UpdateFile) error {
			called = true
			return errors.New("failed")
		},
	}

	aReader := NewReader(f)
	aReader.Register(rp)
	_, err = aReader.Read()
	assert.Error(t, err)
	assert.True(t, called)
}

func TestReadGeneric(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	archive, err := WriteRootfsImageArchive(updateTestDir, RootfsImageStructOK)
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

	archive, err := WriteRootfsImageArchive(updateTestDir, RootfsImageStructOK)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	// open archive file
	f, err := os.Open(archive)
	defer f.Close()
	assert.NoError(t, err)
	assert.NotNil(t, f)

	df, err := os.Create(filepath.Join(updateTestDir, "my_update"))
	rp := &parser.RootfsParser{W: df}
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

	archive, err := WriteRootfsImageArchive(updateTestDir, RootfsImageStructOK)
	assert.NoError(t, err)
	assert.NotEqual(t, "", archive)

	// open archive file
	f, err := os.Open(archive)
	defer f.Close()
	assert.NoError(t, err)
	assert.NotNil(t, f)

	aReader := NewReader(f)
	defer aReader.Close()
	rp := &parser.RootfsParser{}
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
			rp := &parser.RootfsParser{W: df}
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
		if rp, ok := p.(*parser.RootfsParser); ok {
			assert.Equal(t, "core-image-minimal-201608110900", rp.GetImageID())
		}
	}

	data, err := ioutil.ReadFile(path.Join(updateTestDir, "my_update"))
	assert.NoError(t, err)
	assert.Equal(t, "my first update", string(data))
}
