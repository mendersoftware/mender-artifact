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
	"archive/tar"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTarStream(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)

	sa := NewTarWriterStream(tw)
	err := sa.Write([]byte("some data"), "my_file")
	assert.NoError(t, err)

	err = tw.Close()
	assert.NoError(t, err)

	tr := tar.NewReader(buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)
		assert.Equal(t, "my_file", hdr.Name)

		data := bytes.NewBuffer(nil)
		n, err := io.Copy(data, tr)
		assert.NoError(t, err)
		assert.Len(t, "some data", int(n))
		assert.Equal(t, "some data", data.String())
	}
}

func TestToStream(t *testing.T) {
	info := &Info{Version: 3, Format: "custom"}
	s, err := ToStream(info)
	require.Nil(t, err)
	assert.Equal(t, []byte(`{"format":"custom","version":3}`), s)

	info = &Info{Format: "custom"}
	s, err = ToStream(info)
	require.Error(t, err)
	assert.Nil(t, s)
}
