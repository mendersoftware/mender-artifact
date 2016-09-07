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
	"archive/tar"
	"bytes"
	"encoding/json"

	"github.com/mendersoftware/artifacts/metadata"
)

// StreamArchiver implements ReadArchiver interface
type StreamArchiver struct {
	name string
	data []byte
	*bytes.Reader
}

// NewStreamArchiver creates streamArchiver used for storing plain text files
// inside tar archive.
// data is the plain data that will be stored in archive file
// name is the relatve path inside the archive (see tar.Header.Name)
func NewStreamArchiver(data []byte, name string) *StreamArchiver {
	return &StreamArchiver{name, data, bytes.NewReader(data)}
}

// NewJSONStreamArchiver creates streamArchiver used for storing JSON files
// inside tar archive.
// data is the data structure implementing Validater interface and must be
// a struct that can be converted to JSON (see getJSON below)
// name is the relatve path inside the archive (see tar.Header.Name)
func NewJSONStreamArchiver(data metadata.Validater, name string) *StreamArchiver {
	j, err := getJSON(data)
	if err != nil {
		return nil
	}
	return &StreamArchiver{name, j, bytes.NewReader(j)}
}

// Open is implemented as a path of ReadArchiver interface
func (str *StreamArchiver) Open() error { return nil }

// Close is implemented as a path of ReadArchiver interface
func (str *StreamArchiver) Close() error { return nil }

// GetHeader is a path of ReadArchiver interface. It returns tar.Header which
// is then writtem as a part of archive header.
func (str *StreamArchiver) GetHeader() (*tar.Header, error) {
	hdr := &tar.Header{
		Name: str.name,
		Mode: 0600,
		Size: int64(len(str.data)),
	}
	return hdr, nil
}

// gets data which is Validated before converting to JSON
func getJSON(data metadata.Validater) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	if err := data.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(data)
}
