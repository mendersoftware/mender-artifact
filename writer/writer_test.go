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
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/stretchr/testify/assert"
)

func TestMarshallInfo(t *testing.T) {
	info := metadata.MetadataInfo{
		Format:  "test",
		Version: 1,
	}
	infoJSON, err := getInfoJSON(&info)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"format":"test", "version":1}`, string(infoJSON))

	info = metadata.MetadataInfo{
		Format: "test",
	}
	infoJSON, err = getInfoJSON(&info)
	assert.NoError(t, err)
	assert.Empty(t, infoJSON)

	infoJSON, err = getInfoJSON(nil)
	assert.NoError(t, err)
	assert.Empty(t, infoJSON)
}

func TestWriteInfo(t *testing.T) {
	info := metadata.MetadataInfo{
		Format:  "test",
		Version: 1,
	}
	err := WriteInfo(&info)
	assert.NoError(t, err)

	err = WriteInfo(nil)
	assert.Error(t, err)
}

func makeFakeUpdateDir(updateDir string, elements []MetadataDirEntry) error {
	for _, elem := range elements {
		if elem.isDir {
			if err := os.MkdirAll(path.Join(updateDir, elem.path), os.ModeDir|os.ModePerm); err != nil {
				return err
			}
		} else {
			if _, err := os.Create(path.Join(updateDir, elem.path)); err != nil {
				return err
			}
		}
	}
	return nil
}

var dirStructOK = []MetadataDirEntry{
	{path: "files", isDir: false},
	{path: "type-info", isDir: false},
	{path: "meta-data", isDir: false},
	{path: "checksums", isDir: true},
	{path: "checksums/image.sha", isDir: false},
	{path: "signatures", isDir: true},
	{path: "signatures/iamge.sig", isDir: false},
	{path: "scripts", isDir: true},
	{path: "scripts/pre", isDir: true},
	{path: "scripts/post", isDir: true},
	{path: "scripts/check", isDir: true},
}

var dirStructMultipleUpdates = []MetadataDirEntry{
	{path: "files", isDir: false},
	{path: "type-info", isDir: false},
	{path: "meta-data", isDir: false},
	{path: "checksums", isDir: true},
	{path: "checksums/image.sha", isDir: false},
	{path: "checksums/image_next.sha", isDir: false},
	{path: "signatures", isDir: true},
	{path: "signatures/iamge.sig", isDir: false},
	{path: "signatures/iamge_next.sig", isDir: false},
	{path: "scripts", isDir: true, required: false},
	{path: "scripts/pre", isDir: true, required: false},
	{path: "scripts/post", isDir: true, required: false},
	{path: "scripts/check", isDir: true, required: false},
}

var dirStructOKHaveScripts = []MetadataDirEntry{
	{path: "files", isDir: false},
	{path: "type-info", isDir: false},
	{path: "meta-data", isDir: false},
	{path: "checksums", isDir: true},
	{path: "checksums/image.sha", isDir: false},
	{path: "signatures", isDir: true},
	{path: "signatures/iamge.sig", isDir: false},
	{path: "scripts", isDir: true, required: false},
	{path: "scripts/pre", isDir: true, required: false},
	{path: "scripts/pre/0000_install.sh", isDir: false, required: false},
	{path: "scripts/pre/0001_install.sh", isDir: false, required: false},
	{path: "scripts/post", isDir: true, required: false},
	{path: "scripts/check", isDir: true, required: false},
}

var dirStructTypeError = []MetadataDirEntry{
	{path: "files", isDir: false},
	// type-info should be a file
	{path: "type-info", isDir: true},
	{path: "meta-data", isDir: false},
	{path: "checksums", isDir: true},
	{path: "checksums/image.sha", isDir: false},
	{path: "signatures", isDir: true},
	{path: "signatures/iamge.sig", isDir: false},
	{path: "scripts", isDir: true, required: false},
	{path: "scripts/pre", isDir: true, required: false},
	{path: "scripts/post", isDir: true, required: false},
	{path: "scripts/check", isDir: true, required: false},
}

var dirStructInvalidContent = []MetadataDirEntry{
	// can not contain unsupported elements
	{path: "not-supported", isDir: true, required: true},
	{path: "files", isDir: false},
	{path: "type-info", isDir: false},
	{path: "meta-data", isDir: false},
	{path: "checksums", isDir: true},
	{path: "checksums/image.sha", isDir: false},
	{path: "signatures", isDir: true},
	{path: "signatures/iamge.sig", isDir: false},
	{path: "scripts", isDir: true, required: false},
	{path: "scripts/pre", isDir: true, required: false},
	{path: "scripts/post", isDir: true, required: false},
	{path: "scripts/check", isDir: true, required: false},
}

var dirStructInvalidNestedDirs = []MetadataDirEntry{
	{path: "files", isDir: false},
	{path: "type-info", isDir: false},
	{path: "meta-data", isDir: false},
	{path: "checksums", isDir: true},
	{path: "checksums/image.sha", isDir: false},
	{path: "signatures", isDir: true},
	{path: "signatures/iamge.sig", isDir: false},
	{path: "scripts", isDir: true, required: false},
	{path: "scripts/pre", isDir: true, required: false},
	{path: "scripts/post", isDir: true, required: false},
	{path: "scripts/check", isDir: true, required: false},
	{path: "scripts/unsupported_dir", isDir: true, required: true},
}

var dirStructMissingRequired = []MetadataDirEntry{
	{path: "files", isDir: false},
	// does not contain meta-data and type-info
	{path: "checksums", isDir: true},
	{path: "checksums/image.sha", isDir: false},
	{path: "signatures", isDir: true},
	{path: "signatures/iamge.sig", isDir: false},
	{path: "scripts", isDir: true, required: false},
	{path: "scripts/pre", isDir: true, required: false},
	{path: "scripts/post", isDir: true, required: false},
	{path: "scripts/check", isDir: true, required: false},
}

var dirStructMissingOptional = []MetadataDirEntry{
	{path: "files", isDir: false},
	{path: "type-info", isDir: false},
	{path: "meta-data", isDir: false},
	{path: "checksums", isDir: true},
	{path: "checksums/image.sha", isDir: false},
	{path: "signatures", isDir: true},
	{path: "signatures/iamge.sig", isDir: false},
	{path: "scripts", isDir: true, required: false},
	{path: "scripts/pre", isDir: true, required: false},
}

func TestDirectoryStructure(t *testing.T) {
	var validateTests = []struct {
		dirContent []MetadataDirEntry
		err        error
	}{
		{dirStructOK, nil},
		{dirStructMultipleUpdates, nil},
		{dirStructOKHaveScripts, nil},
		{dirStructTypeError, ErrInvalidMetadataElemType},
		{dirStructInvalidContent, ErrUnsupportedElement},
		{dirStructInvalidNestedDirs, ErrUnsupportedElement},
		{dirStructMissingRequired, ErrMissingMetadataElem},
		{dirStructMissingOptional, nil},
	}

	for _, tt := range validateTests {
		updateTestDir, _ := ioutil.TempDir("", "update")
		defer os.RemoveAll(updateTestDir)
		err := makeFakeUpdateDir(updateTestDir, tt.dirContent)
		assert.NoError(t, err)

		mw := MetadataWritter{
			updateDir: updateTestDir,
		}
		err = mw.checkHeaderStructure()
		assert.Equal(t, tt.err, err)
	}
}
