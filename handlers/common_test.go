// Copyright 2018 Northern.tech AS
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
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/stretchr/testify/assert"
)

func TestWriteTypeInfoV3(t *testing.T) {
	// Write type info - success.
	buf := bytes.NewBuffer(nil)
	err := writeTypeInfoV3(&WriteInfoArgs{
		tarWriter:  tar.NewWriter(buf),
		typeinfov3: &artifact.TypeInfoV3{Type: "foobar"},
	})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), `"type":"foobar"`)

	// Fail write
	err = writeTypeInfoV3(&WriteInfoArgs{
		tarWriter: tar.NewWriter(&TestErrWriter{}),
	})
	assert.Contains(t, err.Error(), "unexpected EOF")
}

func TestWriteTypeInfo(t *testing.T) {
	// write update type.
	buf := bytes.NewBuffer(nil)
	err := writeTypeInfo(tar.NewWriter(buf), "rootfs-image", "")
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "rootfs-image")
	// Fail write.
	err = writeTypeInfo(tar.NewWriter(new(TestErrWriter)), "rootfs-image", "")
	assert.Contains(t, err.Error(), "unexpected EOF")
}
