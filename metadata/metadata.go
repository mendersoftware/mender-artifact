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

package metadata

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendersoftware/log"
)

type Validater interface {
	Validate() error
}

var ErrInvalidInfo = errors.New("invalid artifacts info")

type MetadataInfo struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
}

func (m MetadataInfo) Validate() error {
	if len(m.Format) == 0 || m.Version == 0 {
		return ErrInvalidInfo
	}
	return nil
}

var ErrInvalidHeaderInfo = errors.New("invalid artifacts info")

type MetadataUpdateType struct {
	Type string `json:"type"`
}

type MetadataHeaderInfo struct {
	Updates []MetadataUpdateType `json:"updates"`
}

func (m MetadataHeaderInfo) Validate() error {
	if len(m.Updates) == 0 {
		return ErrInvalidHeaderInfo
	}
	for _, update := range m.Updates {
		if update == (MetadataUpdateType{}) {
			return ErrInvalidHeaderInfo
		}
	}
	return nil
}

var ErrInvalidTypeInfo = errors.New("invalid type info")

type MetadataTypeInfo struct {
	Rootfs string `json:"rootfs"`
}

func (m MetadataTypeInfo) Validate() error {
	if len(m.Rootfs) == 0 {
		return ErrInvalidTypeInfo
	}
	return nil
}

var ErrInvalidMetadata = errors.New("invalid metadata")

type Metadata struct {
	// we don't know exactly what type of data we will have here
	data map[string]interface{}
}

func (m Metadata) Validate() error {
	if m.data == nil {
		return ErrInvalidMetadata
	}
	// mandatory fields
	var deviceType interface{}
	var imageID interface{}

	for k, v := range m.data {
		if v == nil {
			return ErrInvalidMetadata
		}
		if strings.Compare(k, "DeviceType") == 0 {
			deviceType = v
		}
		if strings.Compare(k, "ImageID") == 0 {
			imageID = v
		}
	}
	if deviceType == nil || imageID == nil {
		return ErrInvalidMetadata
	}
	return nil
}

var ErrInvalidFilesInfo = errors.New("invalid files info")

type MetadataFile struct {
	File string `json:"file"`
}

type MetadataFiles struct {
	Files []MetadataFile `json:"files"`
}

func (m MetadataFiles) Validate() error {
	if len(m.Files) == 0 {
		return ErrInvalidFilesInfo
	}
	for _, file := range m.Files {
		if file == (MetadataFile{}) {
			return ErrInvalidFilesInfo
		}
	}
	return nil
}

type MetadataDirEntry struct {
	Path     string
	IsDir    bool
	Required bool
}

type MetadataArtifactHeader struct {
	Artifacts map[string]MetadataDirEntry
}

var (
	ErrInvalidMetadataElemType = errors.New("Invalid atrifact type")
	ErrMissingMetadataElem     = errors.New("Missing artifact")
	ErrUnsupportedElement      = errors.New("Unsupported artifact")
)

func (mh MetadataArtifactHeader) processEntry(entry string, isDir bool, required map[string]bool) error {
	elem, ok := mh.Artifacts[entry]
	if !ok {
		// for now we are only allowing file name to be user defined
		// the directory structure is pre defined
		if filepath.Base(entry) == "*" {
			return ErrUnsupportedElement
		}
		newEntry := filepath.Dir(entry) + "/*"
		return mh.processEntry(newEntry, isDir, required)
	}

	if isDir != elem.IsDir {
		return ErrInvalidMetadataElemType
	}

	if elem.Required {
		required[entry] = true
	}
	return nil
}

func (mh MetadataArtifactHeader) CheckHeaderStructure(headerDir string) error {
	if _, err := os.Stat(headerDir); os.IsNotExist(err) {
		return os.ErrNotExist
	}
	var required = make(map[string]bool)
	for k, v := range mh.Artifacts {
		if v.Required {
			required[k] = false
		}
	}
	err := filepath.Walk(headerDir,
		func(path string, f os.FileInfo, err error) error {
			pth, err := filepath.Rel(headerDir, path)
			if err != nil {
				return err
			}

			err = mh.processEntry(pth, f.IsDir(), required)
			if err != nil {
				log.Errorf("unsupported element in update metadata header: %v (is dir: %v)", path, f.IsDir())
				return err
			}

			return nil
		})
	if err != nil {
		return err
	}

	// check if all required elements are in place
	for k, v := range required {
		if !v {
			log.Errorf("missing element in update metadata header: %v", k)
			return ErrMissingMetadataElem
		}
	}

	return nil
}
