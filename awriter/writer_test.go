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
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/stretchr/testify/assert"
)

func TestWriteArtifact(t *testing.T) {
	buf := bytes.NewBuffer(nil)

	w := NewWriter(buf)

	err := w.WriteArtifact("mender", 1, []string{"asd"}, "name", &artifact.Updates{})
	assert.NoError(t, err)

	f, _ := ioutil.TempFile("", "update")
	_, err = io.Copy(f, buf)
	assert.NoError(t, err)

	os.Remove(f.Name())
}

func TestWriteArtifactWithUpdates(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf)

	df, _ := ioutil.TempFile("", "update_data")
	defer os.Remove(df.Name())
	df.WriteString("this is a fake update")
	df.Close()

	u := handlers.NewRootfsV1(df.Name())
	updates := &artifact.Updates{U: []artifact.Composer{u}}

	err := w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates)
	assert.NoError(t, err)

	f, _ := ioutil.TempFile("", "update")
	_, err = io.Copy(f, buf)
	assert.NoError(t, err)

	os.Remove(f.Name())
}

func TestWriteMultipleUpdates(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriter(buf)

	df, _ := ioutil.TempFile("", "update_data")
	defer os.Remove(df.Name())
	df.WriteString("this is a fake update")
	df.Close()

	u1 := handlers.NewRootfsV1(df.Name())
	u2 := handlers.NewRootfsV1(df.Name())
	updates := &artifact.Updates{U: []artifact.Composer{u1, u2}}

	err := w.WriteArtifact("mender", 1, []string{"asd"}, "name", updates)
	assert.NoError(t, err)

	f, _ := ioutil.TempFile("", "update")
	_, err = io.Copy(f, buf)
	assert.NoError(t, err)

	os.Remove(f.Name())
}

func TestWriteArtifactV2(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	w := NewWriterSigned(buf, new(artifact.DummySigner))

	df, _ := ioutil.TempFile("", "update_data")
	defer os.Remove(df.Name())
	df.WriteString("this is a fake update")
	df.Close()

	u := handlers.NewRootfsV2(df.Name())
	updates := &artifact.Updates{U: []artifact.Composer{u}}

	err := w.WriteArtifact("mender", 2, []string{"asd"}, "name", updates)
	assert.NoError(t, err)

	f, _ := ioutil.TempFile("", "update")
	_, err = io.Copy(f, buf)
	assert.NoError(t, err)

	os.Remove(f.Name())
}
