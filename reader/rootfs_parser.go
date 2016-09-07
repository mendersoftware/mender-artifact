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
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/log"
	"github.com/pkg/errors"
)

type rootfsFile struct {
	name      string
	size      int64
	date      time.Time
	checksum  []byte
	signature []byte
}

type RootfsParser struct {
	files    metadata.Files
	tInfo    metadata.TypeInfo
	metadata metadata.Metadata
	updates  map[string]rootfsFile
	order    string

	sStore string
	dStore io.Writer
}

func NewRootfsParser(sStoreDir string, w io.Writer) RootfsParser {
	return RootfsParser{
		sStore:  sStoreDir,
		dStore:  w,
		updates: map[string]rootfsFile{}}
}

func (rp *RootfsParser) GetOrder() string {
	return rp.order
}

func (rp *RootfsParser) SetOrder(ord string) error {
	rp.order = ord
	return nil
}

func (rp RootfsParser) NeedsDataFile() bool {
	return true
}

func withoutExt(name string) string {
	bName := filepath.Base(name)
	return strings.TrimSuffix(bName, filepath.Ext(bName))
}

func (rp *RootfsParser) ParseHeader(tr *tar.Reader, hPath string) error {
	// iterate through tar archive untill some error occurs or we will
	log.Info("processing rootfs image header")

	if tr == nil {
		return errors.New("rootfs updater: uninitialized tar reader")
	}
	// reach end of archive
	for i := 0; ; i++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we have reached end of archive
			log.Debug("rootfs updater: reached end of archive")
			return nil
		}

		relPath, err := filepath.Rel(hPath, hdr.Name)
		if err != nil {
			return err
		}

		log.Infof("processing rootfs image header file: %v %v", hdr.Name, hdr.Linkname)

		switch {
		case i == 0 && strings.Compare(relPath, "files") == 0:

			if _, err = io.Copy(&rp.files, tr); err != nil {
				return errors.Wrapf(err, "rootfs updater: error reading files")
			}
			for _, file := range rp.files.Files {
				rp.updates[withoutExt(file.File)] = rootfsFile{name: file.File}
			}
		case i == 1 && strings.Compare(relPath, "type-info") == 0:
			// we can skip this one for now
		case i == 2 && strings.Compare(relPath, "meta-data") == 0:
			if _, err = io.Copy(&rp.metadata, tr); err != nil {
				return errors.Wrapf(err, "rootfs updater: error reading metadata")
			}
		case strings.HasPrefix(relPath, "checksums"):
			update, ok := rp.updates[withoutExt(hdr.Name)]
			if !ok {
				return errors.New("rootfs updater: found signature for non existing update file")
			}
			buf := bytes.NewBuffer(nil)
			if _, err = io.Copy(buf, tr); err != nil {
				return errors.Wrapf(err, "rootfs updater: error reading checksum")
			}
			update.checksum = buf.Bytes()
			rp.updates[withoutExt(hdr.Name)] = update
		case strings.HasPrefix(relPath, "signatures"):
			//TODO:
		case strings.HasPrefix(relPath, "scripts"):
			//TODO
		default:
			log.Errorf("rootfs updater: found unsupported element: %v", relPath)
			return errors.New("rootfs updater: unsupported element")
		}
	}
}

// data files are stored in tar.gz format
func (rp *RootfsParser) ParseData(r io.Reader) error {
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
		//TODO: support multiple files
		hdr, err := tar.Next()
		if err == io.EOF {
			// once we reach end of archive break the loop
			break
		}
		log.Infof("processing data file: %v", hdr.Name)
		fh, ok := rp.updates[withoutExt(hdr.Name)]
		if !ok {
			return errors.New("rootfs updater: can not find header info for data file")
		}

		// for calculating hash
		h := sha256.New()

		var w io.Writer
		// if we don't want to store the file just calculate checksum
		if rp.dStore == nil {
			log.Info("skipping storing data file as write location is empty")
			w = h
		} else {
			w = io.MultiWriter(h, rp.dStore)
		}

		if _, err := io.Copy(w, tar); err != nil {
			return err
		}
		sum := h.Sum(nil)
		hSum := make([]byte, hex.EncodedLen(len(sum)))
		hex.Encode(hSum, h.Sum(nil))

		log.Infof("hash of file: %v (%v)", string(hSum), string(fh.checksum))
		if bytes.Compare(hSum, fh.checksum) != 0 {
			return errors.New("rootfs updater: invalid data file checksum")
		}

		fh.date = hdr.ModTime
		fh.size = hdr.Size
		rp.updates[withoutExt(hdr.Name)] = fh
	}
	return nil
}
