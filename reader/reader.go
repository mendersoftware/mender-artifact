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

package reader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"

	"github.com/mendersoftware/log"
)

type StreamArchiver struct {
	name   string
	data   []byte
	buffer *bytes.Buffer
}

type ArtifactsReader struct {
	artifact io.ReadCloser
}

func (ar ArtifactsReader) readStream(stream io.Reader) error {
	tr := tar.NewReader(stream)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we have reached end of archive
			break
		}
		log.Infof("Contents of archive: %s", hdr.Name)
		switch hdr.Name {
		case "info":
			log.Info("Processing info file")
			buf := new(bytes.Buffer)
			if _, err = io.Copy(buf, tr); err != nil {
				return err
			}
			log.Infof("Received info: %s", string(buf.Bytes()))

		case "header.tar.gz":
			log.Info("Processing header")
			buf := new(bytes.Buffer)
			gz := gzip.NewReader(buf)

		default:
			log.Info("Procesing data file")
		}

	}
	return nil
}

func (ar ArtifactsReader) Read() error {

	if err := ar.readStream(ar.artifact); err != nil {
		return err
	}
	return nil
}
