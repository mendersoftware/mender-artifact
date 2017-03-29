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

package awriter

import (
	"archive/tar"
	"bytes"
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

func checkTarElemsnts(r io.Reader, expected int) error {
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
		return errors.Errorf("invalid number of elements; expecting %d, atual %d",
			expected, i)
	}
	return nil
}

func TestWriteArtifact(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf)

	err := w.WriteArtifact("mender", 1, []string{"asd"}, "name", &Updates{})
	assert.NoError(t, err)

	assert.NoError(t, checkTarElemsnts(buf, 2))
}

func TestWriteArtifactWithUpdates(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf)

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV1(upd)
	updates := &Updates{U: []handlers.Composer{u}}

	err = w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates)
	assert.NoError(t, err)

	assert.NoError(t, checkTarElemsnts(buf, 3))
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

	err = w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates)
	assert.NoError(t, err)

	assert.NoError(t, checkTarElemsnts(buf, 4))
}

func TestWriteArtifactV2(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriterSigned(buf, new(artifact.DummySigner))

	upd, err := MakeFakeUpdate("my test update")
	assert.NoError(t, err)
	defer os.Remove(upd)

	u := handlers.NewRootfsV2(upd)
	updates := &Updates{U: []handlers.Composer{u}}

	err = w.WriteArtifact("mender", 2, []string{"asd"}, "name", updates)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElemsnts(buf, 5))
	buf.Reset()

	// error creating v1 signed artifact
	err = w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates)
	assert.Error(t, err)
	assert.Equal(t, "writer: can not create version 1 signed artifact",
		err.Error())
	buf.Reset()

	// error creating v3 artifact
	err = w.WriteArtifact("mender", 3, []string{"asd"}, "name", updates)
	assert.Error(t, err)
	assert.Equal(t, "writer: unsupported artifact version",
		err.Error())
	buf.Reset()

	// write empty artifact
	err = w.WriteArtifact("", 2, []string{}, "", &Updates{})
	assert.NoError(t, err)
	assert.NoError(t, checkTarElemsnts(buf, 4))
	buf.Reset()

	w = NewWriterSigned(buf, nil)
	err = w.WriteArtifact("mender", 2, []string{"asd"}, "name", updates)
	assert.NoError(t, err)
	assert.NoError(t, checkTarElemsnts(buf, 4))
	buf.Reset()

	// error writing non-existing
	u = handlers.NewRootfsV2("non-existing")
	updates = &Updates{U: []handlers.Composer{u}}
	err = w.WriteArtifact("mender", 3, []string{"asd"}, "name", updates)
	assert.Error(t, err)
	buf.Reset()
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
