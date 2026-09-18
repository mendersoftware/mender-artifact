// Copyright 2020 Northern.tech AS
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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTarFile(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)

	fa := NewTarWriterFile(tw)
	err := fa.Write(nil, "my_file")
	assert.Error(t, err)

	f, err := os.CreateTemp("", "test")
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

// tarFileWithMtime archives a temporary file stamped with mtime and returns
// the resulting tar stream.
func tarFileWithMtime(t *testing.T, mtime time.Time) []byte {
	t.Helper()

	f, err := os.CreateTemp("", "test")
	assert.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString("some data")
	assert.NoError(t, err)
	assert.NoError(t, f.Close())
	assert.NoError(t, os.Chtimes(f.Name(), mtime, mtime))

	f, err = os.Open(f.Name())
	assert.NoError(t, err)
	defer f.Close()

	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)
	assert.NoError(t, NewTarWriterFile(tw).Write(f, "my_file"))
	assert.NoError(t, tw.Close())

	return buf.Bytes()
}

func firstHeader(t *testing.T, archive []byte) *tar.Header {
	t.Helper()

	hdr, err := tar.NewReader(bytes.NewReader(archive)).Next()
	assert.NoError(t, err)
	return hdr
}

// unsetSourceDateEpoch clears the variable for the duration of the test,
// restoring whatever the ambient environment had.
func unsetSourceDateEpoch(t *testing.T) {
	t.Helper()

	if v, ok := os.LookupEnv(sourceDateEpochVar); ok {
		t.Setenv(sourceDateEpochVar, v)
		assert.NoError(t, os.Unsetenv(sourceDateEpochVar))
	}
}

func TestTarFileSourceDateEpoch(t *testing.T) {
	t.Setenv(sourceDateEpochVar, "1000000000")

	// Same content, different mtimes: the archives must still match byte for
	// byte, which is the property the whole change exists to provide.
	first := tarFileWithMtime(t, time.Unix(1500000000, 0))
	second := tarFileWithMtime(t, time.Unix(1600000000, 0))
	assert.Equal(t, first, second)

	hdr := firstHeader(t, first)
	assert.Equal(t, time.Unix(1000000000, 0).UTC(), hdr.ModTime.UTC())
	assert.Zero(t, hdr.Uid)
	assert.Zero(t, hdr.Gid)
	assert.Empty(t, hdr.Uname)
	assert.Empty(t, hdr.Gname)
}

func TestTarFileWithoutSourceDateEpoch(t *testing.T) {
	unsetSourceDateEpoch(t)

	mtime := time.Unix(1500000000, 0)
	hdr := firstHeader(t, tarFileWithMtime(t, mtime))

	// Unset means unchanged: the file's own mtime is still what lands in the
	// header.
	assert.Equal(t, mtime.UTC(), hdr.ModTime.UTC())
}

func TestTarFileInvalidSourceDateEpoch(t *testing.T) {
	t.Setenv(sourceDateEpochVar, "not-a-timestamp")

	f, err := os.CreateTemp("", "test")
	assert.NoError(t, err)
	defer os.Remove(f.Name())
	defer f.Close()

	tw := tar.NewWriter(bytes.NewBuffer(nil))
	err = NewTarWriterFile(tw).Write(f, "my_file")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), sourceDateEpochVar)
}
