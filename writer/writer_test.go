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
	"testing"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/artifacts/parser"
	"github.com/stretchr/testify/assert"

	. "github.com/mendersoftware/artifacts/test_utils"
)

var dirStructInvalid = []TestDirEntry{
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

	aw := ArtifactsWriter{
		updDir: updateTestDir,
	}

	err = aw.Write()
	assert.Error(t, err)
}

var dirStructOK = []TestDirEntry{
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
	{Path: "0000/scripts/pre/0000_install.sh", Content: []byte("run me!"), IsDir: false},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},
}

var dirStructOKAfterWriting = metadata.ArtifactHeader{
	".":                                {Path: ".", IsDir: true, Required: true},
	"0000/data":                        {Path: "0000/data", IsDir: true, Required: true},
	"0000/data/update.ext4":            {Path: "0000/data/update.ext4", IsDir: false, Required: true},
	"0000/data/update_next.ext3":       {Path: "0000/data/update_next.ext3", IsDir: false, Required: true},
	"artifact.tar.gz":                  {Path: "artifact.tar.gz", IsDir: false, Required: true},
	"0000":                             {Path: "0000", IsDir: true, Required: true},
	"0000/type-info":                   {Path: "0000/type-info", IsDir: false, Required: true},
	"0000/meta-data":                   {Path: "0000/meta-data", IsDir: false, Required: true},
	"0000/signatures":                  {Path: "0000/signatures", IsDir: true, Required: true},
	"0000/signatures/update.sig":       {Path: "0000/signatures/update.sig", IsDir: false, Required: true},
	"0000/signatures/update_next.sig":  {Path: "0000/signatures/update_next.sig", IsDir: false, Required: true},
	"0000/scripts":                     {Path: "0000/scripts", IsDir: true, Required: true},
	"0000/scripts/pre":                 {Path: "0000/scripts/pre", IsDir: true, Required: true},
	"0000/scripts/pre/0000_install.sh": {Path: "0000/scripts/pre/0000_install.sh", IsDir: false, Required: true},
	"0000/scripts/post":                {Path: "0000/scripts/post", IsDir: true, Required: true},
	"0000/scripts/check":               {Path: "0000/scripts/check", IsDir: true, Required: true},
}

func TestWriteArtifactFile(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructOK)
	assert.NoError(t, err)

	aw := NewArtifactsWriter("artifact.tar.gz", updateTestDir, "mender", 1)

	rp := parser.NewRootfsParser("", nil)
	aw.Register(&rp, "rootfs-image")
	err = aw.Write()
	assert.NoError(t, err)

	// check is dir structure is correct
	err = dirStructOKAfterWriting.CheckHeaderStructure(updateTestDir)
	assert.NoError(t, err)
}

var dirStructBroken = []TestDirEntry{
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

	aw := NewArtifactsWriter("artifact", updateTestDir, "mender", 1)
	err = aw.Write()
	assert.Error(t, err)
}
