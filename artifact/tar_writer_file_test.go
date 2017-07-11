// Copyright 2017 Northern.tech AS
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
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTarFile(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)

	fa := NewTarWriterFile(tw)
	err := fa.Write(nil, "my_file")
	assert.Error(t, err)

	f, err := ioutil.TempFile("", "test")
	assert.NoError(t, err)
	assert.NotNil(t, f)
	defer os.Remove(f.Name())

	_, err = f.WriteString("some data")
	assert.NoError(t, err)
	_, err = f.Seek(0, 0)
	assert.NoError(t, err)

	err = fa.Write(f, "my_file")
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
