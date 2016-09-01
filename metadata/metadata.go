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

// Validater interface is providing a method of validating data.
type Validater interface {
	Validate() error
}

// ErrValidatingData is an error returned by Validate() in case of
// invalid data.
var ErrValidatingData = errors.New("error validating data")

// Info contains the information about the format and the version
// of artifact archive.
type Info struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
}

// Validate performs sanity checks on artifact info.
func (i Info) Validate() error {
	if len(i.Format) == 0 || i.Version == 0 {
		return ErrValidatingData
	}
	return nil
}

// UpdateType provides information about the type of update.
// At the moment we are supporting only "rootfs-image" type.
type UpdateType struct {
	Type string `json:"type"`
}

// HeaderInfo contains information of numner and type of update files
// archived in Mender metadata archive.
type HeaderInfo struct {
	Updates []UpdateType `json:"updates"`
}

// Validate checks if header-info structure is correct.
func (hi HeaderInfo) Validate() error {
	if len(hi.Updates) == 0 {
		return ErrValidatingData
	}
	for _, update := range hi.Updates {
		if update == (UpdateType{}) {
			return ErrValidatingData
		}
	}
	return nil
}

// TypeInfo provides information of type of individual updates
// archived in artifacts archive.
type TypeInfo struct {
	Rootfs string `json:"rootfs"`
}

// Validate validates corectness of TypeInfo.
func (ti TypeInfo) Validate() error {
	if len(ti.Rootfs) == 0 {
		return ErrValidatingData
	}
	return nil
}

// Metadata contains artifacts metadata information. The exact metadata fields
// are user-defined and are not specified. The only requirement is that those
// must be stored in a for of JSON.
// The only fields which must exist are 'DeviceType' and 'ImageId'.
type Metadata struct {
	// we don't know exactly what type of data we will have here
	data map[string]interface{}
}

// Validate check corecness of artifacts metadata. Since the exact format is
// nost specified we are only checking if those could be converted to JSON.
// The only fields which must exist are 'DeviceType' and 'ImageId'.
func (m Metadata) Validate() error {
	if m.data == nil {
		return ErrValidatingData
	}
	// mandatory fields
	var deviceType interface{}
	var imageID interface{}

	for k, v := range m.data {
		if v == nil {
			return ErrValidatingData
		}
		if strings.Compare(k, "DeviceType") == 0 {
			deviceType = v
		}
		if strings.Compare(k, "ImageID") == 0 {
			imageID = v
		}
	}
	if deviceType == nil || imageID == nil {
		return ErrValidatingData
	}
	return nil
}

// File is a single file being a part of Files struct.
type File struct {
	File string `json:"file"`
}

// Files represents the list of file names that make up the payload for given
// update.
type Files struct {
	Files []File `json:"files"`
}

// Validate checks format of Files.
func (f Files) Validate() error {
	if len(f.Files) == 0 {
		return ErrValidatingData
	}
	for _, file := range f.Files {
		if file == (File{}) {
			return ErrValidatingData
		}
	}
	return nil
}

// DirEntry contains information about single enttry of artifact archive.
type DirEntry struct {
	// absolute path to file or directory
	Path string
	// specifies if entry is directory or file
	IsDir bool
	// some files are optional thus ew want to check if given entry is needed
	Required bool
}

// ArtifactHeader is a filesystem structure containing information about
// all required elements of given Mender artifact.
type ArtifactHeader struct {
	Artifacts map[string]DirEntry
}

var (
	ErrInvalidMetadataElemType = errors.New("Invalid atrifact type")
	ErrMissingMetadataElem     = errors.New("Missing artifact")
	ErrUnsupportedElement      = errors.New("Unsupported artifact")
)

func (mh ArtifactHeader) processEntry(entry string, isDir bool, required map[string]bool) error {
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

// CheckHeaderStructure checks if headerDir directory contains all needed
// files and sub-directories for creating Mender artifact.
func (mh ArtifactHeader) CheckHeaderStructure(headerDir string) error {
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
