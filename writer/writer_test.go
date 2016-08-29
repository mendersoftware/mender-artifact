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
