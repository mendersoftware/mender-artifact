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
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/stretchr/testify/assert"
)

const (
	TestUpdateFileContent = "test update"
)

func MakeRootfsImageArtifact(version int, signed bool) (io.Reader, error) {
	upd, err := artifact.MakeFakeUpdate(TestUpdateFileContent)
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
	var u artifact.Composer
	switch version {
	case 1:
		u = handlers.NewRootfsV1(upd)
	case 2:
		u = handlers.NewRootfsV2(upd)
	}

	updates := &artifact.Updates{U: []artifact.Composer{u}}
	err = aw.WriteArtifact("mender", version, []string{"vexpress"},
		"mender-1.1", updates)
	if err != nil {
		return nil, err
	}
	return art, nil
}

func TestReadArtifact(t *testing.T) {

	updFileContent := bytes.NewBuffer(nil)
	copy := func(r io.Reader, f *artifact.DataFile) error {
		_, err := io.Copy(updFileContent, r)
		return err
	}

	rfh := handlers.NewRootfsInstaller()
	rfh.InstallHandler = copy

	tc := []struct {
		version   int
		signed    bool
		handler   artifact.Installer
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
