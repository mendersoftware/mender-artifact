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

package writer

import (
	"archive/tar"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/artifacts/parsers"
	"github.com/stretchr/testify/assert"
)

type testDirEntry struct {
	Path    string
	Content []byte
	IsDir   bool
}

func MakeFakeUpdateDir(updateDir string, elements []testDirEntry) error {
	for _, elem := range elements {
		if elem.IsDir {
			if err := os.MkdirAll(path.Join(updateDir, elem.Path), os.ModeDir|os.ModePerm); err != nil {
				return err
			}
		} else {
			f, err := os.Create(path.Join(updateDir, elem.Path))
			if err != nil {
				return err
			}
			defer f.Close()
			if len(elem.Content) > 0 {
				if _, err = f.Write(elem.Content); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

var dirStructInvalid = []testDirEntry{
	{Path: "0000", IsDir: true},
	{Path: "0000/data", IsDir: true},
	{Path: "0000/type-info", IsDir: false},
	{Path: "0000/signatures", IsDir: true},
	{Path: "0000/signatures/update.sig", IsDir: false},
	{Path: "0000/scripts", IsDir: true},
	{Path: "0000/scripts/pre", IsDir: true},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},
}

func TestWriteArtifactBrokenDirStruct(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructInvalid)
	assert.NoError(t, err)

	artifactWriter := ArtifactsWriter{
		updDir: updateTestDir,
	}
	err = artifactWriter.Write()
	assert.Error(t, err)
}

// func TestGenerateHash(t *testing.T) {
// 	tempDir, _ := ioutil.TempDir("", "update")
// 	defer os.RemoveAll(tempDir)
//
// 	err := MakeFakeUpdateDir(tempDir,
// 		[]testDirEntry{
// 			{Path: "update.ext4", Content: []byte("file content"), IsDir: false},
// 			{Path: "next_update.ext3", Content: []byte("different file content"), IsDir: false},
// 		})
// 	assert.NoError(t, err)
//
// 	updates, err := ioutil.ReadDir(tempDir)
// 	assert.NoError(t, err)
// 	assert.NotNil(t, updates)
//
// 	artifactWriter := ArtifactsWriter{}
//
// 	upd := updateArtifact{path: filepath.Join(tempDir, "update.ext4")}
// 	err = artifactWriter.calculateChecksum(&upd)
// 	assert.NoError(t, err)
// 	assert.Equal(t, []byte("e0ac3601005dfa1864f5392aabaf7d898b1b5bab854f1acb4491bcd806b76b0c"), upd.checksum)
//
// 	upd = updateArtifact{path: filepath.Join(tempDir, "next_update.ext3")}
// 	err = artifactWriter.calculateChecksum(&upd)
// 	assert.NoError(t, err)
// 	assert.Equal(t, []byte("90094b71a0bf15ee00e087a3be28579483fb759a718fa4ca97be215b42021121"), upd.checksum)
//
// 	upd = updateArtifact{path: filepath.Join(tempDir, "non_existing")}
// 	err = artifactWriter.calculateChecksum(&upd)
// 	assert.Error(t, err)
// 	assert.Empty(t, upd.checksum)
// }

var dirStructOK = []testDirEntry{
	{Path: "0000", IsDir: true},
	{Path: "0000/data", IsDir: true},
	{Path: "0000/data/update.ext4", Content: []byte("first update"), IsDir: false},
	{Path: "0000/data/update_next.ext3", Content: []byte("second update"), IsDir: false},
	{Path: "0000/type-info", Content: []byte(`{"type": "rootfs-image"}`), IsDir: false},
	{Path: "0000/meta-data", Content: []byte(`{"DeviceType": "vexpress-qemu", "ImageID": "core-image-minimal-201608110900"}`), IsDir: false},
	{Path: "0000/signatures", IsDir: true},
	{Path: "0000/signatures/update.sig", IsDir: false},
	{Path: "0000/signatures/update_next.sig", IsDir: false},
	{Path: "0000/scripts", IsDir: true},
	{Path: "0000/scripts/pre", IsDir: true},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},
}

var dirStructOKAfterWriting = metadata.ArtifactHeader{
	".":                               {Path: ".", IsDir: true, Required: true},
	"data":                            {Path: "data", IsDir: true, Required: true},
	"data/0000.tar.gz":                {Path: "data", IsDir: false, Required: true},
	"0000/data":                       {Path: "0000/data", IsDir: true, Required: true},
	"0000/data/update.ext4":           {Path: "0000/data/update.ext4", IsDir: false, Required: true},
	"0000/data/update_next.ext3":      {Path: "0000/data/update_next.ext3", IsDir: false, Required: true},
	"artifact.mender":                 {Path: "artifact.mender", IsDir: false, Required: true},
	"0000":                            {Path: "0000", IsDir: true, Required: true},
	"0000/type-info":                  {Path: "0000/type-info", IsDir: false, Required: true},
	"0000/meta-data":                  {Path: "0000/meta-data", IsDir: false, Required: true},
	"0000/signatures":                 {Path: "0000/signatures", IsDir: true, Required: true},
	"0000/signatures/update.sig":      {Path: "0000/signatures/update.sig", IsDir: false, Required: true},
	"0000/signatures/update_next.sig": {Path: "0000/signatures/update_next.sig", IsDir: false, Required: true},
	"0000/scripts":                    {Path: "0000/scripts", IsDir: true, Required: true},
	"0000/scripts/pre":                {Path: "0000/scripts/pre", IsDir: true, Required: true},
	"0000/scripts/post":               {Path: "0000/scripts/post", IsDir: true, Required: true},
	"0000/scripts/check":              {Path: "0000/scripts/check", IsDir: true, Required: true},
}

func TestWriteArtifactFile(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	//defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructOK)
	assert.NoError(t, err)

	artifactWriter := NewArtifactsWriter("artifact.tar.gz", updateTestDir, "mender", 1)
	defer artifactWriter.Close()

	rp := parsers.NewRootfsParser("", nil)
	artifactWriter.Register(&rp, "rootfs-image")
	err = artifactWriter.write()
	assert.NoError(t, err)

	// check is dir structure is correct
	// err = dirStructOKAfterWriting.CheckHeaderStructure(updateTestDir)
	// assert.NoError(t, err)
}

var dirStructBroken = []testDirEntry{
	{Path: "0000", IsDir: true},
	{Path: "0000/data", IsDir: true},
	{Path: "0000/data/update.ext4", IsDir: false},
	{Path: "0000/data/update_next.ext3", IsDir: false},
	{Path: "0000/type-info", IsDir: false},
	{Path: "0000/meta-data", IsDir: false},
	{Path: "0000/signatures", IsDir: true},
	{Path: "0000/signatures/update.sig", IsDir: false},
	// signature for one file is missing
	// {Path: "0000/signatures/update_next.sig", IsDir: false},
	{Path: "0000/scripts", IsDir: true},
	{Path: "0000/scripts/pre", IsDir: true},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},
}

func TestWriteBrokenArtifact(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructBroken)
	assert.NoError(t, err)

	artifactWriter := NewArtifactsWriter("artifact", updateTestDir, "mender", 1)
	err = artifactWriter.Write()
	assert.Error(t, err)
}

type fakeArchiver struct {
	readRet   int
	readErr   error
	closeErr  error
	header    *tar.Header
	headerErr error
}

func (f fakeArchiver) Open() error                      { return nil }
func (f fakeArchiver) Read(p []byte) (n int, err error) { return f.readRet, f.readErr }
func (f fakeArchiver) Close() error                     { return f.closeErr }
func (f fakeArchiver) GetHeader() (*tar.Header, error)  { return f.header, f.headerErr }

// func TestWriteBrokenArchive(t *testing.T) {
// 	updateTestDir, _ := ioutil.TempDir("", "update")
// 	defer os.RemoveAll(updateTestDir)
// 	artifactWriter := NewArtifactsWriter("artifact", updateTestDir, "mender", 1)
//
// 	arch, err := os.Create(filepath.Join(updateTestDir, "my_archive"))
// 	assert.NoError(t, err)
// 	err = artifactWriter.writeArchive(arch, nil, false)
// 	assert.Error(t, err)
//
// 	var content []ReadArchiver
// 	content = append(content, &fakeArchiver{readRet: 0, readErr: errors.New("")})
// 	err = artifactWriter.writeArchive(arch, content, false)
// 	assert.Error(t, err)
// }
