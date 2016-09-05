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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/log"
	"github.com/pkg/errors"
)

type ArtifactsReader struct {
	artifact io.ReadCloser
	tar      *tar.Reader

	info  metadata.Info
	hInfo metadata.HeaderInfo
}

func (ar *ArtifactsReader) readHeader(stream io.Reader) error {
	log.Info("Processing header")

	gz, _ := gzip.NewReader(stream)
	defer gz.Close()
	tar := tar.NewReader(gz)

	// first we need to have `header-info`
	hdr, err := tar.Next()
	if strings.Compare(hdr.Name, "header-info") != 0 {
		return errors.New("artifacts reader: element out of order")
	}
	if err = getAndValidateMetadata(&ar.hInfo, tar); err != nil {
		return err
	}
	log.Infof("Contents of header-info: %v", ar.hInfo.Updates)

	for cnt, uType := range ar.hInfo.Updates {
		switch uType.Type {
		// for now we are supporting only "rootfs-image"
		case "rootfs-image":
			if err := ar.readUpdateHeaderBucket(tar, hStreamFormat,
				fmt.Sprintf("%04d", cnt)); err != nil {
				return errors.Wrapf(err, "error reading update bucket")
			}
		default:
			return errors.New("artifacts reader: unsupported update type")
		}
	}
	return nil
}

type ArchiveRawReader struct {
	io.Reader
}

func NewArchiveRawReader(r io.Reader) *ArchiveRawReader {
	return &ArchiveRawReader{r}
}

func (ar ArchiveRawReader) ReadArchive(v interface{}) error {
	if _, err := io.Copy(v.(io.Writer), ar); err != nil {
		return err
	}
	return nil
}

type ArchiveMetadataReader struct {
	io.Reader
}

func NewArchiveMetadataReader(r io.Reader) *ArchiveMetadataReader {
	return &ArchiveMetadataReader{r}
}

func (ar ArchiveMetadataReader) ReadArchive(v interface{}) error {
	dec := json.NewDecoder(ar)
	for {
		if err := dec.Decode(v); err != io.EOF {
			break
		} else if err != nil {
			return err
		}
	}

	data := v.(metadata.Validater)
	if err := data.Validate(); err != nil {
		return err
	}
	return nil
}

type raw []byte

var hStreamFormat = map[string]metadata.DirEntry{
	"files":        {Type: &metadata.Files{}},
	"meta-data":    {Type: &metadata.Metadata{}},
	"type-info":    {Type: &metadata.TypeInfo{}},
	"checksums/*":  {Type: map[string]raw{}},
	"signatures/*": {Type: map[string]raw{}},
	// "scripts/pre/*":   {Type: rawReader{}},
	// "scripts/post/*":  {Type: rawReader{}},
	// "scripts/check/*": {Type: rawReader{}},
}

func (ar ArtifactsReader) getElementFromHeader(header map[string]metadata.DirEntry, path string) (*metadata.DirEntry, error) {
	// Iterare over header all header elements to find one maching path.
	// Header is constructed so that `filepath.Match()` pattern is the
	// same format as header key.
	for k, v := range header {
		match, err := filepath.Match(k, path)
		if err != nil {
			return nil, err
		}
		if match {
			return &v, nil
		}
	}
	return nil, os.ErrNotExist
}

func (ar ArtifactsReader) readUpdateHeaderBucket(tar *tar.Reader,
	header map[string]metadata.DirEntry, uBucket string) error {

	// iterate through tar archive untill some error occurs or we will
	// reach end of archive
	for {
		hdr, err := tar.Next()
		if err == io.EOF {
			// we have reached end of archive
			log.Debug("artifacts reader: reached end of archive")
			log.Errorf("have data struct at end of parsing: %v -> %v", header, header["files"].Type)
			return nil
		}

		// get path relative to current update bucket: [headers/0001/]xxx
		relPath, err := filepath.Rel(filepath.Join("headers", uBucket), hdr.Name)
		if err != nil {
			return err
		}

		// check if given archive file is allowed in header and read it if so
		hElem, err := ar.getElementFromHeader(header, relPath)
		if err != nil {
			return err
		}

		switch hElem.Type.(type) {
		case metadata.Validater:
			getAndValidateMetadata(hElem.Type.(metadata.Validater), tar)

		case map[string]raw:
			ar := NewArchiveRawReader(tar)
			var buf bytes.Buffer
			if err := ar.ReadArchive(&buf); err != nil {
				return err
			}
			log.Errorf("my data in reading: %v", buf.Bytes())
			hElem.Type.(map[string]raw)[filepath.Base(hdr.Name)] = buf.Bytes()

		default:
			return errors.New("unsupported element type")

		}
	}
}

func getAndValidateMetadata(data metadata.Validater, r io.Reader) error {
	dec := json.NewDecoder(r)
	for {
		if err := dec.Decode(&data); err != io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	if err := data.Validate(); err != nil {
		return err
	}
	return nil
}

const (
	info = iota
	header
	data
)

type dataType int

func (ar *ArtifactsReader) readStream(tr *tar.Reader, dType dataType) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}

		switch {
		case strings.Compare(hdr.Name, "info") == 0:
			if dType == info {
				log.Info("Processing info file")

				r := NewArchiveMetadataReader(tr)
				if err = r.ReadArchive(&ar.info); err != nil {
					return err
				}
				log.Infof("Contents of header info: %v", ar.info)
				return nil
			}

		case strings.Compare(hdr.Name, "header.tar.gz") == 0:
			if dType == header {
				if err = ar.readHeader(tr); err != nil {
					return err
				}
				return nil
			}

		case strings.HasPrefix(hdr.Name, "data"):
			if dType == data {
				log.Info("Procesing data file")
				return nil
			}

		default:
			log.Errorf("unsupported element (%v)", hdr)
			return errors.New("artifacts reader: unsupported element in archive")
		}
	}
	return nil
}

func (ar *ArtifactsReader) initTarReader() *tar.Reader {
	if ar.tar == nil {
		ar.tar = tar.NewReader(ar.artifact)
	}
	return ar.tar
}

func (ar *ArtifactsReader) StoreData(writer *io.Writer) error {
	ar.initTarReader()

	if err := ar.readStream(ar.tar, data); err != nil {
		return err
	}
	return nil
}

func (ar *ArtifactsReader) ReadHeader() (*metadata.ArtifactHeader, error) {
	ar.initTarReader()

	hdr := metadata.ArtifactHeader{}
	if err := ar.readStream(ar.tar, header); err != nil {
		return nil, err
	}
	return &hdr, nil
}

func (ar *ArtifactsReader) ReadInfo() (*metadata.Info, error) {
	ar.initTarReader()

	if err := ar.readStream(ar.tar, info); err != nil {
		return nil, err
	}
	return &ar.info, nil
}

func (ar *ArtifactsReader) Read() error {
	info, err := ar.ReadInfo()
	if err != nil {
		return err
	}
	if strings.Compare(info.Format, "mender") != 0 || info.Version != 1 {
		return errors.New("artifacts reader: unsupported artifact format or version")
	}
	if _, err := ar.ReadHeader(); err != nil {
		return err
	}
	if err := ar.StoreData(nil); err != nil {
		return err
	}

	return nil
}
