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

package handlers

import (
	"archive/tar"
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/stretchr/testify/assert"
)

func TestHandlerRootfs(t *testing.T) {
	// test if update type is reported correctly
	r := NewRootfsV1("")
	assert.Equal(t, "rootfs-image", r.GetType())

	// test get update files
	r.update = &artifact.DataFile{Name: "update.ext4"}
	assert.Equal(t, "update.ext4", r.GetUpdateFiles()[0].Name)
	assert.Equal(t, 1, r.version)

	r = NewRootfsV2("")
	assert.Equal(t, "rootfs-image", r.GetType())

	// test get update files
	r.update = &artifact.DataFile{Name: "update_next.ext4"}
	assert.Equal(t, "update_next.ext4", r.GetUpdateFiles()[0].Name)
	assert.Equal(t, 2, r.version)
}

func TestRootfsCompose(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	f, err := ioutil.TempFile("", "update")
	assert.NoError(t, err)
	defer os.Remove(f.Name())

	r := NewRootfsV1(f.Name())
	err = r.ComposeHeader(tw, 1)
	assert.NoError(t, err)

	err = r.ComposeData(tw, 1)
	assert.NoError(t, err)
}
