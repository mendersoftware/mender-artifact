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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mendersoftware/artifacts/metadata"
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

func (ar ArtifactsReader) readHeader(stream io.Reader) error {
	log.Info("Processing header")

	gz, _ := gzip.NewReader(stream)
	tar := tar.NewReader(gz)
	for {
		hdr, err := tar.Next()
		if err == io.EOF {
			// we have reached end of archive
			break
		}

		switch {
		case strings.Compare(hdr.Name, "header-info") == 0:
			var meta metadata.HeaderInfo
			if err = ar.getAndValidateData(&meta, tar); err != nil {
				return err
			}
			log.Infof("Contents of header-info: %v", meta.Updates)

		case strings.Contains(hdr.Name, "files"):
			var meta metadata.Files
			if err = ar.getAndValidateData(&meta, tar); err != nil {
				return err
			}
			log.Infof("Contents of files: %v", meta)

		case strings.Contains(hdr.Name, "type-info"):
			var meta metadata.TypeInfo
			if err = ar.getAndValidateData(&meta, tar); err != nil {
				return err
			}
			log.Infof("Contents of type-info: %v", meta.Rootfs)

		case strings.Contains(hdr.Name, "meta-data"):
			var meta metadata.Metadata
			if err = ar.getAndValidateData(&meta, tar); err != nil {
				return err
			}
			log.Infof("Contents of meta-data: %v", meta["ImageID"])
		default:
			log.Infof("Contents of sub-archive: %v", hdr.Name)
			buf := new(bytes.Buffer)
			if _, err = io.Copy(buf, tar); err != nil {
				return err
			}
			log.Infof("Contents of sub-archive file: [%v]", string(buf.Bytes()))

		}

	}
	return nil
}

func (ar ArtifactsReader) getAndValidateData(data metadata.Validater, stream io.Reader) error {
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, stream); err != nil {
		return err
	}

	fmt.Printf("have data: %v\n\n\n", string(buf.Bytes()))

	if buf.Len() == 0 {
		return errors.New("artifacts reader: empty file")
	}

	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		return err
	}

	if err := data.Validate(); err != nil {
		return err
	}
	return nil
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
		switch {
		case strings.Compare(hdr.Name, "info") == 0:
			log.Info("Processing info file")
			var info metadata.Info
			if err = ar.getAndValidateData(&info, tr); err != nil {
				return err
			}
			log.Infof("Contents of header info: %v", info)

		case strings.Compare(hdr.Name, "header.tar.gz") == 0:
			if err = ar.readHeader(tr); err != nil {
				return err
			}

		case strings.HasPrefix(hdr.Name, "data"):
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
