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
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompressorGzip(t *testing.T) {
	c := NewCompressorGzip()
	assert.Equal(t, c.GetFileExtension(), ".gz")

	buf := bytes.NewBuffer(nil)
	w := c.NewWriter(buf)

	i, err := w.Write([]byte(testData))
	assert.NoError(t, err)
	assert.Equal(t, i, len(testData))
	w.Close()

	r, err := c.NewReader(buf)
	assert.NoError(t, err)

	rbuf := make([]byte, len(testData))
	i, err = r.Read(rbuf)
	assert.Equal(t, err, io.EOF)
	assert.Equal(t, i, len(testData))
	assert.Equal(t, []byte(testData), rbuf)
}
