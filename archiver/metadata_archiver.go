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

package archiver

import (
	"bytes"
	"encoding/json"

	"github.com/mendersoftware/artifacts/metadata"
)

// NewMetadataArchiver creates streamArchiver used for storing metadata elements
// inside tar archive.
// data is the data structure implementing Validater interface and must be
// a struct that can be converted to JSON (see getJSON below)
// archivePath is the relatve path inside the archive (see tar.Header.Name)
func NewMetadataArchiver(data metadata.Validater, archivePath string) *StreamArchiver {
	j, err := convertToJSON(data)
	if err != nil {
		return nil
	}
	return &StreamArchiver{archivePath, bytes.NewReader(j)}
}

// gets data which is Validated before converting to JSON
func convertToJSON(data metadata.Validater) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	if err := data.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(data)
}
