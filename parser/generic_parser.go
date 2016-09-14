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

package parser

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/pkg/errors"
)

type UpdateFile struct {
	Name     string
	Size     int64
	Date     time.Time
	checksum []byte
}

type GenericParser struct {
	Metadata metadata.Metadata
	files    metadata.Files
	updates  map[string]UpdateFile
}

func (rp *GenericParser) ReadUpdateType() (*metadata.UpdateType, error) {
	return nil, nil
}
func (rp *GenericParser) ReadUpdateFiles() error {
	return nil
}
func (rp *GenericParser) ReadDeviceType() (string, error) {
	return "", nil
}
func (rp *GenericParser) ReadMetadata() (*metadata.Metadata, error) {
	return nil, nil
}

func NewGenericParser() Parser {
	return &GenericParser{
		updates: map[string]UpdateFile{}}
}

func (rp *GenericParser) ParseHeader(tr *tar.Reader, hPath string) error {
	if tr == nil {
		return errors.New("parser: uninitialized tar reader")
	}
	// reach end of archive
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we have reached end of archive
			return nil
		} else if err != nil {
			return errors.Wrapf(err, "parser: error reading archive header")
		}

		relPath, err := filepath.Rel(hPath, hdr.Name)
		if err != nil {
			return err
		}

		switch {
		case strings.Compare(relPath, "files") == 0:
			if _, err = io.Copy(&rp.files, tr); err != nil {
				return errors.Wrapf(err, "parser: error reading files")
			}
			for _, file := range rp.files.File {
				rp.updates[withoutExt(file)] = UpdateFile{Name: file}
			}
		case strings.Compare(relPath, "type-info") == 0:
			// we can skip this one for now
		case strings.Compare(relPath, "meta-data") == 0:
			if _, err = io.Copy(&rp.Metadata, tr); err != nil {
				return errors.Wrapf(err, "parser: error reading metadata")
			}
		case strings.HasPrefix(relPath, "checksums"):
			update, ok := rp.updates[withoutExt(hdr.Name)]
			if !ok {
				return errors.New("parser: found checksum for non existing update file")
			}
			buf := bytes.NewBuffer(nil)
			if _, err = io.Copy(buf, tr); err != nil {
				return errors.Wrapf(err, "rparser: error reading checksum")
			}
			update.checksum = buf.Bytes()
			rp.updates[withoutExt(hdr.Name)] = update
		}
	}
}

// data files are stored in tar.gz format
func (rp *GenericParser) ParseData(r io.Reader) error {
	if r == nil {
		return errors.New("rootfs updater: uninitialized tar reader")
	}
	//[data.tar].gz
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	//data[.tar].gz
	tar := tar.NewReader(gz)
	// iterate over the files in tar archive
	for {
		hdr, err := tar.Next()
		if err == io.EOF {
			// once we reach end of archive break the loop
			break
		} else if err != nil {
			return errors.Wrapf(err, "rootfs updater: error reading archive")
		}
		fh, ok := rp.updates[withoutExt(hdr.Name)]
		if !ok {
			return errors.New("rootfs updater: can not find header info for data file")
		}

		// for calculating hash
		h := sha256.New()
		if _, err := io.Copy(h, r); err != nil {
			return err
		}
		sum := h.Sum(nil)
		hSum := make([]byte, hex.EncodedLen(len(sum)))
		hex.Encode(hSum, h.Sum(nil))

		if bytes.Compare(hSum, fh.checksum) != 0 {
			return errors.New("rootfs updater: invalid data file checksum: " + hdr.Name)
		}

		fh.Date = hdr.ModTime
		fh.Size = hdr.Size
		rp.updates[withoutExt(hdr.Name)] = fh
	}
	return nil
}

func (rp *GenericParser) ArchiveData(tw *tar.Writer, srcDir, dst string) error {
	return nil
}

func (rp *GenericParser) ArchiveHeader(tw *tar.Writer, srcDir, dstDir string) error {
	return nil
}
