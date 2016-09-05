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

type ArtifactHeader struct {
	hInfo   metadata.HeaderInfo
	updates []updateBucket
}

type ArtifactsReader struct {
	artifact io.ReadCloser
	tar      *tar.Reader
	dStore   io.Writer
	sStore   io.Writer

	ArtifactHeader
	info metadata.Info
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
	r := NewArchiveMetadataReader(tar)
	if err = r.ReadArchive(&ar.hInfo); err != nil {
		return err
	}
	log.Infof("Contents of header-info: %v", ar.hInfo.Updates)

	for cnt, uType := range ar.hInfo.Updates {
		switch uType.Type {
		// for now we are supporting only "rootfs-image"
		case "rootfs-image":
			// set what data we need to read (maybe for different type we will need
			// different files to read)
			if err := ar.processRootfsImage(tar, fmt.Sprintf("%04d", cnt)); err != nil {
				return errors.Wrapf(err, "error processing rootfs-image type files")
			}
		default:
			return errors.New("artifacts reader: unsupported update type")
		}
	}
	return nil
}

type updateBucket struct {
	location        string
	meta            metadata.Metadata
	updateArtifacts map[string]updateArtifact
}

type updateArtifact struct {
	name      string
	bucket    string
	checksum  []byte
	signature []byte
}

func (ub *updateBucket) addUpdateArtifact(name, bucket string) {
	ub.updateArtifacts[strings.TrimSuffix(name, filepath.Ext(name))] =
		updateArtifact{name: name, bucket: bucket}
}

func (ub *updateBucket) addUpdateChecksum(ch []byte, name string) error {
	upd, ok := ub.updateArtifacts[strings.TrimSuffix(name, filepath.Ext(name))]
	if !ok {
		return os.ErrNotExist
	}
	log.Errorf("have data: %v", ch)
	upd.checksum = ch
	ub.updateArtifacts[strings.TrimSuffix(name, filepath.Ext(name))] = upd
	return nil
}

func (ub *updateBucket) addUpdateSignature(s []byte, name string) error {
	upd, ok := ub.updateArtifacts[strings.TrimSuffix(name, filepath.Ext(name))]
	if !ok {
		return os.ErrNotExist
	}
	upd.signature = s
	ub.updateArtifacts[strings.TrimSuffix(name, filepath.Ext(name))] = upd
	return nil
}

func (ar *ArtifactsReader) processRootfsImage(tr *tar.Reader, bucket string) error {

	updB := updateBucket{updateArtifacts: map[string]updateArtifact{}}
	// iterate through tar archive untill some error occurs or we will
	// reach end of archive
	for i := 0; ; i++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we have reached end of archive
			log.Debug("artifacts reader: reached end of archive")
			ar.updates = append(ar.updates, updB)
			log.Errorf("artifacts reader: %v", updB.updateArtifacts)
			return nil
		}
		// get path relative to current update bucket: [headers/0001/]xxx
		relPath, err := filepath.Rel(filepath.Join("headers", bucket), hdr.Name)
		if err != nil {
			return err
		}

		switch {
		case i == 0 && strings.Compare(relPath, "files") == 0:
			files := metadata.Files{}
			r := NewArchiveMetadataReader(tr)
			if err = r.ReadArchive(&files); err != nil {
				return err
			}
			for _, file := range files.Files {
				updB.addUpdateArtifact(file.File, bucket)
			}

		case i == 1 && strings.Compare(relPath, "type-info") == 0:
			tInfo := metadata.TypeInfo{}
			r := NewArchiveMetadataReader(tr)
			if err = r.ReadArchive(&tInfo); err != nil {
				return err
			}
		case i == 2 && strings.Compare(relPath, "meta-data") == 0:
			mData := metadata.Metadata{}
			r := NewArchiveMetadataReader(tr)
			if err = r.ReadArchive(&mData); err != nil {
				return err
			}
			updB.meta = mData
		case strings.HasPrefix(relPath, "checksums"):
			r := NewArchiveRawReader(tr)
			buf := bytes.NewBuffer(nil)
			if err = r.ReadArchive(buf); err != nil {
				return err
			}
			if err = updB.addUpdateChecksum(buf.Bytes(), filepath.Base(relPath)); err != nil {
				return err
			}
		case strings.HasPrefix(relPath, "signatures"):
			r := NewArchiveRawReader(tr)
			buf := bytes.NewBuffer(nil)
			if err = r.ReadArchive(buf); err != nil {
				return err
			}
			if err = updB.addUpdateSignature(buf.Bytes(), filepath.Base(relPath)); err != nil {
				return err
			}
		case strings.HasPrefix(relPath, "scripts"):
			// TODO:
		default:
			return errors.New("artifacts reader: element not supported")
		}
	}
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

type ArchiveRawReader struct {
	io.Reader
}

func NewArchiveRawReader(r io.Reader) *ArchiveRawReader {
	return &ArchiveRawReader{r}
}

func (ar *ArchiveRawReader) ReadArchive(v interface{}) error {
	if _, err := io.Copy(v.(*bytes.Buffer), ar); err != nil {
		return err
	}
	return nil
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

				// for calculating hash
				h := sha256.New()

				// for storing and unpacking data file
				gz, _ := gzip.NewReader(tr)
				defer gz.Close()

				tar := tar.NewReader(gz)
				for {
					_, err := tar.Next()
					if err == io.EOF {
						break
					}
					w := io.MultiWriter(h, ar.dStore)
					if _, err := io.Copy(w, tar); err != nil {
						return err
					}
					chksum := h.Sum(nil)
					checksum := make([]byte, hex.EncodedLen(len(chksum)))
					hex.Encode(checksum, h.Sum(nil))
					log.Infof("hash of file: %v\n", string(checksum))
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

func (ar *ArtifactsReader) StoreData(writer *io.Writer) error {
	ar.initTarReader()

	if err := ar.readStream(ar.tar, data); err != nil {
		return err
	}
	return nil
}

func (ar *ArtifactsReader) ReadHeader() (*ArtifactHeader, error) {
	ar.initTarReader()

	if err := ar.readStream(ar.tar, header); err != nil {
		return nil, err
	}
	return &ar.ArtifactHeader, nil
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
