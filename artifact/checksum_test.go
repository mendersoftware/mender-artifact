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
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	checksumData    = "this is test checksum data"
	sumData         = "00beab03a67bac97343603854a374671e978a01a8791d1526cb75ae92967fd50"
	checksumBigData = `ajkshfdkjahsdfjkahsdfkjhasdfkjhasdkjfaksjdfhakjsdhfaksjdh
asdfasdfjasdlfjaslkdjflkasjdflkasjdflkasjdflkajsdlfkjasldfjalksdjflkasdjflkasd
ajkshfdkjahsdfjkahsdfkjhasdfkjhasdkjfaksjdfhakjsdhfaksjdhfakjsdhfaksdjfhalksjd
asdfasdfjasdlfjaslkdjflkasjdflkasjdflkasjdflkajsdlfkjasldfjalksdjflkasdjflkasd
ajkshfdkjahsdfjkahsdfkjhasdfkjhasdkjfaksjdfhakjsdhfaksjdhfakjsdhfaksdjfhalksjd
asdfasdfjasdlfjaslkdjflkasjdflkasjdflkasjdflkajsdlfkjasldfjalksdjflkasdjflkasd
ajkshfdkjahsdfjkahsdfkjhasdfkjhasdkjfaksjdfhakjsdhfaksjdhfakjsdhfaksdjfhalksjd
asdfasdfjasdlfjaslkdjflkasjdflkasjdflkasjdflkajsdlfkjasldfjalksdjflkasdjflkasd
ajkshfdkjahsdfjkahsdfkjhasdfkjhasdkjfaksjdfhakjsdhfaksjdhfakjsdhfaksdjfhalksjd
asdfasdfjasdlfjaslkdjflkasjdflkasjdflkasjdflkajsdlfkjasldfjalksdjflkasdjflkasd
ajkshfdkjahsdfjkahsdfkjhasdfkjhasdkjfaksjdfhakjsdhfaksjdhfakjsdhfaksdjfhalksjd
asdfasdfjasdlfjaslkdjflkasjdflkasjdflkasjdflkajsdlfkjasldfjalksdjflkasdjflkasd
ajkshfdkjahsdfjkahsdfkjhasdfkjhasdkjfaksjdfhakjsdhfaksjdhfakjsdhfaksdjfhalksjd
asdfasdfjasdlfjaslkdjflkasjdflkasjdflkasjdflkajsdlfkjasldfjalksdjflkasdjflkasd
`
	sumBigData = "1a21d16c585551950f516151c5caba996140b2c8f3390dac0d8042d5d96e5216"
)

// Taken from io packege for testing purposes

// copyBuffer is the actual implementation of Copy and CopyBuffer.
// if buf is nil, one is allocated.
func copyBuffer(dst io.Writer, src io.Reader) (written int64, err error) {
	buf := make([]byte, 128)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}

func TestChecksumWrite(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriterChecksum(buf)

	data := bytes.NewBuffer([]byte(checksumData))
	n, err := io.Copy(w, data)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(checksumData)), n)
	assert.Equal(t, []byte(sumData), w.Checksum())
	assert.Equal(t, checksumData, buf.String())

	w = NewWriterChecksum(nil)
	i, err := w.Write([]byte(checksumData))
	assert.Error(t, err)
	assert.Equal(t, 0, i)
	assert.Nil(t, w.Checksum())
}

func TestChecksumRead(t *testing.T) {
	sum := bytes.NewBuffer([]byte(checksumData))
	r := NewReaderChecksum(sum, []byte(sumData))
	n, err := io.Copy(ioutil.Discard, r)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(checksumData)), n)

	r = NewReaderChecksum(nil, nil)
	n, err = io.Copy(ioutil.Discard, r)
	assert.Error(t, err)
	assert.Zero(t, n)

	sum = bytes.NewBuffer([]byte(checksumData))
	r = NewReaderChecksum(sum, []byte("12121212"))
	_, err = io.Copy(ioutil.Discard, r)
	assert.Error(t, err)
}

func TestChecksumReadBigData(t *testing.T) {
	sum := bytes.NewBuffer([]byte(checksumBigData))
	r := NewReaderChecksum(sum, []byte(sumBigData))

	n, err := copyBuffer(ioutil.Discard, r)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(checksumBigData)), n)
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
