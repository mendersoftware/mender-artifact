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

package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testData = "this is test data"
)

func TestCompressorFromId(t *testing.T) {
	c, err := NewCompressorFromId("gzip")
	assert.NoError(t, err, nil)
	assert.Equal(t, c.GetFileExtension(), ".gz")

	c, err = NewCompressorFromId("none")
	assert.NoError(t, err, nil)
	assert.Equal(t, c.GetFileExtension(), "")

	c, err = NewCompressorFromId("fake")
	assert.Error(t, err)
}

func TestCompressorFromFileName(t *testing.T) {
	c, err := NewCompressorFromFileName("file.tar.gz")
	assert.NoError(t, err, nil)
	assert.Equal(t, c.GetFileExtension(), ".gz")

	c, err = NewCompressorFromFileName("file.tar")
	assert.NoError(t, err, nil)
	assert.Equal(t, c.GetFileExtension(), "")

	c, err = NewCompressorFromFileName("file.tar.foo")
	assert.NoError(t, err, nil)
	assert.Equal(t, c.GetFileExtension(), "")
}

func TestRegisteredCompressors(t *testing.T) {
	compressorIds := GetRegisteredCompressorIds()

	// Verify the list contains all non-optional compressors
	assert.True(t, len(compressorIds) >= 2)
	assert.Contains(t, compressorIds, "none")
	assert.Contains(t, compressorIds, "gzip")

	// Verify 'none' is at the beginning of the list
	assert.Equal(t, "none", compressorIds[0])
}
