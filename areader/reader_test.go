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

package areader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	TestUpdateFileContent = "test update"
)

func MakeRootfsImageArtifact(version int, signed bool) (io.Reader, error) {
	upd, err := MakeFakeUpdate(TestUpdateFileContent)
	if err != nil {
		return nil, err
	}
	defer os.Remove(upd)

	art := bytes.NewBuffer(nil)
	var aw *awriter.Writer
	if !signed {
		aw = awriter.NewWriter(art)
	} else {
		aw = awriter.NewWriterSigned(art, new(artifact.DummySigner))
	}
	var u handlers.Composer
	switch version {
	case 1:
		u = handlers.NewRootfsV1(upd)
	case 2:
		u = handlers.NewRootfsV2(upd)
	}
	updates := &awriter.Updates{U: []handlers.Composer{u}}
	err = aw.WriteArtifact("mender", version, []string{"vexpress"},
		"mender-1.1", updates)
	if err != nil {
		return nil, err
	}
	return art, nil
}

func TestReadArtifact(t *testing.T) {

	updFileContent := bytes.NewBuffer(nil)
	copy := func(r io.Reader, f *handlers.DataFile) error {
		_, err := io.Copy(updFileContent, r)
		return err
	}

	rfh := handlers.NewRootfsInstaller()
	rfh.InstallHandler = copy

	tc := []struct {
		version   int
		signed    bool
		handler   handlers.Installer
		verifier  artifact.Verifier
		readError error
	}{
		{1, false, rfh, nil, nil},
		{2, false, rfh, nil, nil},
		{2, true, rfh, new(artifact.DummySigner), nil},
		// test that we need a verifier for signed artifact
		{2, true, rfh, nil, errors.New("reader: verify signature callback not registered")},
	}

	// first create archive, that we will be able to read
	for _, test := range tc {
		art, err := MakeRootfsImageArtifact(test.version, test.signed)
		assert.NoError(t, err)

		aReader := NewReader(art)
		if test.handler != nil {
			aReader.RegisterHandler(test.handler)
		}

		if test.verifier != nil {
			aReader.VerifySignatureCallback = test.verifier.Verify
		}

		err = aReader.ReadArtifact()
		if test.readError != nil {
			assert.Equal(t, test.readError.Error(), err.Error())
			continue
		}
		assert.NoError(t, err)
		assert.Equal(t, TestUpdateFileContent, updFileContent.String())

		devComp := aReader.GetCompatibleDevices()
		assert.Len(t, devComp, 1)
		assert.Equal(t, "vexpress", devComp[0])

		if test.handler != nil {
			assert.Len(t, aReader.GetHandlers(), 1)
			assert.Equal(t, test.handler.GetType(), aReader.GetHandlers()[0].GetType())
		}
		assert.Equal(t, "mender-1.1", aReader.GetArtifactName())

		// clean the buffer
		updFileContent.Reset()
	}
}

func TestRegisterMultipleHandlers(t *testing.T) {
	aReader := NewReader(nil)
	err := aReader.RegisterHandler(handlers.NewRootfsInstaller())
	assert.NoError(t, err)

	err = aReader.RegisterHandler(handlers.NewRootfsInstaller())
	assert.Error(t, err)

	err = aReader.RegisterHandler(nil)
	assert.Error(t, err)
}

func TestReadNoHandler(t *testing.T) {
	art, err := MakeRootfsImageArtifact(1, false)
	assert.NoError(t, err)

	aReader := NewReader(art)
	err = aReader.ReadArtifact()
	assert.NoError(t, err)

	assert.Len(t, aReader.GetHandlers(), 1)
	assert.Equal(t, "rootfs-image", aReader.GetHandlers()[0].GetType())
}

func TestReadBroken(t *testing.T) {
	broken := []byte("this is broken artifact")
	buf := bytes.NewBuffer(broken)

	aReader := NewReader(buf)
	err := aReader.ReadArtifact()
	assert.Error(t, err)

	aReader = NewReader(nil)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
}

func MakeFakeUpdate(data string) (string, error) {
	f, err := ioutil.TempFile("", "test_update")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if len(data) > 0 {
		if _, err := f.WriteString(data); err != nil {
			return "", err
		}
	}
	return f.Name(), nil
}

type installer struct {
	Data *handlers.DataFile
}

func (i *installer) GetUpdateFiles() [](*handlers.DataFile) {
	return [](*handlers.DataFile){i.Data}
}

func (i *installer) GetType() string {
	return ""
}

func (i *installer) Copy() handlers.Installer {
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
	sw := artifact.NewTarWriterStream(tw)
	err := sw.Write([]byte(data), name)
	assert.NoError(t, err)
	err = tw.Close()
	assert.NoError(t, err)
	err = gz.Close()
	assert.NoError(t, err)

	return buf
}

func TestReadAndInstall(t *testing.T) {
	err := readAndInstall(bytes.NewBuffer(nil), nil, nil, 1)
	assert.Error(t, err)
	assert.Equal(t, "EOF", errors.Cause(err).Error())

	i := &installer{
		Data: &handlers.DataFile{
			Name: "update.ext4",
			// this is a calculated checksum of `data` string
			Checksum: []byte("3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"),
		},
	}
	r := writeDataFile(t, "update.ext4", "data")
	err = readAndInstall(r, i, nil, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(len("data")), i.GetUpdateFiles()[0].Size)

	// test missing data file
	i = &installer{
		Data: &handlers.DataFile{
			Name: "non-existing",
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = readAndInstall(r, i, nil, 1)
	assert.Error(t, err)
	assert.Equal(t, "update: can not find data file: update.ext4",
		errors.Cause(err).Error())

	// test missing checksum
	i = &installer{
		Data: &handlers.DataFile{
			Name: "update.ext4",
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = readAndInstall(r, i, nil, 1)
	assert.Error(t, err)
	assert.Equal(t, "update: checksum missing for file: update.ext4",
		errors.Cause(err).Error())

	// test with manifest
	m := artifact.NewChecksumStore()
	i = &installer{
		Data: &handlers.DataFile{
			Name:     "update.ext4",
			Checksum: []byte("3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"),
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = readAndInstall(r, i, m, 1)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "checksum missing")

	// test invalid checksum
	i = &installer{
		Data: &handlers.DataFile{
			Name:     "update.ext4",
			Checksum: []byte("12121212121212"),
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = readAndInstall(r, i, nil, 1)
	// assert.Error(t, err)
	// assert.Contains(t, errors.Cause(err).Error(), "invalid checksum")

	// test with manifest
	err = m.Add("update.ext4", []byte("3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"))
	assert.NoError(t, err)
	r = writeDataFile(t, "update.ext4", "data")
	err = readAndInstall(r, i, m, 1)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "checksum missing")
}
