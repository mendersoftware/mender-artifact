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
	dStore   io.Writer
	sStore   string

	Header
	info metadata.Info
}

// func NewArtifactReader(r io.ReadCloser) *ArtifactsReader {
// 	h := map[string]interface{}{}
//
// }

type Header struct {
	hInfo   metadata.HeaderInfo
	updates map[string]interface{}
}

func (ar *ArtifactsReader) readHeader(r io.Reader) error {
	log.Info("Processing header")

	gz, _ := gzip.NewReader(r)
	defer gz.Close()
	tar := tar.NewReader(gz)

	// first we need to have `header-info`
	hdr, err := tar.Next()
	if err != nil {
		return errors.New("artifacts reader: can not read tar header")
	}
	if strings.Compare(hdr.Name, "header-info") != 0 {
		return errors.New("artifacts reader: element out of order")
	}

	if _, err := io.Copy(&ar.hInfo, tar); err != nil {
		return err
	}
	log.Infof("Contents of header-info: %v", ar.hInfo.Updates)

	for cnt, uType := range ar.hInfo.Updates {
		switch uType.Type {
		// for now we are supporting only "rootfs-image"
		case "rootfs-image":
			// set what data we need to read (maybe for different type we will need
			// different files to read)
			b := fmt.Sprintf("%04d", cnt)
			rImage, err := ar.processRootfsImageHdr(tar, b)
			if err != nil {
				return errors.Wrapf(err, "error processing rootfs-image type files")
			}
			ar.Header.updates[b+".tar.gz"] = rImage
		default:
			return errors.New("artifacts reader: unsupported update type")
		}
	}
	return nil
}

type rootfsEntry struct {
	order   int
	matcher string
	writer  io.Writer
}

type rootfsImage map[string]rootfsEntry

func (ri rootfsImage) getFiles() *metadata.Files {
	if data, ok := ri["files"].writer.(*metadata.Files); ok {
		return data
	}
	return nil
}

func (ri rootfsImage) getTypeInfo() *metadata.TypeInfo {
	if data, ok := ri["type-info"].writer.(*metadata.TypeInfo); ok {
		return data
	}
	return nil
}

func (ri rootfsImage) getMetaData() *metadata.Metadata {
	if data, ok := ri["metadata"].writer.(*metadata.Metadata); ok {
		return data
	}
	return nil
}

func (ri rootfsImage) getChecksum() []byte {
	if data, ok := ri["checksums/*"].writer.(*bytes.Buffer); ok {
		return data.Bytes()
	}
	return nil
}

func (ri rootfsImage) getSignatures() []byte {
	if data, ok := ri["signatures/*"].writer.(*bytes.Buffer); ok {
		return data.Bytes()
	}
	return nil
}

const orderDoNotMatter = 736334

func (ar ArtifactsReader) getElementFromHeader(header rootfsImage, path string) (*rootfsEntry, error) {
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

func (ar *ArtifactsReader) processRootfsImageHdr(tr *tar.Reader, bucket string) (rootfsImage, error) {

	iContent := rootfsImage{
		"files":         rootfsEntry{0, "files", &metadata.Files{}},
		"type-info":     rootfsEntry{1, "type-info", &metadata.TypeInfo{}},
		"meta-data":     rootfsEntry{2, "meta-data", &metadata.Metadata{}},
		"checksums/*":   rootfsEntry{orderDoNotMatter, "checksums", bytes.NewBuffer(nil)},
		"signatures/*":  rootfsEntry{orderDoNotMatter, "signatures", bytes.NewBuffer(nil)},
		"scripts/pre/*": rootfsEntry{orderDoNotMatter, "scripts", nil},
	}

	// iterate through tar archive untill some error occurs or we will
	// reach end of archive
	for i := 0; ; i++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we have reached end of archive
			log.Debug("artifacts reader: reached end of archive")
			return iContent, nil
		}

		// get path relative to current update bucket: [headers/0001/]xxx
		relPath, err := filepath.Rel(filepath.Join("headers", bucket), hdr.Name)
		if err != nil {
			return nil, err
		}

		elem, err := ar.getElementFromHeader(iContent, relPath)
		if err != nil {
			return nil, err
		}

		if elem.order != orderDoNotMatter {
			if elem.order != i {
				return nil, errors.New("artifacts reader: element out of order")
			}
		}

		switch elem.matcher {
		case "scripts":
			f, er := os.Create(filepath.Join(ar.sStore, relPath))
			if er != nil {
				return nil, er
			}
			new := rootfsEntry{orderDoNotMatter, "scripts", f}
			iContent[relPath] = new
			if _, err = io.Copy(new.writer, tr); err != nil {
				return nil, err
			}
			break

			// if we ever will need multiple files
			// case "checksums":
			// 	new := rootfsEntry{orderDoNotMatter, "signatures", bytes.NewBuffer(nil)}
			// 	iContent[relPath] = new
			// 	if _, err = io.Copy(new.writer, tr); err != nil {
			// 		return nil, err
			// 	}
			// 	break
		}

		if _, err = io.Copy(elem.writer, tr); err != nil {
			return nil, err
		}
	}
}

type dataType int

const (
	info = iota
	header
	data
)

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

				if _, err = io.Copy(&ar.info, tr); err != nil {
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
				log.Infof("Procesing data file(s): %v %v", hdr.Name, ar.Header.updates[filepath.Base(hdr.Name)])
				ah, ok := ar.Header.updates[filepath.Base(hdr.Name)]
				if !ok {
					return errors.New("artifacts reader: invalid data file")
				}
				rfs, ok := ah.(rootfsImage)
				if !ok {
					return errors.New("artifacts reader: invalid update type")
				}

				// for calculating hash
				h := sha256.New()

				// for storing and unpacking data file
				gz, _ := gzip.NewReader(tr)
				defer gz.Close()

				tar := tar.NewReader(gz)
				for {
					hdr, err = tar.Next()
					if err == io.EOF {
						break
					}
					log.Errorf("name: %v %v", hdr.Name, rfs.getFiles())
					w := io.MultiWriter(h, ar.dStore)
					if _, err := io.Copy(w, tar); err != nil {
						return err
					}
					chck := h.Sum(nil)
					chH := make([]byte, hex.EncodedLen(len(chck)))
					hex.Encode(chH, h.Sum(nil))
					log.Infof("hash of file: %v:%v\n", string(chH), string(rfs.getChecksum()))
					if bytes.Compare(chH, rfs.getChecksum()) != 0 {
						return errors.New("artifacts reader: invalid checksum")
					}
				}

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

func (ar *ArtifactsReader) StoreData(w io.Writer) error {
	ar.initTarReader()
	ar.dStore = w

	if err := ar.readStream(ar.tar, data); err != nil {
		return err
	}
	return nil
}

func (ar *ArtifactsReader) ReadHeader() (*Header, error) {
	ar.initTarReader()

	if err := ar.readStream(ar.tar, header); err != nil {
		return nil, err
	}
	return &ar.Header, nil
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
	if err := ar.StoreData(ar.dStore); err != nil {
		return err
	}

	return nil
}
