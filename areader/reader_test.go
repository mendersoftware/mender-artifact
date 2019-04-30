// Copyright 2019 Northern.tech AS
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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func MakeRootfsImageArtifact(version int, signed, hasScripts, augmented bool) (io.Reader, error) {

	upd, err := MakeFakeUpdate(TestUpdateFileContent)
	if err != nil {
		return nil, err
	}
	defer os.Remove(upd)

	var composer, augment handlers.Composer
	switch version {
	case 1:
		composer = handlers.NewRootfsV1(upd)
	case 2:
		composer = handlers.NewRootfsV2(upd)
	case 3:
		if augmented {
			composer = handlers.NewRootfsV3("")
			augment = handlers.NewAugmentedRootfs(composer, upd)
		} else {
			composer = handlers.NewRootfsV3(upd)
		}
	default:
		return nil, fmt.Errorf("Unsupported artifact version: %d", version)
	}

	updates := awriter.Updates{
		Updates: []handlers.Composer{composer},
	}
	if augmented {
		updates.Augments = []handlers.Composer{augment}
	}

	return MakeAnyImageArtifact(version, signed, hasScripts, &updates)
}

func MakeModuleImageArtifact(signed, hasScripts bool, updateType string,
	numFiles, numAugmentFiles int) (io.Reader, error) {

	updates := awriter.Updates{}

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
	updates.Updates = []handlers.Composer{compose}

	if numAugmentFiles > 0 {
		compose = handlers.NewAugmentedModuleImage(compose, updateType)
		augmentFiles := make([]*handlers.DataFile, numAugmentFiles)
		for index := 0; index < numAugmentFiles; index++ {
			file, err := MakeFakeUpdate("test-file")
			if err != nil {
				return nil, err
			}
			augmentFiles[index] = &handlers.DataFile{Name: file}
		}
		compose.SetUpdateAugmentFiles(augmentFiles)
		updates.Augments = []handlers.Composer{compose}
	}

	return MakeAnyImageArtifact(3, signed, hasScripts, &updates)
}

func MakeAnyImageArtifact(version int, signed bool,
	hasScripts bool, updates *awriter.Updates) (io.Reader, error) {

	comp := artifact.NewCompressorGzip()

	art := bytes.NewBuffer(nil)
	var aw *awriter.Writer
	if !signed {
		aw = awriter.NewWriter(art, comp)
	} else {
		s := artifact.NewSigner([]byte(PrivateKey))
		aw = awriter.NewWriterSigned(art, comp, s)
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

	err := aw.WriteArtifact(&awriter.WriteArtifactArgs{
		Format:  "mender",
		Version: version,
		Devices: []string{"vexpress"},
		Name:    "mender-1.1",
		Updates: updates,
		Scripts: &scr,
		// Version 3 specifics:
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "mender-1.1",
			ArtifactGroup: "group-1",
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

	rfh := func() handlers.Installer {
		rfh := handlers.NewRootfsInstaller()
		rfh.SetUpdateStorerProducer(&testUpdateStorer{updFileContent})
		return rfh
	}

	tc := map[string]struct {
		version   int
		signed    bool
		handler   handlers.Installer
		verifier  artifact.Verifier
		readError error
	}{
		"version 1":        {1, false, rfh(), nil, nil},
		"version 2 pass":   {2, false, rfh(), nil, nil},
		"version 2 signed": {2, true, rfh(), artifact.NewVerifier([]byte(PublicKey)), nil},
		"version 2 - public key error": {2, true, rfh(), artifact.NewVerifier([]byte(PublicKeyError)),
			errors.New("reader: invalid signature: crypto/rsa: verification error")},
		// test that we do not need a verifier for signed artifact
		"version 2 - no verifier needed for a signed artifact": {2, true, rfh(), nil, nil},
		// Version 3 tests.
		"version 3 - base case": {3, false, rfh(), nil, nil},
		"version 3 - signed":    {3, true, rfh(), artifact.NewVerifier([]byte(PublicKey)), nil},
		"version 3 - public key error": {3, true, rfh(), artifact.NewVerifier([]byte(PublicKeyError)),
			errors.New("readHeaderV3: reader: invalid signature: crypto/rsa: verification error")},
	}

	// first create archive, that we will be able to read
	for name, test := range tc {
		t.Run(name, func(t *testing.T) {
			art, err := MakeRootfsImageArtifact(test.version, test.signed, false, false)
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
				return
			}
			require.NoError(t, err)
			assert.Equal(t, TestUpdateFileContent, updFileContent.String())

			devComp := aReader.GetCompatibleDevices()
			require.Len(t, devComp, 1)
			assert.Equal(t, "vexpress", devComp[0])

			if test.handler != nil {
				assert.Len(t, aReader.GetHandlers(), 1)
				assert.Equal(t, test.handler.GetUpdateType(), aReader.GetHandlers()[0].GetUpdateType())
			}
			assert.Equal(t, "mender-1.1", aReader.GetArtifactName())

			// clean the buffer
			updFileContent.Reset()
		})
	}
}

func TestReadSigned(t *testing.T) {
	art, err := MakeRootfsImageArtifact(2, true, false, false)
	assert.NoError(t, err)
	aReader := NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: verify signature callback not registered")

	art, err = MakeRootfsImageArtifact(2, false, false, false)
	assert.NoError(t, err)
	aReader = NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: expecting signed artifact, but no signature file found")

	art, err = MakeRootfsImageArtifact(2, true, false, false)
	assert.NoError(t, err)
	aReader = NewReader(art)
	err = aReader.ReadArtifact()
	assert.NoError(t, err)

	art, err = MakeRootfsImageArtifact(1, false, false, false)
	assert.NoError(t, err)
	aReader = NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: expecting signed artifact")

	art, err = MakeRootfsImageArtifact(3, true, false, false)
	assert.NoError(t, err)
	aReader = NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: verify signature callback not registered")

	art, err = MakeRootfsImageArtifact(3, false, false, false)
	assert.NoError(t, err)
	aReader = NewReaderSigned(art)
	err = aReader.ReadArtifact()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),
		"reader: expecting signed artifact, but no signature file found")

	art, err = MakeRootfsImageArtifact(3, true, false, false)
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
	art, err := MakeRootfsImageArtifact(1, false, false, false)
	assert.NoError(t, err)

	aReader := NewReader(art)
	err = aReader.ReadArtifact()
	assert.NoError(t, err)

	assert.Len(t, aReader.GetHandlers(), 1)
	assert.Equal(t, "rootfs-image", aReader.GetHandlers()[0].GetUpdateType())
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
	art, err := MakeRootfsImageArtifact(2, false, true, false)
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
	t          *testing.T
	Data       *handlers.DataFile
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

func (i *installer) GetUpdateDepends() (*artifact.TypeInfoDepends, error) {
	return &artifact.TypeInfoDepends{}, nil
}

func (i *installer) GetUpdateProvides() (*artifact.TypeInfoProvides, error) {
	return &artifact.TypeInfoProvides{}, nil
}

func (i *installer) GetUpdateMetaData() (map[string]interface{}, error) {
	return nil, nil
}

func (i *installer) GetUpdateOriginalDepends() *artifact.TypeInfoDepends {
	return &artifact.TypeInfoDepends{}
}

func (i *installer) GetUpdateOriginalProvides() *artifact.TypeInfoProvides {
	return &artifact.TypeInfoProvides{}
}

func (i *installer) GetUpdateOriginalMetaData() map[string]interface{} {
	return nil
}

func (i *installer) GetUpdateAugmentDepends() *artifact.TypeInfoDepends {
	return &artifact.TypeInfoDepends{}
}

func (i *installer) GetUpdateAugmentProvides() *artifact.TypeInfoProvides {
	return &artifact.TypeInfoProvides{}
}

func (i *installer) GetUpdateAugmentMetaData() map[string]interface{} {
	return nil
}

func (i *installer) GetVersion() int {
	return 3
}

func (i *installer) GetUpdateType() string {
	return i.updateType
}

func (i *installer) GetUpdateOriginalType() string {
	return ""
}

func (i *installer) NewInstance() handlers.Installer {
	return i
}

func (i *installer) NewAugmentedInstance(orig handlers.ArtifactUpdate) (handlers.Installer, error) {
	return nil, nil
}

func (i *installer) ReadHeader(r io.Reader, path string, version int, augmented bool) error {
	return nil
}

func (i *installer) NewUpdateStorer(updateType string, payloadNum int) (handlers.UpdateStorer, error) {
	return &testUpdateStorer{}, nil
}

func (i *installer) SetUpdateStorerProducer(producer handlers.UpdateStorerProducer) {
	// Not used currently.
}

func (i *installer) GetUpdateOriginalTypeInfoWriter() io.Writer {
	return nil
}

func (i *installer) GetUpdateAugmentTypeInfoWriter() io.Writer {
	return nil
}

type testUpdateStorer struct {
	w io.Writer
}

func (s *testUpdateStorer) Initialize(artifactHeaders,
	artifactAugmentedHeaders artifact.HeaderInfoer,
	payloadHeaders handlers.ArtifactUpdateHeaders) error {

	return nil
}

func (s *testUpdateStorer) PrepareStoreUpdate() error {
	return nil
}

func (s *testUpdateStorer) StoreUpdate(r io.Reader, info os.FileInfo) error {
	var w io.Writer
	if s.w != nil {
		w = s.w
	} else {
		w = ioutil.Discard
	}
	_, err := io.Copy(w, r)
	return err
}

func (s *testUpdateStorer) FinishStoreUpdate() error {
	return nil
}

func (s *testUpdateStorer) NewUpdateStorer(updateType string, payloadNum int) (handlers.UpdateStorer, error) {
	return s, nil
}

func writeDataFile(t *testing.T, name, data string) io.Reader {
	comp := artifact.NewCompressorGzip()

	buf := bytes.NewBuffer(nil)
	gz, err := comp.NewWriter(buf)
	assert.NoError(t, err)
	tw := tar.NewWriter(gz)
	sw := artifact.NewTarWriterStream(tw)
	err = sw.Write([]byte(data), name)
	assert.NoError(t, err)
	err = tw.Close()
	assert.NoError(t, err)
	err = gz.Close()
	assert.NoError(t, err)

	return buf
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
	art, err := MakeRootfsImageArtifact(3, true, false, false)
	require.NoError(t, err)
	ar := NewReader(art)
	err = ar.ReadArtifact()
	require.NoError(t, err)

	assert.Equal(t, ar.GetInfo(), artifact.Info{Format: "mender", Version: 3})
	assert.Equal(t, *ar.GetArtifactProvides(), artifact.ArtifactProvides{ArtifactName: "mender-1.1",
		ArtifactGroup: "group-1"})
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
		successful         bool
		errorStr           string
		rootfsImage        bool
		numFiles           int
		numAugmentFiles    int
	}
	cases := map[string]testCase{
		"rootfs-image, Everything ok": {
			manipulateArtifact: func(tmpdir string) {
				// Do nothing. Just checking that the tar cmd works.
			},
			successful:  true,
			rootfsImage: true,
			numFiles:    1,
		},
		"rootfs-image, version missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "version"))
			},
			successful:  false,
			rootfsImage: true,
		},
		"rootfs-image, header.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "header.tar.gz"))
			},
			successful:  false,
			rootfsImage: true,
		},
		"rootfs-image, header-augment.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "header-augment.tar.gz"))
			},
			successful:      false,
			rootfsImage:     true,
			numAugmentFiles: 1,
		},
		"Everything ok": {
			manipulateArtifact: func(tmpdir string) {
				// Do nothing. Just checking that the tar cmd works.
			},
			successful: true,
		},
		"Everything ok, augmented files present": {
			manipulateArtifact: func(tmpdir string) {
				// Do nothing. Just checking that the tar cmd works.
			},
			successful:      true,
			numAugmentFiles: 1,
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
		"no manifest-augment": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "manifest-augment"))
			},
			successful:      false,
			errorStr:        "invalid data file",
			numAugmentFiles: 1,
		},
		"augmented files, but header-augment.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "header-augment.tar.gz"))

				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer file.Close()
				stat, _ := file.Stat()
				buf := make([]byte, stat.Size())
				newbuf := make([]byte, 0, stat.Size())
				file.Read(buf)
				lines := bytes.Split(buf, []byte("\n"))
				for _, line := range lines {
					if len(line) != 0 && bytes.HasPrefix(bytes.Split(line, []byte("  "))[1], []byte("header-augment")) {
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
			successful:      false,
			numAugmentFiles: 1,
			errorStr:        "Invalid structure",
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
			numFiles:   1,
		},
		"data/0000.tar.gz missing": {
			manipulateArtifact: func(tmpdir string) {
				os.Remove(filepath.Join(tmpdir, "data/0000.tar.gz"))
			},
			successful: false,
			numFiles:   1,
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
			errorStr:   "can not find data file",
			numFiles:   1,
		},
		"Too many files in manifest": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_WRONLY|os.O_APPEND, 0)
				require.NoError(t, err)
				file.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  missing_file\n"))
				file.Close()
			},
			successful: false,
			numFiles:   1,
			errorStr:   "not part of artifact",
		},
		"Too many files in empty manifest": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_WRONLY|os.O_APPEND, 0)
				require.NoError(t, err)
				file.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  missing_file\n"))
				file.Close()
			},
			successful: false,
			errorStr:   "not part of artifact",
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
			successful:      false,
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
			successful:      false,
			errorStr:        "can not find data file",
			numAugmentFiles: 1,
		},
		"Too many files in manifest-augment": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_WRONLY|os.O_APPEND, 0)
				require.NoError(t, err)
				file.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  missing_file\n"))
				file.Close()
			},
			successful:      false,
			numAugmentFiles: 1,
			errorStr:        "not part of artifact",
		},
		"Too many files in empty manifest-augment": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
				require.NoError(t, err)
				file.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  missing_file\n"))
				file.Close()
			},
			successful: false,
			errorStr:   "Invalid structure",
		},
		"Files in both manifest files": {
			manipulateArtifact: func(tmpdir string) {
				// Do nothing.
			},
			successful:      true,
			numFiles:        1,
			numAugmentFiles: 1,
		},
		"Conflicting checksums in manifest files": {
			manipulateArtifact: func(tmpdir string) {
				file, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer file.Close()

				// Copy the valid checksums to manifest-augment
				augm, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"),
					os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
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
			errorStr:   "file already exists",
			numFiles:   1,
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
			successful:      true,
			numFiles:        2,
			numAugmentFiles: 4,
		},
		"version file in wrong manifest": {
			manipulateArtifact: func(tmpdir string) {
				manifestFd, err := os.OpenFile(filepath.Join(tmpdir, "manifest"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer manifestFd.Close()
				augmFd, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_WRONLY|os.O_APPEND, 0)
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
			},
			successful:      false,
			errorStr:        "checksum missing for file: 'version'",
			numFiles:        1,
			numAugmentFiles: 1,
		},
		"rootfs-image, invalid character in update data file": {
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
					if len(line) == 0 {
						continue
					}

					fileName := bytes.Split(line, []byte("  "))[1]

					if bytes.HasPrefix(fileName, []byte("data")) {
						fileNameStr := string(fileName[:])
						updateDataDirPath := filepath.Dir(fileNameStr)
						updateDataDirPathFull := filepath.Join(tmpdir, updateDataDirPath)
						dataArchive := updateDataDirPathFull + ".tar.gz"
						require.NoError(t, os.Mkdir(updateDataDirPathFull, 0755))

						// uncompress update data file
						cmdArgs := []string{"-C", updateDataDirPathFull}
						cmdArgs = append(cmdArgs, []string{"-zxf", dataArchive}...)
						cmd := exec.Command("tar", cmdArgs...)
						require.NoError(t, cmd.Run())

						updateDataFilePath := filepath.Join(tmpdir, fileNameStr)
						_, err := os.Stat(updateDataFilePath)
						require.False(t, os.IsNotExist(err))

						// rename update data file by replacing last character with an asterisk
						lastChar := updateDataFilePath[len(updateDataFilePath)-1:]
						updateDataFilePathNew := strings.TrimSuffix(updateDataFilePath, lastChar) + "*"
						err = os.Rename(updateDataFilePath, updateDataFilePathNew)
						assert.NoError(t, err)

						// compress back update data file
						cmdArgs = []string{"-czf", dataArchive}
						cmdArgs = append(cmdArgs,
							[]string{"-C", updateDataDirPathFull, filepath.Base(updateDataFilePathNew)}...)
						cmd = exec.Command("tar", cmdArgs...)
						require.NoError(t, cmd.Run())

						// remove path to uncompressed update data file
						require.NoError(t, os.RemoveAll(updateDataDirPathFull))
						// replace last character in line with an asterisk
						line[len(line)-1] = 0x2A
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
			successful:  false,
			errorStr:    "Only letters, digits and characters in the set \".,_-\" are allowed",
			rootfsImage: true,
		},
		"Scripts in augmented header": {
			manipulateArtifact: func(tmpdir string) {
				headertmp := filepath.Join(tmpdir, "headertmp")
				require.NoError(t, os.Mkdir(headertmp, 0755))
				cmd := exec.Command("tar", "xzf", "../header-augment.tar.gz")
				cmd.Dir = headertmp
				require.NoError(t, cmd.Run())

				require.NoError(t, os.Mkdir(filepath.Join(headertmp, "scripts"), 0755))
				fd, err := os.OpenFile(filepath.Join(headertmp, "scripts", "ArtifactInstall_Enter_00"),
					os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
				require.NoError(t, err)
				_, err = fd.Write([]byte("#!/bin/sh\n"))
				require.NoError(t, err)
				fd.Close()

				cmd = exec.Command("tar", "czf", "../header-augment.tar.gz", "header-info",
					"scripts", "headers")
				cmd.Dir = headertmp
				require.NoError(t, cmd.Run())

				fd, err = os.Open(filepath.Join(tmpdir, "header-augment.tar.gz"))
				require.NoError(t, err)
				buf, err := ioutil.ReadAll(fd)
				require.NoError(t, err)
				checksumBytes := sha256.Sum256(buf)
				checksum := hex.EncodeToString(checksumBytes[:])

				augmFd, err := os.OpenFile(filepath.Join(tmpdir, "manifest-augment"), os.O_RDWR, 0)
				require.NoError(t, err)
				defer augmFd.Close()

				buf, err = ioutil.ReadAll(augmFd)
				require.NoError(t, err)

				manifestLines := bytes.Split(bytes.TrimSpace(buf), []byte("\n"))
				for n, line := range manifestLines {
					if string(bytes.SplitN(line, []byte("  "), 2)[1]) == "header-augment.tar.gz" {
						manifestLines[n] = []byte(fmt.Sprintf("%s  header-augment.tar.gz", checksum))
					}
				}

				augmFd.Seek(0, 0)
				buf = bytes.Join(manifestLines, []byte("\n"))
				_, err = augmFd.Write(buf)
				require.NoError(t, err)
				_, err = augmFd.Write([]byte("\n"))
				require.NoError(t, err)

				require.NoError(t, os.RemoveAll(headertmp))
			},
			successful: false,
			// A somewhat strange error message to look for, but
			// it's because the script directory is not expected at
			// all, so the code goes straight to trying to figure
			// out the payload index number.
			errorStr:        "can not get Payload order from tar path",
			numFiles:        1,
			numAugmentFiles: 1,
		},
		"Two rootfs images": {
			manipulateArtifact: func(tmpdir string) {
				datatmp := filepath.Join(tmpdir, "data", "datatmp")
				require.NoError(t, os.Mkdir(datatmp, 0755))
				cmd := exec.Command("tar", "xzf", "../0000.tar.gz")
				cmd.Dir = datatmp
				require.NoError(t, cmd.Run())

				fd, err := os.OpenFile(filepath.Join(datatmp, "test-datafile"),
					os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
				require.NoError(t, err)
				content := []byte("dummy")
				fd.Write(content)
				fd.Close()

				statlist, err := ioutil.ReadDir(datatmp)
				require.NoError(t, err)
				arglist := make([]string, 0, len(statlist)+2)
				arglist = append(arglist, "czf")
				arglist = append(arglist, "../0000.tar.gz")
				for _, s := range statlist {
					arglist = append(arglist, s.Name())
				}

				cmd = exec.Command("tar", arglist...)
				cmd.Dir = datatmp
				require.NoError(t, cmd.Run())

				fd, err = os.OpenFile(filepath.Join(tmpdir, "manifest"),
					os.O_WRONLY|os.O_APPEND, 0)
				require.NoError(t, err)
				checksumBytes := sha256.Sum256(content)
				checksum := hex.EncodeToString(checksumBytes[:])
				fd.Write([]byte(fmt.Sprintf("%s  data/0000/test-datafile\n", checksum)))
				fd.Close()
			},
			successful:  false,
			errorStr:    "Must provide exactly one update file",
			rootfsImage: true,
			numFiles:    1,
		},
		"Non-JSON meta-data": {
			manipulateArtifact: func(tmpdir string) {
				headertmp := filepath.Join(tmpdir, "headertmp")
				require.NoError(t, os.Mkdir(headertmp, 0755))
				cmd := exec.Command("tar", "xzf", "../header.tar.gz")
				cmd.Dir = headertmp
				require.NoError(t, cmd.Run())

				fd, err := os.OpenFile(filepath.Join(headertmp, "headers/0000/meta-data"),
					os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
				require.NoError(t, err)
				fd.Write([]byte("Not JSON"))
				fd.Close()

				cmd = exec.Command("tar", "czf", "../header.tar.gz", "header-info", "headers")
				cmd.Dir = headertmp
				require.NoError(t, cmd.Run())

				fd, err = os.Open(filepath.Join(tmpdir, "header.tar.gz"))
				require.NoError(t, err)
				content, err := ioutil.ReadAll(fd)
				require.NoError(t, err)
				fd.Close()
				checksumBytes := sha256.Sum256(content)
				checksum := hex.EncodeToString(checksumBytes[:])

				fd, err = os.OpenFile(filepath.Join(tmpdir, "manifest"),
					os.O_RDWR, 0)
				require.NoError(t, err)
				manifestLines, err := ioutil.ReadAll(fd)
				require.NoError(t, err)
				fd.Seek(0, 0)
				fd.Truncate(0)
				for _, line := range bytes.Split(manifestLines, []byte("\n")) {
					if strings.Contains(string(line), "header.tar.gz") {
						copy(line[0:len(checksum)], checksum)
					}
					fd.Write(line)
					fd.Write([]byte("\n"))
				}
				fd.Close()
			},
			successful: false,
			errorStr:   "error reading meta-data: invalid character",
		},
		"Non-object meta-data": {
			manipulateArtifact: func(tmpdir string) {
				headertmp := filepath.Join(tmpdir, "headertmp")
				require.NoError(t, os.Mkdir(headertmp, 0755))
				cmd := exec.Command("tar", "xzf", "../header.tar.gz")
				cmd.Dir = headertmp
				require.NoError(t, cmd.Run())

				fd, err := os.OpenFile(filepath.Join(headertmp, "headers/0000/meta-data"),
					os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
				require.NoError(t, err)
				fd.Write([]byte("[\"Json\",\"list\"]"))
				fd.Close()

				cmd = exec.Command("tar", "czf", "../header.tar.gz", "header-info", "headers")
				cmd.Dir = headertmp
				require.NoError(t, cmd.Run())

				fd, err = os.Open(filepath.Join(tmpdir, "header.tar.gz"))
				require.NoError(t, err)
				content, err := ioutil.ReadAll(fd)
				require.NoError(t, err)
				fd.Close()
				checksumBytes := sha256.Sum256(content)
				checksum := hex.EncodeToString(checksumBytes[:])

				fd, err = os.OpenFile(filepath.Join(tmpdir, "manifest"),
					os.O_RDWR, 0)
				require.NoError(t, err)
				manifestLines, err := ioutil.ReadAll(fd)
				require.NoError(t, err)
				fd.Seek(0, 0)
				fd.Truncate(0)
				for _, line := range bytes.Split(manifestLines, []byte("\n")) {
					if strings.Contains(string(line), "header.tar.gz") {
						copy(line[0:len(checksum)], checksum)
					}
					fd.Write(line)
					fd.Write([]byte("\n"))
				}
				fd.Close()
			},
			successful: false,
			errorStr:   "Top level object in meta-data must be a JSON object",
		},
	}

	for desc, c := range cases {
		t.Run(desc, func(t *testing.T) {
			// Make basic artifact.
			var art io.Reader
			var err error
			augmented := (c.numAugmentFiles > 0)
			if c.rootfsImage {
				art, err = MakeRootfsImageArtifact(3, false, false, augmented)
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
			err = r.ReadArtifact()
			if c.successful {
				assert.NoError(t, err, desc)
			} else {
				require.Error(t, err, desc)
				if len(c.errorStr) > 0 {
					assert.Contains(t, err.Error(), c.errorStr)
				}
				return
			}

			assert.Equal(t, 1, len(r.GetHandlers()))
			handler := r.GetHandlers()[0]
			assert.Equal(t, c.numFiles, len(handler.GetUpdateFiles()))
			assert.Equal(t, c.numAugmentFiles, len(handler.GetUpdateAugmentFiles()))
		})
	}
}
