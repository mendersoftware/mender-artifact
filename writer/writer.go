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
	"io/ioutil"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/pkg/errors"
)

type MetadataWritter struct {
	updateLocation string
	format         string
	version        string
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

func (mv MetadataWritter) Write() error {

	// get directories list containing updates
	entries, err := ioutil.ReadDir(mv.updateLocation)
	if err != nil {
		return err
	}

	// create data directory

	// iterate through all directories containing updates
	for _, location := range entries {
		// check files and directories consistency
		header := metadata.MetadataArtifactHeader{Artifacts: metadata.MetadataHeaderFormat}
		header.CheckHeaderStructure(location.Name())

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
