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

// implements ReadArchiver interface
type streamArchiver struct {
	name   string
	data   []byte
	buffer *bytes.Buffer
}

func NewStreamArchiver(data metadata.Validater, name string) *streamArchiver {
	j, err := getJSON(data)
	if err != nil {
		return nil
	}
	return &streamArchiver{
		name:   name,
		data:   j,
		buffer: bytes.NewBuffer(j),
	}
}

func (str streamArchiver) Read(p []byte) (n int, err error) {
	return str.buffer.Read(p)
}

func (str streamArchiver) Close() error { return nil }

func (str streamArchiver) GetHeader() (*tar.Header, error) {
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
