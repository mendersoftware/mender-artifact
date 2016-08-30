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
	"path/filepath"
	"testing"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/stretchr/testify/assert"
)

func TestMarshallInfo(t *testing.T) {
	info := metadata.MetadataInfo{
		Format:  "test",
		Version: 1,
	}
	infoJSON, err := getJSON(&info)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"format":"test", "version":1}`, string(infoJSON))

	info = metadata.MetadataInfo{
		Format: "test",
	}
	infoJSON, err = getJSON(&info)
	assert.Equal(t, err, metadata.ErrInvalidInfo)
	assert.Empty(t, infoJSON)

	infoJSON, err = getJSON(nil)
	assert.NoError(t, err)
	assert.Empty(t, infoJSON)
}

var dirStructInvalid = []metadata.MetadataDirEntry{
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

func TestWriteArtifactBrokenDirStruct(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructInvalid)
	assert.NoError(t, err)

	artifactWriter := MetadataWriter{
		updateLocation:  updateTestDir,
		headerStructure: metadata.MetadataArtifactHeader{Artifacts: MetadataWriterHeaderFormat},
	}
	err = artifactWriter.Write()
	assert.Error(t, err)
}

func TestGenerateHash(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(tempDir)

	err := MakeFakeUpdateDir(tempDir,
		[]metadata.MetadataDirEntry{
			{Path: "update.ext4", IsDir: false},
			{Path: "next_update.ext3", IsDir: false},
		})
	assert.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tempDir, "update.ext4"),
		[]byte("file content"), os.ModePerm)
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempDir, "next_update.ext3"),
		[]byte("different file content"), os.ModePerm)
	assert.NoError(t, err)

	updates, err := ioutil.ReadDir(tempDir)
	assert.NoError(t, err)
	assert.NotNil(t, updates)

	//artifactWriter := MetadataWriter{}

	// hashes, err := artifactWriter.generateChecksums(tempDir, updates)
	// assert.NoError(t, err)
	// assert.NotEmpty(t, hashes)
	// assert.Len(t, hashes, 2)
	// assert.Equal(t, "e0ac3601005dfa1864f5392aabaf7d898b1b5bab854f1acb4491bcd806b76b0c", hashes["update.ext4"])
	// assert.Equal(t, "90094b71a0bf15ee00e087a3be28579483fb759a718fa4ca97be215b42021121", hashes["next_update.ext3"])
}

var dirStructOK = []metadata.MetadataDirEntry{
	{Path: "0000", IsDir: true},
	{Path: "0000/data", IsDir: true},
	{Path: "0000/data/update.ext4", IsDir: false},
	{Path: "0000/data/update_next.ext3", IsDir: false},
	{Path: "0000/type-info", IsDir: false},
	{Path: "0000/meta-data", IsDir: false},
	{Path: "0000/signatures", IsDir: true},
	{Path: "0000/signatures/update.sig", IsDir: false},
	{Path: "0000/signatures/update_next.sig", IsDir: false},
	{Path: "0000/scripts", IsDir: true},
	{Path: "0000/scripts/pre", IsDir: true},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},
}

var dirStructOKAfterWriting = map[string]metadata.MetadataDirEntry{
	".":                                                   {Path: ".", IsDir: true, Required: true},
	"data":                                                {Path: "data", IsDir: true, Required: true},
	"data/0000.tar.gz":                                    {Path: "data", IsDir: false, Required: true},
	"header":                                              {Path: "header", IsDir: true, Required: true},
	"header/headers":                                      {Path: "header/headers", IsDir: true, Required: true},
	"header/headers/0000":                                 {Path: "header/headers/0000", IsDir: true, Required: true},
	"header/headers/0000/files":                           {Path: "header/headers/0000/files", IsDir: false, Required: true},
	"header/headers/0000/type-info":                       {Path: "header/headers/0000/type-info", IsDir: false, Required: true},
	"header/headers/0000/meta-data":                       {Path: "header/headers/0000/meta-data", IsDir: false, Required: true},
	"header/headers/0000/signatures":                      {Path: "header/headers/0000/signatures", IsDir: true, Required: true},
	"header/headers/0000/signatures/update.sig":           {Path: "header/headers/0000/signatures/update.sig", IsDir: false, Required: true},
	"header/headers/0000/signatures/update_next.sig":      {Path: "header/headers/0000/signatures/update_next.sig", IsDir: false, Required: true},
	"header/headers/0000/checksums":                       {Path: "header/headers/0000/checksums", IsDir: true, Required: true},
	"header/headers/0000/checksums/update.sha256sum":      {Path: "header/headers/0000/checksums/update.sha256sum", IsDir: false, Required: true},
	"header/headers/0000/checksums/update_next.sha256sum": {Path: "header/headers/0000/checksums/update_next.sha256sum", IsDir: false, Required: true},
	"header/headers/0000/scripts":                         {Path: "header/headers/0000/scripts", IsDir: true, Required: true},
	"header/headers/0000/scripts/pre":                     {Path: "header/headers/0000/scripts/pre", IsDir: true, Required: true},
	"header/headers/0000/scripts/post":                    {Path: "header/headers/0000/scripts/post", IsDir: true, Required: true},
	"header/headers/0000/scripts/check":                   {Path: "header/headers/0000/scripts/check", IsDir: true, Required: true},
	"header/header-info":                                  {Path: "header/header-info", IsDir: false, Required: true},
	"info":                                                {Path: "info", IsDir: false, Required: true},
}

func TestWriteArtifactFile(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructOK)
	assert.NoError(t, err)

	artifactWriter := MetadataWriter{
		updateLocation:  updateTestDir,
		headerStructure: metadata.MetadataArtifactHeader{Artifacts: MetadataWriterHeaderFormat},
		format:          "mender",
		version:         1,
	}
	err = artifactWriter.Write()
	assert.NoError(t, err)

	// check is dir structure is correct
	headerAfterWrite := metadata.MetadataArtifactHeader{Artifacts: dirStructOKAfterWriting}
	err = headerAfterWrite.CheckHeaderStructure(updateTestDir)
	assert.NoError(t, err)
}
