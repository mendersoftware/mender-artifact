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

package awriter

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkTarElements(r io.Reader, expected int) error {
	tr := tar.NewReader(r)
	i := 0
	for ; ; i++ {
		_, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	if i != expected {
		return errors.Errorf("invalid number of elements; expecting %d, actual %d",
			expected, i)
	}
	return nil
}

func checkTarElementsByName(r io.Reader, expected []string) error {
	tr := tar.NewReader(r)
	actual := []string{}
	for hdr, err := tr.Next(); err != io.EOF && hdr != nil; hdr, err = tr.Next() {
		actual = append(actual, filepath.Base(hdr.Name))
	}
	if len(expected) != len(actual) {
		return fmt.Errorf("The expected list: %s and the actual list: %s are not the same length", expected, actual)
	}
	for i, _ := range expected {
		if strings.Trim(expected[i], " ") != strings.Trim(actual[i], " ") {
			return fmt.Errorf("%s and %s mismatch", expected[i], actual[i])
		}
	}
	return nil
}

func TestWriteArtifactWrongVersion(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf, artifact.NewCompressorGzip())

	// Version 1 no longer allowed
	err := w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 1,
		Devices: []string{"asd"},
		Name:    "name",
	})
	assert.EqualError(t, err, "writer: The Mender-Artifact version 1 is outdated. Refusing to create artifact.")

	// Version 0 not allowed
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 0,
		Devices: []string{"asd"},
		Name:    "name",
	})
	assert.EqualError(t, err, "Unsupported artifact version")

	// Version 4 not allowed
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 4,
		Devices: []string{"asd"},
		Name:    "name",
	})
	assert.EqualError(t, err, "Unsupported artifact version")
}

func TestWriteArtifactWithUpdates(t *testing.T) {
	comp := artifact.NewCompressorGzip()

	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf, comp)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV2(upd)
	updates := &Updates{Updates: []handlers.Composer{u}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 2,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
	})
	assert.NoError(t, err)

	assert.NoError(t, checkTarElements(buf, 4))

	// Update with invalid data file name.
	upd, err = MakeFakeInvalidUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u = handlers.NewRootfsV2(upd)
	updates = &Updates{Updates: []handlers.Composer{u}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 2,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
	})

	assert.Error(t, err)
}

func TestWriteMultipleUpdates(t *testing.T) {
	comp := artifact.NewCompressorGzip()

	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf, comp)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u1 := handlers.NewRootfsV2(upd)
	u2 := handlers.NewRootfsV2(upd)
	updates := &Updates{Updates: []handlers.Composer{u1, u2}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 2,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
	})
	assert.NoError(t, err)

	assert.NoError(t, checkTarElements(buf, 5))

	// Update with invalid data file name.
	upd, err = MakeFakeInvalidUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u1 = handlers.NewRootfsV2(upd)
	u2 = handlers.NewRootfsV2(upd)
	updates = &Updates{Updates: []handlers.Composer{u1, u2}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 2,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
	})

	assert.Error(t, err)
}

const PrivateKey = `-----BEGIN RSA PRIVATE KEY-----
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

func TestWriteArtifactV2(t *testing.T) {
	comp := artifact.NewCompressorGzip()

	buf := bytes.NewBuffer(nil)

	s := artifact.NewSigner([]byte(PrivateKey))
	w := NewWriterSigned(buf, comp, s)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV2(upd)
	updates := &Updates{Updates: []handlers.Composer{u}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 2,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
	})
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 5))
	buf.Reset()

	w = NewWriterSigned(buf, comp, nil)
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 2,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
	})
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 4))
	buf.Reset()

	// Update with invalid data file name.
	upd, err = MakeFakeInvalidUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u = handlers.NewRootfsV2(upd)
	updates = &Updates{Updates: []handlers.Composer{u}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 2,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
	})

	assert.Error(t, err)
}

func TestWriteArtifactV3(t *testing.T) {
	comp := artifact.NewCompressorGzip()

	buf := bytes.NewBuffer(nil)

	s := artifact.NewSigner([]byte(PrivateKey))
	w := NewWriterSigned(buf, comp, s)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV3(upd)
	updates := &Updates{Updates: []handlers.Composer{u}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates,
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	require.NoError(t, err)
	assert.NoError(t, checkTarElementsByName(buf, []string{
		"version",
		"manifest",
		"manifest.sig",
		"header.tar.gz",
		"0000.tar.gz",
	}))
	buf.Reset()

	// error writing non-existing
	u = handlers.NewRootfsV3("non-existing")
	updates = &Updates{Updates: []handlers.Composer{u}}
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates,
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.Error(t, err)
	buf.Reset()

	// Unsigned artifact V3
	buf.Reset()
	w = NewWriter(buf, comp)
	upd, err = MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u = handlers.NewRootfsV3(upd)
	updates = &Updates{Updates: []handlers.Composer{u}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates,
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.NoError(t, err)
	assert.NoError(t, checkTarElementsByName(buf, []string{
		"version",
		"manifest",
		"header.tar.gz",
		"0000.tar.gz",
	}))
	buf.Reset()

	// Signed artifact V3
	buf.Reset()
	s = artifact.NewSigner([]byte(PrivateKey))
	w = NewWriterSigned(buf, comp, s)
	upd, err = MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u = handlers.NewRootfsV3(upd)
	updates = &Updates{Updates: []handlers.Composer{u}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates,
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.NoError(t, err)
	assert.NoError(t, checkTarElementsByName(buf, []string{
		"version",
		"manifest",
		"manifest.sig",
		"header.tar.gz",
		"0000.tar.gz",
	}))
	buf.Reset()

	// error writing non-existing
	u = handlers.NewRootfsV3("non-existing")
	updates = &Updates{Updates: []handlers.Composer{u}}
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates,
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.Error(t, err)
	buf.Reset()

	// Test errors returned on write-errors.
	// NOTE: The failOnWriteNr could change if the underlying
	// writing of the artifact changes, and thus has to be manually
	// changed to fail on the correct write later. Hopefully this should not happen
	// though, as the artifact format stays constant once implemented.

	// Fail writing artifact version
	failBuf := &TestErrWriter{FailOnWriteData: []byte("version")}
	w = NewWriterSigned(failBuf, comp, s)
	u = handlers.NewRootfsV3(upd)
	updates = &Updates{Updates: []handlers.Composer{u}} // Update existing.
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates,
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.Contains(t, err.Error(), "writer: can not write version tar header")

	// Fail writing artifact manifest.
	failBuf = &TestErrWriter{FailOnWriteData: []byte("manifest")}
	w = NewWriterSigned(failBuf, comp, s)
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates, // Update existing.
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.Contains(t, err.Error(), "WriteArtifact: writer: can not write manifest stream")

	// Fail writing artifact header.
	failBuf = &TestErrWriter{FailOnWriteData: []byte("header.tar.gz")}
	w = NewWriterSigned(failBuf, comp, s)
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates, // Update existing.
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.Contains(t, err.Error(), "writer: can not tar header")

	u = handlers.NewRootfsV3("")
	a := handlers.NewRootfsV3(upd)
	updates = &Updates{Updates: []handlers.Composer{u}, Augments: []handlers.Composer{a}}

	// Fail writing artifact header-augment.
	failBuf = &TestErrWriter{FailOnWriteData: []byte("header-augment.tar.gz")}
	w = NewWriterSigned(failBuf, comp, s)
	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates, // Update existing.
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.Contains(t, err.Error(), "writer: can not tar augmented-header")

	// Unsigned artifact V3 with augments section.
	buf.Reset()
	w = NewWriter(buf, comp)
	upd, err = MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u = handlers.NewRootfsV3("")
	a = handlers.NewRootfsV3(upd)
	updates = &Updates{Updates: []handlers.Composer{u}, Augments: []handlers.Composer{a}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates,
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.NoError(t, err)
	assert.NoError(t, checkTarElementsByName(buf, []string{
		"version",
		"manifest",
		"manifest-augment",
		"header.tar.gz",
		"header-augment.tar.gz",
		"0000.tar.gz",
	}))
	buf.Reset()

	// Signed artifact V3 with augments section.
	buf.Reset()
	s = artifact.NewSigner([]byte(PrivateKey))
	w = NewWriterSigned(buf, comp, s)
	upd, err = MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u = handlers.NewRootfsV3("")
	a = handlers.NewRootfsV3(upd)
	updates = &Updates{Updates: []handlers.Composer{u}, Augments: []handlers.Composer{a}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"vexpress-qemu"},
		Name:    "name",
		Updates: updates,
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  "name",
			ArtifactGroup: "group-1",
		},
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      []string{"depends-name"},
			CompatibleDevices: []string{"vexpress-qemu"},
		},
	})
	assert.NoError(t, err)
	assert.NoError(t, checkTarElementsByName(buf, []string{
		"version",
		"manifest",
		"manifest.sig",
		"manifest-augment",
		"header.tar.gz",
		"header-augment.tar.gz",
		"0000.tar.gz",
	}))
	buf.Reset()

	// Update with invalid data file name.
	upd, err = MakeFakeInvalidUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u = handlers.NewRootfsV3(upd)
	updates = &Updates{Updates: []handlers.Composer{u}}

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 3,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
	})

	assert.Error(t, err)
}

func TestWithScripts(t *testing.T) {
	comp := artifact.NewCompressorGzip()

	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf, comp)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV2(upd)
	updates := &Updates{Updates: []handlers.Composer{u}}

	scr, err := ioutil.TempFile("", "ArtifactInstall_Enter_10_")
	assert.NoError(t, err)
	defer os.Remove(scr.Name())

	s := new(artifact.Scripts)
	err = s.Add(scr.Name())
	assert.NoError(t, err)

	err = w.WriteArtifact(&WriteArtifactArgs{
		Format:  "mender",
		Version: 2,
		Devices: []string{"asd"},
		Name:    "name",
		Updates: updates,
		Scripts: s,
	})
	assert.NoError(t, err)

	assert.NoError(t, checkTarElements(buf, 4))
}

// TestErrWriter is a utility for simulating failed writes during tests.
type TestErrWriter struct {
	FailOnWriteData []byte
}

func (t *TestErrWriter) Write(b []byte) (n int, err error) {
	if bytes.HasPrefix(b, t.FailOnWriteData) {
		return 0, io.ErrUnexpectedEOF
	}
	return len(b), nil
}

func TestWriteManifestVersion(t *testing.T) {
	augmentedChecksumStore := artifact.NewChecksumStore()
	// Add one file to force creation of the augment section.
	augmentedChecksumStore.Add("dummy-file", []byte("dummy-checksum"))

	testcases := map[string]struct {
		version  int
		signer   artifact.Signer
		tw       *tar.Writer
		mchk     *artifact.ChecksumStore
		augmchk  *artifact.ChecksumStore
		aistream []byte
		err      string
	}{
		"wrong version": {version: -1,
			err: "writer: unsupported artifact version: -1"},
		"version 2, fail on write to manifest checksum store": {
			version: 2,
			mchk:    artifact.NewChecksumStore(),
			tw:      tar.NewWriter(&TestErrWriter{FailOnWriteData: []byte("manifest")}),
			err:     "writer: can not write manifest stream",
		},
		"version 2, fail on signature write": {
			version: 2,
			mchk:    artifact.NewChecksumStore(),
			tw:      tar.NewWriter(&TestErrWriter{FailOnWriteData: []byte("manifest.sig")}),
			signer:  artifact.NewSigner([]byte(PrivateKey)),
			err:     "writer: can not tar signature",
		},
		"version 3, fail on write to manifest checksum store": {
			version: 3,
			mchk:    artifact.NewChecksumStore(),
			tw:      tar.NewWriter(&TestErrWriter{FailOnWriteData: []byte("manifest")}),
			err:     "writer: can not write manifest stream",
		},
		"version 3, fail on signature write": {
			version: 3,
			mchk:    artifact.NewChecksumStore(),
			tw:      tar.NewWriter(&TestErrWriter{FailOnWriteData: []byte("manifest.sig")}),
			signer:  artifact.NewSigner([]byte(PrivateKey)),
			err:     "writer: can not tar signature",
		},
		"version 3, fail write augmented-manifest": {
			version: 3,
			mchk:    artifact.NewChecksumStore(),
			augmchk: augmentedChecksumStore,
			tw:      tar.NewWriter(&TestErrWriter{FailOnWriteData: []byte("manifest-augment")}),
			signer:  artifact.NewSigner([]byte(PrivateKey)),
			err:     "writer: can not write manifest stream",
		},
	}

	for desc, test := range testcases {
		t.Run(desc, func(t *testing.T) {
			err := writeManifestVersion(test.version, test.signer, test.tw, test.mchk, test.augmchk, test.aistream)
			if test.err != "" {
				assert.Contains(t, err.Error(), test.err)
			}
		})
	}
}

type TestDirEntry struct {
	Path    string
	Content []byte
	IsDir   bool
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

func MakeFakeInvalidUpdate(data string) (string, error) {
	// random string replaces the last "*", hence double "*" are needed
	f, err := ioutil.TempFile("", "test_update_**")
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

func MakeFakeUpdateDir(updateDir string, elements []TestDirEntry) error {
	for _, elem := range elements {
		if elem.IsDir {
			if err := os.MkdirAll(path.Join(updateDir, elem.Path), os.ModeDir|os.ModePerm); err != nil {
				return err
			}
		} else {
			f, err := os.Create(path.Join(updateDir, elem.Path))
			if err != nil {
				return err
			}
			defer f.Close()
			if len(elem.Content) > 0 {
				if _, err = f.Write(elem.Content); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func TestRootfsCompose(t *testing.T) {
	comp := artifact.NewCompressorGzip()

	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	f, err := ioutil.TempFile("", "update")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	var r handlers.Composer
	r = handlers.NewRootfsV2(f.Name())
	err = r.ComposeHeader(&handlers.ComposeHeaderArgs{
		TarWriter: tw,
		No:        1,
	})
	require.NoError(t, err)

	err = writeData(tw, comp, &Updates{[]handlers.Composer{r}, nil}, nil)
	require.NoError(t, err)

	// error compose data with missing data file
	r = handlers.NewRootfsV2("non-existing")
	err = writeData(tw, comp, &Updates{[]handlers.Composer{r}, nil}, nil)
	require.Error(t, err)
	require.Contains(t, errors.Cause(err).Error(),
		"no such file or directory")

	// Artifact format version 3
	r = handlers.NewRootfsV3(f.Name())
	err = r.ComposeHeader(&handlers.ComposeHeaderArgs{
		TarWriter: tw,
		No:        3,
	})
	require.NoError(t, err, "failed to compose the rootfs header - version 3")

	// Artifact format version 3, augmented
	r = handlers.NewRootfsV3(f.Name())
	err = r.ComposeHeader(&handlers.ComposeHeaderArgs{
		TarWriter: tw,
		Augmented: true,
		No:        3,
	})
	require.NoError(t, err, "failed to compose the rootfs header - version 3")

	// Artifact format version 3, augmented - fail write.
	r = handlers.NewRootfsV3(f.Name())
	tw = tar.NewWriter(new(TestErrWriter))
	err = r.ComposeHeader(&handlers.ComposeHeaderArgs{
		TarWriter: tw,
		Augmented: true,
		No:        3,
	})
	require.Contains(t, err.Error(), "can not tar type-info")

}
