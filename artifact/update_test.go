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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestUpdateUtils(t *testing.T) {
	assert.Equal(t, "data/0001", UpdatePath(1))
	assert.Equal(t, "headers/0002", UpdateHeaderPath(2))
	assert.Equal(t, "data/0003.tar.gz", UpdateDataPath(3))
}

type installer struct {
	Data *DataFile
}

func (i *installer) GetUpdateFiles() [](*DataFile) {
	return [](*DataFile){i.Data}
}

func (i *installer) GetType() string {
	return ""
}

func (i *installer) Copy() Installer {
	return i
}

func (i *installer) ReadHeader(r io.Reader, path string) error {
	return nil
}

func (i *installer) Install(r io.Reader, info *os.FileInfo) error {
	_, err := io.Copy(ioutil.Discard, r)
	return err
}

func writeDataFile(t *testing.T, name, data string) io.Reader {
	buf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	sw := NewTarWriterStream(tw)
	err := sw.Write([]byte(data), name)
	assert.NoError(t, err)
	err = tw.Close()
	assert.NoError(t, err)
	err = gz.Close()
	assert.NoError(t, err)

	return buf
}

func TestReadAndInstall(t *testing.T) {
	err := ReadAndInstall(bytes.NewBuffer(nil), nil, nil, 1)
	assert.Error(t, err)
	assert.Equal(t, "EOF", errors.Cause(err).Error())

	i := &installer{
		Data: &DataFile{
			Name: "update.ext4",
			// this is a calculated checksum of `data` string
			Checksum: []byte("3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"),
		},
	}
	r := writeDataFile(t, "update.ext4", "data")
	err = ReadAndInstall(r, i, nil, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(len("data")), i.GetUpdateFiles()[0].Size)

	// test missing data file
	i = &installer{
		Data: &DataFile{
			Name: "non-existing",
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = ReadAndInstall(r, i, nil, 1)
	assert.Error(t, err)
	assert.Equal(t, "update: can not find data file: update.ext4",
		errors.Cause(err).Error())

	// test missing checksum
	i = &installer{
		Data: &DataFile{
			Name: "update.ext4",
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = ReadAndInstall(r, i, nil, 1)
	assert.Error(t, err)
	assert.Equal(t, "update: checksum missing for file: update.ext4",
		errors.Cause(err).Error())

	// test with manifest
	m := NewChecksumStore()
	i = &installer{
		Data: &DataFile{
			Name:     "update.ext4",
			Checksum: []byte("3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"),
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = ReadAndInstall(r, i, m, 1)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "checksum missing")

	// test invalid checksum
	i = &installer{
		Data: &DataFile{
			Name:     "update.ext4",
			Checksum: []byte("12121212121212"),
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = ReadAndInstall(r, i, nil, 1)
	// assert.Error(t, err)
	// assert.Contains(t, errors.Cause(err).Error(), "invalid checksum")

	// test with manifest
	err = m.Add("update.ext4", []byte("3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"))
	assert.NoError(t, err)
	r = writeDataFile(t, "update.ext4", "data")
	err = ReadAndInstall(r, i, m, 1)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "checksum missing")
}
