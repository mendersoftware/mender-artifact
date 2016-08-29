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

package writer

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/log"
	"github.com/pkg/errors"
)

type MetadataWritter struct {
	updateDir string
}

func getInfoJSON(info *metadata.MetadataInfo) ([]byte, error) {
	if info == nil {
		return nil, nil
	}
	if err := info.Validate(); err == metadata.ErrInvalidInfo {
		return nil, nil
	}
	return json.Marshal(info)
}

func WriteInfo(info *metadata.MetadataInfo) error {
	infoJSON, err := getInfoJSON(info)
	// below should handle passing empty or broken metadata
	if err != nil || infoJSON == nil {
		return errors.New("unable to convert metadata to JSON")
	}
	return nil
}

type MetadataDirEntry struct {
	path     string
	isDir    bool
	required bool
}

var MetadataHeader = map[string]MetadataDirEntry{
	// while calling filepath.Walk() `.` (root) directory is included
	// when iterating throug entries in the tree
	".":               {path: ".", isDir: true, required: false},
	"files":           {path: "files", isDir: false, required: false},
	"meta-data":       {path: "meta-data", isDir: false, required: true},
	"type-info":       {path: "type-info", isDir: false, required: true},
	"checksums":       {path: "checksums", isDir: true, required: false},
	"checksums/*":     {path: "checksums", isDir: false, required: false},
	"signatures":      {path: "signatures", isDir: true, required: true},
	"signatures/*":    {path: "signatures", isDir: false, required: true},
	"scripts":         {path: "scripts", isDir: true, required: false},
	"scripts/pre":     {path: "scripts/pre", isDir: true, required: false},
	"scripts/pre/*":   {path: "scripts/pre", isDir: false, required: false},
	"scripts/post":    {path: "scripts/post", isDir: true, required: false},
	"scripts/post/*":  {path: "scripts/post", isDir: false, required: false},
	"scripts/check":   {path: "scripts/check", isDir: true, required: false},
	"scripts/check/*": {path: "scripts/check/*", isDir: false, required: false},
}

var (
	ErrInvalidMetadataElemType = errors.New("Invalid atrifact type")
	ErrMissingMetadataElem     = errors.New("Missing artifact")
	ErrUnsupportedElement      = errors.New("Unsupported artifact")
)

func processEntry(entry string, isDir bool, required map[string]bool) error {
	elem, ok := MetadataHeader[entry]
	if !ok {
		// for now we are only allowing file name to be user defined
		// the directory structure is pre defined
		if filepath.Base(entry) == "*" {
			return ErrUnsupportedElement
		}
		newEntry := filepath.Dir(entry) + "/*"
		return processEntry(newEntry, isDir, required)
	}

	if isDir != elem.isDir {
		return ErrInvalidMetadataElemType
	}

	if elem.required {
		required[entry] = true
	}
	return nil
}

func (mv MetadataWritter) checkHeaderStructure() error {
	var required = make(map[string]bool)
	for k, v := range MetadataHeader {
		if v.required {
			required[k] = false
		}
	}
	err := filepath.Walk(mv.updateDir,
		func(path string, f os.FileInfo, err error) error {
			pth, err := filepath.Rel(mv.updateDir, path)
			if err != nil {
				return err
			}

			err = processEntry(pth, f.IsDir(), required)
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
