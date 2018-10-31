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

package areader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	TestUpdateFileContent = "test update"
	PublicKey             = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDSTLzZ9hQq3yBB+dMDVbKem6ia
v1J6opg6DICKkQ4M/yhlw32BCGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKc
XwaUNml5EhW79AdibBXZiZt8fMhCjUd/4ce3rLNjnbIn1o9L6pzV4CcVJ8+iNhne
5vbA+63vRCnrc8QuYwIDAQAB
-----END PUBLIC KEY-----`
	PrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQDSTLzZ9hQq3yBB+dMDVbKem6iav1J6opg6DICKkQ4M/yhlw32B
CGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKcXwaUNml5EhW79AdibBXZiZt8
fMhCjUd/4ce3rLNjnbIn1o9L6pzV4CcVJ8+iNhne5vbA+63vRCnrc8QuYwIDAQAB
AoGAQKIRELQOsrZsxZowfj/ia9jPUvAmO0apnn2lK/E07k2lbtFMS1H4m1XtGr8F
oxQU7rLyyP/FmeJUqJyRXLwsJzma13OpxkQtZmRpL9jEwevnunHYJfceVapQOJ7/
6Oz0pPWEq39GCn+tTMtgSmkEaSH8Ki9t32g9KuQIKBB2hbECQQDsg7D5fHQB1BXG
HJm9JmYYX0Yk6Z2SWBr4mLO0C4hHBnV5qPCLyevInmaCV2cOjDZ5Sz6iF5RK5mw7
qzvFa8ePAkEA46Anom3cNXO5pjfDmn2CoqUvMeyrJUFL5aU6W1S6iFprZ/YwdHcC
kS5yTngwVOmcnT65Vnycygn+tZan2A0h7QJBAJNlowZovDdjgEpeCqXp51irD6Dz
gsLwa6agK+Y6Ba0V5mJyma7UoT//D62NYOmdElnXPepwvXdMUQmCtpZbjBsCQD5H
VHDJlCV/yzyiJz9+tZ5giaAkO9NOoUBsy6GvdfXWn2prXmiPI0GrrpSvp7Gj1Tjk
r3rtT0ysHWd7l+Kx/SUCQGlitd5RDfdHl+gKrCwhNnRG7FzRLv5YOQV81+kh7SkU
73TXPIqLESVrqWKDfLwfsfEpV248MSRou+y0O1mtFpo=
-----END RSA PRIVATE KEY-----
`

	PublicKeyError = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDSTLzZ9hQq3yBB+dMDVbKem6ia
v1J6opg6DICKkQ4M/yhlw32BCGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKc
XwaUNml5EhW79AdibBXZiZt8fMhCjUd/4ce3rLNjnbIn1o9L6pzV4CcVJ8+iNhne
5vbA+63vRCnrc8QuYwIDAQAC
-----END PUBLIC KEY-----`
)

func MakeRootfsImageArtifact(version int, signed bool,
	hasScripts bool) (io.Reader, error) {

	upd, err := MakeFakeUpdate(TestUpdateFileContent)
	if err != nil {
		return nil, err
	}
	defer os.Remove(upd)

	var u handlers.Composer
	switch version {
	case 1:
		u = handlers.NewRootfsV1(upd)
	case 2:
		u = handlers.NewRootfsV2(upd)
	case 3:
		u = handlers.NewRootfsV3(upd)
	default:
		return nil, fmt.Errorf("Unsupported artifact version: %d", version)
	}

	return MakeAnyImageArtifact(version, signed, hasScripts, u)
}

func MakeModuleImageArtifact(signed, hasScripts bool, updateType string,
	numFiles, numAugmentFiles int) (io.Reader, error) {

	compose := handlers.NewModuleImage(updateType)

	files := make([]*handlers.DataFile, numFiles)
	for index := 0; index < numFiles; index++ {
		file, err := MakeFakeUpdate("test-file")
		if err != nil {
			return nil, err
		}
		files[index] = &handlers.DataFile{Name: file}
	}
	compose.SetUpdateFiles(files)

	augmentFiles := make([]*handlers.DataFile, numAugmentFiles)
	for index := 0; index < numAugmentFiles; index++ {
		file, err := MakeFakeUpdate("test-file")
		if err != nil {
			return nil, err
		}
		augmentFiles[index] = &handlers.DataFile{Name: file}
	}
	compose.SetUpdateAugmentFiles(augmentFiles)

	return MakeAnyImageArtifact(3, signed, hasScripts, compose)
}

func MakeAnyImageArtifact(version int, signed bool,
	hasScripts bool, u handlers.Composer) (io.Reader, error) {

	art := bytes.NewBuffer(nil)
	var aw *awriter.Writer
	if !signed {
		aw = awriter.NewWriter(art)
	} else {
		s := artifact.NewSigner([]byte(PrivateKey))
		aw = awriter.NewWriterSigned(art, s)
	}

	scr := artifact.Scripts{}
	if hasScripts {
		s, err := ioutil.TempFile("", "ArtifactInstall_Enter_10_")
		if err != nil {
			return nil, err
		}
		defer os.Remove(s.Name())

		_, err = io.WriteString(s, "execute me!")

		if err := scr.Add(s.Name()); err != nil {
			return nil, err
		}
	}

	updates := &awriter.Updates{U: []handlers.Composer{u}}

	err := aw.WriteArtifact(&awriter.WriteArtifactArgs{
		Format:  "mender",
		Version: version,
		Devices: []string{"vexpress"},
		Name:    "mender-1.1",
		Updates: updates,
		Scripts: &scr,
		// Version 3 specifics:
		Provides: &artifact.ArtifactProvides{
			ArtifactName:         "mender-1.1",
			ArtifactGroup:        "group-1",
			SupportedUpdateTypes: []string{"rootfs-image", "delta"},
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"mender-1.0"},
			CompatibleDevices: []string{"vexpress"},
		},
	})
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

	rfh := func(version int) handlers.Installer {
		rfh := handlers.NewRootfsInstaller()
		rfh.InstallHandler = copy
		return rfh
	}

	tc := map[string]struct {
		version   int
		signed    bool
		handler   handlers.Installer
		verifier  artifact.Verifier
		readError error
	}{
		"version 1":        {1, false, rfh(1), nil, nil},
		"version 2 pass":   {2, false, rfh(1), nil, nil},
		"version 2 signed": {2, true, rfh(2), artifact.NewVerifier([]byte(PublicKey)), nil},
		"version 2 - public key error": {2, true, rfh(2), artifact.NewVerifier([]byte(PublicKeyError)),
			errors.New("reader: invalid signature: crypto/rsa: verification error")},
		// test that we do not need a verifier for signed artifact
		"version 2 - no verifier needed for a signed artifact": {2, true, rfh(2), nil, nil},
		// Version 3 tests.
		"version 3 - base case": {3, false, rfh(3), nil, nil},
		"version 3 - signed":    {3, true, rfh(3), artifact.NewVerifier([]byte(PublicKey)), nil},
		"version 3 - public key error": {3, true, rfh(3), artifact.NewVerifier([]byte(PublicKeyError)),
			errors.New("readHeaderV3: reader: invalid signature: crypto/rsa: verification error")},
	}

	// first create archive, that we will be able to read
	for name, test := range tc {
		art, err := MakeRootfsImageArtifact(test.version, test.signed, false)
		assert.NoError(t, err)

		aReader := NewReader(art)
		if test.handler != nil {
			require.NoError(t, aReader.RegisterHandler(test.handler))
		}

		if test.verifier != nil {
			aReader.VerifySignatureCallback = test.verifier.Verify
		}

		err = aReader.ReadArtifact()
		if test.readError != nil {
			assert.NotNil(t, err, name+"Test expected an error, but ReadArtifact did not return an error")
			assert.Equal(t, test.readError.Error(), err.Error())
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, TestUpdateFileContent, updFileContent.String())

		devComp := aReader.GetCompatibleDevices()
		require.Len(t, devComp, 1)
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

func TestReadSigned(t *testing.T) {
	art, err := MakeRootfsImageArtifact(2, true, false)
	assert.NoError(t, err)
	aReader := NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: verify signature callback not registered")

	art, err = MakeRootfsImageArtifact(2, false, false)
	assert.NoError(t, err)
	aReader = NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: expecting signed artifact, but no signature file found")

	art, err = MakeRootfsImageArtifact(2, true, false)
	assert.NoError(t, err)
	aReader = NewReader(art)
	err = aReader.ReadArtifact()
	assert.NoError(t, err)

	art, err = MakeRootfsImageArtifact(1, false, false)
	assert.NoError(t, err)
	aReader = NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: expecting signed artifact")

	art, err = MakeRootfsImageArtifact(3, true, false)
	assert.NoError(t, err)
	aReader = NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: verify signature callback not registered")

	art, err = MakeRootfsImageArtifact(3, false, false)
	assert.NoError(t, err)
	aReader = NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: expecting signed artifact, but no signature file found")

	art, err = MakeRootfsImageArtifact(3, true, false)
	assert.NoError(t, err)
	aReader = NewReader(art)
	err = aReader.ReadArtifact()
	assert.NoError(t, err)
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
	art, err := MakeRootfsImageArtifact(1, false, false)
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

func TestReadWithScripts(t *testing.T) {
	art, err := MakeRootfsImageArtifact(2, false, true)
	assert.NoError(t, err)

	aReader := NewReader(art)

	noExec := 0
	aReader.ScriptsReadCallback = func(r io.Reader, info os.FileInfo) error {
		noExec++

		assert.Contains(t, info.Name(), "ArtifactInstall_Enter_10_")

		buf := bytes.NewBuffer(nil)
		_, err = io.Copy(buf, r)
		assert.NoError(t, err)
		assert.Equal(t, "execute me!", buf.String())
		return nil
	}

	err = aReader.ReadArtifact()
	assert.NoError(t, err)
	assert.Equal(t, 1, noExec)
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
	t *testing.T
	Data *handlers.DataFile
	updateType string
}

func (i *installer) GetUpdateFiles() [](*handlers.DataFile) {
	return [](*handlers.DataFile){i.Data}
}

func (i *installer) GetUpdateAugmentFiles() [](*handlers.DataFile) {
	return make([](*handlers.DataFile), 0)
}

func (i *installer) GetUpdateAllFiles() [](*handlers.DataFile) {
	return i.GetUpdateFiles()
}

func (i *installer) SetUpdateFiles(files [](*handlers.DataFile)) error {
	// Should not be called in this test.
	i.t.Fail()
	return nil
}

func (i *installer) SetUpdateAugmentFiles(files [](*handlers.DataFile)) error {
	// Should not be called in this test.
	i.t.Fail()
	return nil
}

func (i *installer) GetUpdateDepends() *artifact.TypeInfoDepends {
	return &artifact.TypeInfoDepends{}
}

func (i *installer) GetUpdateProvides() *artifact.TypeInfoProvides {
	return &artifact.TypeInfoProvides{}
}

func (i *installer) GetType() string {
	return i.updateType
}

func (i *installer) Copy() handlers.Installer {
	return i
}

func (i *installer) ReadHeader(r io.Reader, path string, version int, augmented bool) error {
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
		t: t,
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
		t: t,
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
		t: t,
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
		t: t,
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
		t: t,
		Data: &handlers.DataFile{
			Name:     "update.ext4",
			Checksum: []byte("12121212121212"),
		},
	}
	r = writeDataFile(t, "update.ext4", "data")
	err = readAndInstall(r, i, nil, 1)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "invalid checksum")

	// test with manifest
	err = m.Add("update.ext4", []byte("3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"))
	assert.NoError(t, err)
	r = writeDataFile(t, "update.ext4", "data")
	err = readAndInstall(r, i, m, 1)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "checksum missing")
}

func TestValidParsePathV3(t *testing.T) {
	tests := map[string]struct {
		path      []string
		nextToken string
		validPath bool
		err       error
	}{

		"Unsigned": {
			path:      []string{"manifest", "manifest-augment", "header.tar.gz", "header-augment.tar.gz"},
			nextToken: "header-augment.tar.gz",
			validPath: true,
		},
		"Signed": {
			path:      []string{"manifest", "manifest.sig", "manifest-augment", "header.tar.gz", "header-augment.tar.gz"},
			nextToken: "header-augment.tar.gz",
			validPath: true,
		},
		"Missing manifest": {
			path:      []string{"header.tar.gz"},
			err:       errParseOrder,
			validPath: false,
		},
		"manifest means we're still on a valid path, but path is not finished": {
			path:      []string{"manifest"},
			nextToken: "manifest",
			validPath: false,
		},
		"Manifest-augment missing": {
			path:      []string{"manifest", "manifest.sig", "header.tar.gz", "header-augment.tar.gz"},
			err:       errParseOrder,
			validPath: false,
		},
		"Header.tar.gz is still on a valid path, we're not finished, as the artifact is augmented": {
			path:      []string{"manifest", "manifest.sig", "manifest-augment", "header.tar.gz"},
			nextToken: "header.tar.gz",
			validPath: false,
		},
	}
	for name, test := range tests {
		res, validPath, err := verifyParseOrder(test.path)
		if err != nil {
			assert.EqualError(t, err, test.err.Error())
		}
		assert.Contains(t, res, test.nextToken, "%q: Failed to verify the order the artifact was parsed", name)
		assert.Equal(t, test.validPath, validPath, "%q: ValidPath values are wrong", name)
	}
}

func TestReadArtifactDependsAndProvides(t *testing.T) {
	art, err := MakeRootfsImageArtifact(3, true, false)
	require.NoError(t, err)
	ar := NewReader(art)
	err = ar.ReadArtifact()
	require.NoError(t, err)

	assert.Equal(t, ar.GetInfo(), artifact.Info{Format: "mender", Version: 3})
	assert.Equal(t, *ar.GetArtifactProvides(), artifact.ArtifactProvides{ArtifactName: "mender-1.1",
		ArtifactGroup: "group-1", SupportedUpdateTypes: []string{"rootfs-image", "delta"}})
	assert.Equal(t, *ar.GetArtifactDepends(), artifact.ArtifactDepends{
		ArtifactName:      []string{"mender-1.0"},
		CompatibleDevices: []string{"vexpress"}})
}

func assembleSubset(flags []string, tmpdir string) []string {
	assembleOrder := []string{
		"version",
		"manifest",
		"manifest.sig",
		"manifest-augment",
		"header.tar.gz",
		"header-augment.tar.gz",
		"data/0000.tar.gz",
		"data/0001.tar.gz",
	}
	subset := make([]string, len(flags), len(flags)+len(assembleOrder))
	copy(subset, flags)
	for index := range assembleOrder {
		if _, err := os.Stat(filepath.Join(tmpdir, assembleOrder[index])); !os.IsNotExist(err) {
			subset = append(subset, assembleOrder[index])
		}
	}
	return subset
}

func TestReadBrokenArtifact(t *testing.T) {
	type testCase struct {
		manipulateArtifact func(tmpdir string)
		successful bool
		errorStr string
		inst *installer
		rootfsImage bool
		numFiles int
		numAugmentFiles int
	}
	cases := map[string]testCase{
		"rootfs-image, Everything ok": {
			manipulateArtifact: func(tmpdir string) {
				// Do nothing. Just checking that the tar cmd works.
			},
			successful: true,
			rootfsImage: true,
		},
		"rootfs-image, version missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "version"))
			},
			successful: false,
			rootfsImage: true,
		},
		"rootfs-image, header.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "header.tar.gz"))
			},
			successful: false,
			rootfsImage: true,
		},
		"rootfs-image, header-augment.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "header-augment.tar.gz"))
			},
			successful: false,
			rootfsImage: true,
		},
		"Everything ok": {
			manipulateArtifact: func(tmpdir string) {
				// Do nothing. Just checking that the tar cmd works.
			},
			successful: true,
		},
		"version missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "version"))
			},
			successful: false,
		},
		"header.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "header.tar.gz"))
			},
			successful: false,
		},
		"header-augment.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "header-augment.tar.gz"))
			},
			successful: false,
		},
		"data files broken checksum": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer file.Close()
				stat, _ := file.Stat()
				buf := make([]byte, stat.Size())
				file.Read(buf)
				lines := bytes.Split(buf, []byte("\n"))
				for _, line := range lines {
					if len(line) == 0 {
						continue
					}
					if bytes.HasPrefix(bytes.Split(line, []byte("  "))[1], []byte("data")) {
						// Flip one bit.
						line[0] ^= 0x1
						break
					}
				}
				file.Seek(0, 0)
				file.Write(buf)
			},
			successful: false,
			numFiles: 1,
		},
		"data/0000.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "data/0000.tar.gz"))
			},
			successful: false,
			numFiles: 1,
		},
		"data file missing from manifest": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer file.Close()
				stat, _ := file.Stat()
				buf := make([]byte, stat.Size())
				newbuf := make([]byte, 0, stat.Size())
				file.Read(buf)
				lines := bytes.Split(buf, []byte("\n"))
				for _, line := range lines {
					if len(line) != 0 && bytes.HasPrefix(bytes.Split(line, []byte("  "))[1], []byte("data")) {
						// Skip line.
						continue
					}
					for _, char := range line {
						newbuf = append(newbuf, char)
					}
					newbuf = append(newbuf, byte('\n'))
				}
				file.Seek(0, 0)
				file.Write(newbuf)
				file.Truncate(int64(len(newbuf)))
			},
			successful: false,
			errorStr: "can not find data file",
			numFiles: 1,
		},
		"Too many files in manifest": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_WRONLY | os.O_APPEND, 0)
				require.NoError(t, err)
				file.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  missing_file\n"))
				file.Close()
			},
			successful: false,
			numFiles: 1,
			errorStr: "not part of artifact",
		},
		"Too many files in empty manifest": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_WRONLY | os.O_APPEND, 0)
				require.NoError(t, err)
				file.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  missing_file\n"))
				file.Close()
			},
			successful: false,
			errorStr: "not part of artifact",
		},
		"augmented data files broken checksum": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer file.Close()
				stat, _ := file.Stat()
				buf := make([]byte, stat.Size())
				file.Read(buf)
				lines := bytes.Split(buf, []byte("\n"))
				for _, line := range lines {
					if len(line) == 0 {
						continue
					}
					if bytes.HasPrefix(bytes.Split(line, []byte("  "))[1], []byte("data")) {
						// Flip one bit.
						line[0] ^= 0x1
						break
					}
				}
				file.Seek(0, 0)
				file.Write(buf)
			},
			successful: false,
			numAugmentFiles: 1,
		},
		"augmented data file missing from manifest": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer file.Close()
				stat, _ := file.Stat()
				buf := make([]byte, stat.Size())
				newbuf := make([]byte, 0, stat.Size())
				file.Read(buf)
				lines := bytes.Split(buf, []byte("\n"))
				for _, line := range lines {
					if len(line) != 0 && bytes.HasPrefix(bytes.Split(line, []byte("  "))[1], []byte("data")) {
						// Skip line.
						continue
					}
					for _, char := range line {
						newbuf = append(newbuf, char)
					}
					newbuf = append(newbuf, byte('\n'))
				}
				file.Seek(0, 0)
				file.Write(newbuf)
				file.Truncate(int64(len(newbuf)))
			},
			successful: false,
			errorStr: "can not find data file",
			numAugmentFiles: 1,
		},
		"Too many files in manifest-augment": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_WRONLY | os.O_APPEND, 0)
				require.NoError(t, err)
				file.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  missing_file\n"))
				file.Close()
			},
			successful: false,
			numAugmentFiles: 1,
			errorStr: "not part of artifact",
		},
		"Too many files in empty manifest-augment": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_WRONLY | os.O_APPEND, 0)
				require.NoError(t, err)
				file.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  missing_file\n"))
				file.Close()
			},
			successful: false,
			errorStr: "not part of artifact",
		},
		"Files in both manifest files": {
			manipulateArtifact: func(tmpdir string) {
				// Do nothing.
			},
			successful: true,
			numFiles: 1,
			numAugmentFiles: 1,
		},
		"Conflicting checksums in manifest files": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer file.Close()

				// Copy the valid checksums to manifest-augment
				augm, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"),
					os.O_WRONLY | os.O_CREATE | os.O_TRUNC, 0644)
				require.NoError(t, err)
				defer augm.Close()
				_, err = io.Copy(augm, file)
				require.NoError(t, err)
				file.Seek(0, 0)

				// Now corrupt the one in the original manifest.
				stat, _ := file.Stat()
				buf := make([]byte, stat.Size())
				file.Read(buf)
				lines := bytes.Split(buf, []byte("\n"))
				for _, line := range lines {
					if len(line) == 0 {
						continue
					}
					if bytes.HasPrefix(bytes.Split(line, []byte("  "))[1], []byte("data")) {
						// Flip one bit.
						line[0] ^= 0x1
						break
					}
				}
				file.Seek(0, 0)
				file.Write(buf)
			},
			successful: false,
			errorStr: "file already exists",
			numFiles: 1,
		},
		"Several files": {
			manipulateArtifact: func(tmpdir string) {
				// Do nothing, but check that file number is correct.
				manifest, err := os.Open(filepath.Join(tmpdir, "manifest"))
				require.NoError(t, err)
				stat, _ := manifest.Stat()
				buf := make([]byte, stat.Size())
				_, err = manifest.Read(buf)
				require.NoError(t, err)
				// version + header.tar.gz + 2 files
				assert.Equal(t, 4, len(bytes.Split(bytes.TrimSpace(buf), []byte("\n"))))

				manifest, err = os.Open(filepath.Join(tmpdir, "manifest-augment"))
				require.NoError(t, err)
				stat, _ = manifest.Stat()
				buf = make([]byte, stat.Size())
				_, err = manifest.Read(buf)
				require.NoError(t, err)
				// header-augment.tar.gz + 4 files
				assert.Equal(t, 5, len(bytes.Split(bytes.TrimSpace(buf), []byte("\n"))))
			},
			successful: true,
			numFiles: 2,
			numAugmentFiles: 4,
		},
		"version file in wrong manifest": {
			manipulateArtifact: func(tmpdir string) {
				manifestFd, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer manifestFd.Close()
				augmFd, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_WRONLY | os.O_APPEND, 0)
				require.NoError(t, err)
				defer augmFd.Close()

				stat, _ := manifestFd.Stat()
				buf := make([]byte, stat.Size())
				_, err = manifestFd.Read(buf)
				require.NoError(t, err)

				manifestLines := bytes.Split(bytes.TrimSpace(buf), []byte("\n"))
				for n, line := range manifestLines {
					if string(bytes.SplitN(line, []byte("  "), 2)[1]) == "version" {
						_, err = augmFd.Write(line)
						require.NoError(t, err)

						copy(manifestLines[n:], manifestLines[n+1:])
						manifestLines = manifestLines[:len(manifestLines)-1]
						break
					}
				}
				manifestFd.Seek(0, 0)
				buf = bytes.Join(manifestLines, []byte("\n"))
				_, err = manifestFd.Write(buf)
				require.NoError(t, err)
				_, err = manifestFd.Write([]byte("\n"))
				require.NoError(t, err)
				manifestFd.Truncate(int64(len(buf) + 1))
				exec.Command("bash", "-c", fmt.Sprintf("( cat %s/manifest ; echo --- ; cat %s/manifest-augment ) > /home/kristian/Downloads/test.log", tmpdir, tmpdir)).Run()
			},
			successful: false,
			errorStr: "checksum missing for file: 'version'",
		},
	}

	for desc, c:= range cases {
		// Make basic artifact.
		var art io.Reader
		var err error
		if c.rootfsImage {
			art, err = MakeRootfsImageArtifact(3, false, false)
		} else {
			art, err = MakeModuleImageArtifact(false, false, "test-type", c.numFiles, c.numAugmentFiles)
		}
		require.NoError(t, err)


		// Perform manipulation of artifact
		tmpdir, err := ioutil.TempDir("", "mender-TestReadBrokenArtifact")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		cmd := exec.Command("tar", "x")
		cmd.Stdin = art
		cmd.Dir = tmpdir
		require.NoError(t, cmd.Run())

		c.manipulateArtifact(tmpdir)

		pipeR, pipeW := io.Pipe()
		cmd = exec.Command("tar", assembleSubset([]string{"c"}, tmpdir)...)
		cmd.Stdout = pipeW
		cmd.Dir = tmpdir
		require.NoError(t, cmd.Start())
		art = pipeR


		// Validate test case.
		r := NewReader(art)
		if c.inst != nil {
			r.RegisterHandler(c.inst)
		}
		err = r.ReadArtifact()
		if c.successful {
			assert.NoError(t, err, desc)
		} else {
			assert.Error(t, err, desc)
			if len(c.errorStr) > 0 {
				assert.Contains(t, err.Error(), c.errorStr)
			}
		}
	}
}
