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
	"archive/tar"
	"bytes"
	"io"

	"github.com/mendersoftware/log"
	"github.com/pkg/errors"
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

func (str *StreamArchiver) Archive(tw *tar.Writer) error {

	hdr := &tar.Header{
		Name: str.name,
		Mode: 0600,
		Size: int64(len(str.data)),
	}
	log.Debugf("arch: have header: %v", hdr)
	if err := tw.WriteHeader(hdr); err != nil {
		return errors.Wrapf(err, "arch: can not write header")
	}

	_, err := io.Copy(tw, str.Reader)
	if err != nil {
		return errors.Wrapf(err, "arch: can not write bocy")
	}
	return nil
}
