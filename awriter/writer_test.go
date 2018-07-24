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

package awriter

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func checkTarElements(r io.Reader, expected int) error {
	tr := tar.NewReader(r)
	i := 0
	for ; ; i++ {
		tarHeader, err := tr.Next()
		if tarHeader != nil {
			fmt.Println(tarHeader.Name)
		} else {
			fmt.Println("tar Header is nil")
		}
		// fmt.Println(tarHeader.Name)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	if i != expected {
		return errors.Errorf("invalid number of elements; expecting %d, atual %d",
			expected, i)
	}
	return nil
}

func TestWriteArtifact(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf)

	err := w.WriteArtifact("mender", 1, []string{"asd"}, "name", &Updates{}, nil)
	assert.NoError(t, err)

	assert.NoError(t, checkTarElements(buf, 2))
}

func TestWriteArtifactWithUpdates(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV1(upd)
	updates := &Updates{U: []handlers.Composer{u}}

	err = w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates, nil)
	assert.NoError(t, err)

	assert.NoError(t, checkTarElements(buf, 3))
}

func TestWriteMultipleUpdates(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u1 := handlers.NewRootfsV1(upd)
	u2 := handlers.NewRootfsV1(upd)
	updates := &Updates{U: []handlers.Composer{u1, u2}}

	err = w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates, nil)
	assert.NoError(t, err)

	assert.NoError(t, checkTarElements(buf, 4))
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
	buf := bytes.NewBuffer(nil)

	s := artifact.NewSigner([]byte(PrivateKey))
	w := NewWriterSigned(buf, s)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV2(upd)
	updates := &Updates{U: []handlers.Composer{u}}

	err = w.WriteArtifact("mender", 2, []string{"asd"}, "name", updates, nil)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 5))
	buf.Reset()

	// error creating v1 signed artifact
	err = w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates, nil)
	assert.Error(t, err)
	assert.Equal(t, "writer: can not create version 1 signed artifact",
		err.Error())
	buf.Reset()

	// write empty artifact
	err = w.WriteArtifact("", 2, []string{}, "", &Updates{}, nil)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 4))
	buf.Reset()

	w = NewWriterSigned(buf, nil)
	err = w.WriteArtifact("mender", 2, []string{"asd"}, "name", updates, nil)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 4))
	buf.Reset()
}

// TODO - manifest-augment and header-augment should be optional.
func TestWriteArtifactV3(t *testing.T) {
	buf := bytes.NewBuffer(nil)

	s := artifact.NewSigner([]byte(PrivateKey))
	w := NewWriterSigned(buf, s)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV3(upd)
	updates := &Updates{U: []handlers.Composer{u}}

	err = w.WriteArtifact("mender", 3, []string{"vexpress-qemu"}, "name", updates, nil)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 7))
	buf.Reset()

	// write empty artifact
	err = w.WriteArtifact("", 3, []string{}, "", &Updates{}, nil)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 6))
	buf.Reset()

	// error writing non-existing
	u = handlers.NewRootfsV3("non-existing")
	updates = &Updates{U: []handlers.Composer{u}}
	err = w.WriteArtifact("mender", 3, []string{"vexpress-qemu"}, "name", updates, nil)
	assert.Error(t, err)
	buf.Reset()

	// Unsigned artifact V3
	buf.Reset()
	w = NewWriter(buf)
	upd, err = MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u = handlers.NewRootfsV3(upd)
	updates = &Updates{U: []handlers.Composer{u}}

	err = w.WriteArtifact("mender", 3, []string{"vexpress-qemu"}, "name", updates, nil)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 6))
	buf.Reset()

	// write empty artifact
	err = w.WriteArtifact("", 3, []string{}, "", &Updates{}, nil)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElements(buf, 5))
	buf.Reset()

	// error writing non-existing
	u = handlers.NewRootfsV3("non-existing")
	updates = &Updates{U: []handlers.Composer{u}}
	err = w.WriteArtifact("mender", 3, []string{"vexpress-qemu"}, "name", updates, nil)
	assert.Error(t, err)
	buf.Reset()
}

func TestWithScripts(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV1(upd)
	updates := &Updates{U: []handlers.Composer{u}}

	scr, err := ioutil.TempFile("", "ArtifactInstall_Enter_10_")
	assert.NoError(t, err)
	defer os.Remove(scr.Name())

	s := new(artifact.Scripts)
	err = s.Add(scr.Name())
	assert.NoError(t, err)

	err = w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates, s)
	assert.NoError(t, err)

	assert.NoError(t, checkTarElements(buf, 3))
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
