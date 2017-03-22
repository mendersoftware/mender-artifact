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

package artifact

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	checksumData = "this is test checksum data"
	checksumSum  = "00beab03a67bac97343603854a374671e978a01a8791d1526cb75ae92967fd50"
)

func TestChecksumWrite(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriterChecksum(buf)

	n, err := w.Write([]byte(checksumData))
	assert.NoError(t, err)
	assert.Equal(t, len(checksumData), n)
	assert.Equal(t, []byte(checksumSum), w.Checksum())
	assert.Equal(t, checksumData, buf.String())

	w = NewWriterChecksum(nil)
	n, err = w.Write([]byte(checksumData))
	assert.Error(t, err)
	assert.Equal(t, 0, n)
	assert.Nil(t, w.Checksum())
}

func TestChecksumRead(t *testing.T) {
	sum := bytes.NewBuffer([]byte(checksumData))
	r := NewReaderChecksum(sum)

	n, err := r.Read([]byte(checksumData))
	assert.NoError(t, err)
	assert.Equal(t, len(checksumData), n)
	assert.Empty(t, sum.String())

	r = NewReaderChecksum(nil)
	n, err = r.Read([]byte(checksumData))
	assert.Error(t, err)
	assert.Equal(t, 0, n)

	sum = bytes.NewBuffer([]byte(checksumData))
	r = NewReaderChecksum(sum)
	n, err = r.Read(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestStore(t *testing.T) {
	s := NewChecksumStore()

	err := s.Add("test", []byte("1234567890"))
	assert.NoError(t, err)

	sum, err := s.Get("test")
	assert.NoError(t, err)
	assert.Equal(t, []byte("1234567890"), sum)

	// can not add the same sum multiple times
	err = s.Add("test", []byte("1212121212"))
	assert.Equal(t, err, os.ErrExist)

	sum, err = s.Get("test")
	assert.NoError(t, err)
	assert.Equal(t, []byte("1234567890"), sum)

	sum, err = s.Get("non-existing")
	assert.Error(t, err)
	assert.Nil(t, sum)

	raw := s.GetRaw()
	assert.Equal(t, raw, []byte("1234567890  test\n"))

	// already exists
	err = s.ReadRaw([]byte("1234567890  test\n"))
	assert.Equal(t, err, os.ErrExist)
	assert.Equal(t, []byte("1234567890  test\n"), s.GetRaw())

	err = s.ReadRaw([]byte("1212121212  version\n"))
	assert.NoError(t, err)
	assert.Equal(t, []byte("1234567890  test\n1212121212  version\n"), s.GetRaw())

	// malformed input
	err = s.ReadRaw([]byte("1212121212\n"))
	assert.Error(t, err)
	assert.Equal(t, []byte("1234567890  test\n1212121212  version\n"), s.GetRaw())
}
