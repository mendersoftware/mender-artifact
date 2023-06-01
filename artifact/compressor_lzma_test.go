// Copyright 2023 Northern.tech AS
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
//go:build !nolzma
// +build !nolzma

package artifact

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompressorLzma(t *testing.T) {
	c := NewCompressorLzma()
	assert.Equal(t, c.GetFileExtension(), ".xz")

	buf := bytes.NewBuffer(nil)
	w, err := c.NewWriter(buf)
	assert.NoError(t, err)

	i, err := w.Write([]byte(testData))
	assert.NoError(t, err)
	assert.Equal(t, i, len(testData))
	w.Close()

	r, err := c.NewReader(buf)
	assert.NoError(t, err)

	rbuf := make([]byte, len(testData))
	i, err = r.Read(rbuf)
	assert.NoError(t, err)
	assert.Equal(t, i, len(testData))
	assert.Equal(t, []byte(testData), rbuf)
}
