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

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestHandlerGeneric(t *testing.T) {
	// test if update type is reported correctly
	uType := "custom-type"
	g := NewGeneric(uType)
	assert.Equal(t, uType, g.GetType())

	// test we can not copy generic parser
	assert.Nil(t, g.Copy())

	// test get update files
	g.files["custom"] = &DataFile{Name: "update.ext4"}
	assert.Len(t, g.GetUpdateFiles(), 1)
	assert.Equal(t, "update.ext4", g.GetUpdateFiles()[0].Name)
}

func TestReadData(t *testing.T) {
	buf := bytes.NewBuffer([]byte("data"))
	g := NewGeneric("custom")

	err := g.Install(buf, nil)
	assert.NoError(t, err)
}

func TestReadHeader(t *testing.T) {
	g := NewGeneric("custom")

	tc := []struct {
		data      string
		version   int
		name      string
		shouldErr bool
		errMsg    string
	}{
		{data: "invalid", name: "headers/0000/files", shouldErr: true,
			errMsg: "error validating data", version: 2},
		{data: `{"files":["update.ext4", "next_update.ext4"]}`,
			name: "headers/0000/files", shouldErr: false, version: 2},
		{data: `1212121212121212121212121212`,
			name: "headers/0000/checksums/update.ext4.sum", shouldErr: false, version: 2},
		{data: "", name: "headers/0000/non-existing", shouldErr: true,
			errMsg: "unsupported file"},
		{data: "data", name: "headers/0000/type-info", shouldErr: false, version: 2},
		{data: "", name: "headers/0000/meta-data", shouldErr: false},
		{data: "", name: "headers/0000/scripts/pre/my_script", shouldErr: false, version: 2},
		{data: "", name: "headers/0000/scripts/post/my_script", shouldErr: false, version: 2},
		{data: "", name: "headers/0000/scripts/check/my_script", shouldErr: false, version: 2},
		{data: "", name: "headers/0000/signatures/update.sig", shouldErr: false, version: 2},
		// Version 3 specifics:
		{data: `{"files":["update.ext4", "next_update.ext4"]}`, name: "headers/0000/files", shouldErr: true, version: 3},
	}

	for _, test := range tc {
		buf := bytes.NewBuffer(nil)

		tw := tar.NewWriter(buf)
		err := tw.WriteHeader(&tar.Header{
			Name: "not-needed",
			Size: int64(len(test.data)),
		})
		assert.NoError(t, err)
		_, err = tw.Write([]byte(test.data))
		assert.NoError(t, err)
		err = tw.Close()
		assert.NoError(t, err)

		tr := tar.NewReader(buf)
		_, err = tr.Next()
		assert.NoError(t, err)

		err = g.ReadHeader(buf, test.name, test.version)
		if test.shouldErr {
			assert.Error(t, err)
			if test.errMsg != "" {
				assert.Contains(t, errors.Cause(err).Error(), test.errMsg)
			}
			// Second read in version 3 should accept files in the header.
			if test.version == 3 {
				err = g.ReadHeader(bytes.NewReader([]byte(test.data)), test.name, test.version)
				assert.NoError(t, err)
			}
		} else {
			assert.NoError(t, err)
		}
	}
}
