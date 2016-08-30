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
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/pkg/errors"
)

type MetadataWriter struct {
	updateLocation  string
	headerStructure metadata.MetadataArtifactHeader
	format          string
	version         string
}

var MetadataWriterHeaderFormat = map[string]metadata.MetadataDirEntry{
	// while calling filepath.Walk() `.` (root) directory is included
	// when iterating throug entries in the tree
	".":               {Path: ".", IsDir: true, Required: false},
	"files":           {Path: "files", IsDir: false, Required: false},
	"meta-data":       {Path: "meta-data", IsDir: false, Required: true},
	"type-info":       {Path: "type-info", IsDir: false, Required: true},
	"checksums":       {Path: "checksums", IsDir: true, Required: false},
	"checksums/*":     {Path: "checksums", IsDir: false, Required: false},
	"signatures":      {Path: "signatures", IsDir: true, Required: true},
	"signatures/*":    {Path: "signatures", IsDir: false, Required: true},
	"scripts":         {Path: "scripts", IsDir: true, Required: false},
	"scripts/pre":     {Path: "scripts/pre", IsDir: true, Required: false},
	"scripts/pre/*":   {Path: "scripts/pre", IsDir: false, Required: false},
	"scripts/post":    {Path: "scripts/post", IsDir: true, Required: false},
	"scripts/post/*":  {Path: "scripts/post", IsDir: false, Required: false},
	"scripts/check":   {Path: "scripts/check", IsDir: true, Required: false},
	"scripts/check/*": {Path: "scripts/check/*", IsDir: false, Required: false},
	// we must have data directory containing update
	"data":   {Path: "data", IsDir: true, Required: true},
	"data/*": {Path: "data/*", IsDir: false, Required: true},
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

func (mv MetadataWriter) Write() error {

	// get directories list containing updates
	entries, err := ioutil.ReadDir(mv.updateLocation)
	if err != nil {
		return err
	}

	// create data directory

	// iterate through all directories containing updates
	for _, location := range entries {
		fmt.Printf("file: %v\n", filepath.Join(mv.updateLocation, location.Name()))
		// check files and directories consistency

		err := mv.headerStructure.CheckHeaderStructure(filepath.Join(mv.updateLocation, location.Name()))
		if err != nil {
			return err
		}

		// get list of update files

		// generate `checksums` directory and needed checksums

		// generate `files` file

		// move (and compress) updates from `data` to `../data/location.zip`
	}

	// generate header info

	// (compress header)

	// generate `info`

	// (compress all)

	return nil
}
