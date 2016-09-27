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

package awriter

import (
	"io/ioutil"
	"os"
	"path/filepath"
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

	aw := NewWriter("mender", 1)
	defer func() {
		err = aw.Close()
		assert.NoError(t, err)
	}()

	err = aw.Write(updateTestDir, filepath.Join(updateTestDir, "artifact"))
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

	aw := NewWriter("mender", 1)
	defer func() {
		err = aw.Close()
		assert.NoError(t, err)
	}()

	rp := &parser.RootfsParser{}
	aw.Register(rp)

	err = aw.Write(updateTestDir, filepath.Join(updateTestDir, "artifact.tar.gz"))
	assert.NoError(t, err)

	// check is dir structure is correct
	err = dirStructOKAfterWriting.CheckHeaderStructure(updateTestDir)
	assert.NoError(t, err)
}

var dirStructOKSingle = []TestDirEntry{
	{Path: "data", IsDir: true},
	{Path: "data/update.ext4", Content: []byte("first update"), IsDir: false},
	{Path: "type-info", Content: []byte(`{"type": "rootfs-image"}`), IsDir: false},
	{Path: "meta-data", Content: []byte(`{"DeviceType": "vexpress-qemu", "ImageID": "core-image-minimal-201608110900"}`), IsDir: false},
}

func TestWriteSingleArtifactFile(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	//defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructOKSingle)
	assert.NoError(t, err)

	aw := NewWriter("mender", 1)
	defer func() {
		err = aw.Close()
		assert.NoError(t, err)
	}()

	rp := &parser.RootfsParser{}
	aw.Register(rp)

	err = aw.Write(updateTestDir, filepath.Join(updateTestDir, "artifact.tar.gz"))
	assert.NoError(t, err)
}

var dirStructMultiple = []TestDirEntry{
	{Path: "0000", IsDir: true},
	{Path: "0000/data", IsDir: true},
	{Path: "0000/data/update.ext4", Content: []byte("first update"), IsDir: false},
	{Path: "0000/type-info", Content: []byte(`{"type": "rootfs-image"}`), IsDir: false},
	{Path: "0000/meta-data", Content: []byte(`{"DeviceType": "vexpress-qemu", "ImageID": "core-image-minimal-201608110900"}`), IsDir: false},
	{Path: "0000/signatures", IsDir: true},
	{Path: "0000/signatures/update.sig", IsDir: false},
	{Path: "0000/scripts", IsDir: true},
	{Path: "0000/scripts/pre", IsDir: true},
	{Path: "0000/scripts/pre/0000_install.sh", Content: []byte("run me!"), IsDir: false},
	{Path: "0000/scripts/post", IsDir: true},
	{Path: "0000/scripts/check", IsDir: true},

	{Path: "0001", IsDir: true},
	{Path: "0001/data", IsDir: true},
	{Path: "0001/data/update_next.ext3", Content: []byte("second update"), IsDir: false},
	{Path: "0001/type-info", Content: []byte(`{"type": "rootfs-image"}`), IsDir: false},
	{Path: "0001/meta-data", Content: []byte(`{"DeviceType": "vexpress-qemu", "ImageID": "core-image-minimal-201608110900"}`), IsDir: false},
	{Path: "0001/signatures", IsDir: true},
	{Path: "0001/signatures/update_next.sig", IsDir: false},
	{Path: "0001/scripts", IsDir: true},
	{Path: "0001/scripts/pre", IsDir: true},
	{Path: "0001/scripts/pre/0000_install.sh", Content: []byte("run me!"), IsDir: false},
	{Path: "0001/scripts/post", IsDir: true},
	{Path: "0001/scripts/check", IsDir: true},
}

func TestWriteMultiple(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructMultiple)
	assert.NoError(t, err)

	aw := NewWriter("mender", 1)
	defer func() {
		err = aw.Close()
		assert.NoError(t, err)
	}()

	rp := &parser.RootfsParser{}
	aw.Register(rp)

	err = aw.Write(updateTestDir, filepath.Join(updateTestDir, "artifact.tar.gz"))
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

	aw := NewWriter("mender", 1)
	defer func() {
		err = aw.Close()
		assert.NoError(t, err)
	}()

	err = aw.Write(updateTestDir, filepath.Join(updateTestDir, "artifact.tar.gz"))
	assert.Error(t, err)
}

var dirStructCustom = []TestDirEntry{
	{Path: "my_data", IsDir: true},
	{Path: "my_data/update.ext4", Content: []byte("first update"), IsDir: false},
	{Path: "some_dir", IsDir: true},
	{Path: "some_dir/meta-data", Content: []byte(`{"DeviceType": "vexpress-qemu", "ImageID": "core-image-minimal-201608110900"}`), IsDir: false},
}

func TestWriteCustom(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)
	err := MakeFakeUpdateDir(updateTestDir, dirStructCustom)
	assert.NoError(t, err)

	aw := NewWriter("mender", 1)
	defer func() {
		err = aw.Close()
		assert.NoError(t, err)
	}()
	rp := &parser.RootfsParser{}
	aw.Register(rp)

	err = aw.WriteSingle(filepath.Join(updateTestDir, "some_dir"),
		filepath.Join(updateTestDir, "my_data/update.ext4"),
		"rootfs-image", "/tmp/mender.tar.gz")
	assert.NoError(t, err)
}
