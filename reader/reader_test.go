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

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/artifacts/writer"
	"github.com/stretchr/testify/assert"
)

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

func writeArchive(dir string) (string, error) {

	err := MakeFakeUpdateDir(dir, dirStructOK)
	if err != nil {
		return "", err
	}

	artifactWriter := writer.NewArtifactsWriter("artifact", dir, "mender", 1)
	err = artifactWriter.Write()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "artifact.mender"), nil
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
	assert.NoError(t, err)
	assert.NotNil(t, f)

	aReader := ArtifactsReader{artifact: f}
	err = aReader.Read()
	assert.NoError(t, err)
}
