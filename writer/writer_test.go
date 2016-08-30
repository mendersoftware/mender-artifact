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

var dirStructOK = []metadata.MetadataDirEntry{
	{Path: "0000", IsDir: true},
	{Path: "0000/data", IsDir: true},
	{Path: "0000/data/update.ext4", IsDir: false},
	{Path: "0000/type-info", IsDir: false},
	{Path: "0000/meta-data", IsDir: false},
	{Path: "0000/signatures", IsDir: true},
	{Path: "0000/signatures/update.sig", IsDir: false},
	{Path: "0000/scripts", IsDir: true},
	{Path: "0000/scripts/pre", IsDir: true},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},
}

func MakeFakeUpdateDir(updateDir string, elements []metadata.MetadataDirEntry) error {
	for _, elem := range elements {
		if elem.IsDir {
			if err := os.MkdirAll(path.Join(updateDir, elem.Path), os.ModeDir|os.ModePerm); err != nil {
				return err
			}
		} else {
			if _, err := os.Create(path.Join(updateDir, elem.Path)); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestWriteArtifactFile(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructOK)
	assert.NoError(t, err)

	artifactWriter := MetadataWriter{
		updateLocation:  updateTestDir,
		headerStructure: metadata.MetadataArtifactHeader{Artifacts: MetadataWriterHeaderFormat},
	}
	err = artifactWriter.Write()
	assert.NoError(t, err)
}
