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

package handlers

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
)

type Generic struct {
	updateType string
	files      map[string](*artifact.File)
}

func NewGeneric(t string) *Generic {
	return &Generic{
		updateType: t,
		files:      make(map[string](*artifact.File)),
	}
}

func (g *Generic) GetUpdateFiles() [](*artifact.File) {
	list := make([](*artifact.File), len(g.files))
	i := 0
	for _, f := range g.files {
		list[i] = f
		i++
	}
	return list
}

func (g *Generic) GetType() string {
	return g.updateType
}

// Copy is implemented only to satisfy Installer interface.
// Generic parser is not supposed to be copied.
func (g *Generic) Copy() artifact.Installer {
	return nil
}

func stripSum(path string) string {
	bName := filepath.Base(path)
	return strings.TrimSuffix(bName, filepath.Ext(bName))
}

func (g *Generic) SetFromHeader(r io.Reader, path string) error {
	switch {
	case filepath.Base(path) == "files":
		files, err := parseFiles(r)
		if err != nil {
			return err
		}
		for _, f := range files.FileList {
			g.files[filepath.Base(f)] = &artifact.File{
				Name: f,
			}
		}
	case filepath.Base(path) == "type-info":
		// TODO:

	case filepath.Base(path) == "meta-data":
		// TODO:

	case match(artifact.HeaderDirectory+"/*/checksums/*", path):
		buf := bytes.NewBuffer(nil)
		if _, err := io.Copy(buf, r); err != nil {
			return errors.Wrapf(err, "update: error reading checksum")
		}
		key := stripSum(path)
		if _, ok := g.files[key]; !ok {
			return errors.Errorf("generic handler: can not find data file: %v", key)
		}
		g.files[key].Checksum = buf.Bytes()

	case match(artifact.HeaderDirectory+"/*/signatres/*", path):
	case match(artifact.HeaderDirectory+"/*/scripts/*/*", path):

	default:
		return errors.Errorf("update: unsupported file: %v", path)
	}
	return nil
}

func (g *Generic) Install(r io.Reader, f os.FileInfo) error {
	if _, err := io.Copy(ioutil.Discard, r); err != nil {
		return err
	}
	key := filepath.Base(f.Name())
	if _, ok := g.files[key]; !ok {
		return errors.Errorf("generic handler: can not find data file: %v", key)
	}
	g.files[key].Size = f.Size()
	g.files[key].Date = f.ModTime()

	return nil
}
