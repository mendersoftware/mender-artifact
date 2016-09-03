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

type rawReader struct {
	data []byte
}

type jsonReader struct {
	data metadata.Validater
}

// func (ar ArtifactsReader) readUpdateBucket(update metadata.UpdateType, tar *tar.Reader, uBucket int) error {
// 	var files metadata.Files
// 	var tInfo metadata.TypeInfo
// 	var meta metadata.Metadata
//
// 	for i := 0; ; i++ {
// 		hdr, err := tar.Next()
// 		if err == io.EOF {
// 			// we have reached end of archive
// 			break
// 		}
//
// 		// all the files in artifacts header are stored in `headers/xxxx/` directory
// 		relPath, err := filepath.Rel(filepath.Join("headers", fmt.Sprintf("%04d", uBucket)), hdr.Name)
// 		if err != nil {
// 			return err
// 		}
//
// 		switch i {
// 		case 0:
// 			if !strings.Compare(relPath, "files") != 0 {
// 				return errors.New("artifacts reader: element out of order; expecting files")
// 			}
// 			if err = ar.getAndValidateJSONData(&files, tar); err != nil {
// 				return err
// 			}
// 			log.Infof("Contents of files: %v", files)
//
// 		case 1:
// 			//strings.Contains(hdr.Name, "type-info"):
// 			if err = ar.getAndValidateJSONData(&tInfo, tar); err != nil {
// 				return err
// 			}
// 			log.Infof("Contents of type-info: %v", tInfo.Rootfs)
//
// 		case 2:
// 			//strings.Contains(hdr.Name, "meta-data"):
// 			if err = ar.getAndValidateJSONData(&meta, tar); err != nil {
// 				return err
// 			}
// 			log.Infof("Contents of meta-data: %v", meta["ImageID"])
// 		default:
// 			log.Infof("Contents of sub-archive: %v", hdr.Name)
// 			buf := new(bytes.Buffer)
// 			if _, err = io.Copy(buf, tar); err != nil {
// 				return err
// 			}
// 			log.Infof("Contents of sub-archive file: [%v]", string(buf.Bytes()))
//
// 		}
// 	}
//
// 	return nil
// }

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
	if err = ar.getAndValidateJSONData(&ar.hInfo, tar); err != nil {
		return err
	}
	log.Infof("Contents of header-info: %v", ar.hInfo.Updates)

	for cnt := range ar.hInfo.Updates {
		if err := ar.readTarHeaderEntry(tar, hStreamFormat, cnt); err != nil {
			return errors.Wrapf(err, "error reading update bucket")
		}
	}
	return nil
}

type raw []byte

var hStreamFormat = map[string]metadata.DirEntry{
	"files":        {Path: "files", IsDir: false, Required: false, Type: &metadata.Files{}},
	"meta-data":    {Path: "meta-data", IsDir: false, Required: true, Type: &metadata.Metadata{}},
	"type-info":    {Path: "type-info", IsDir: false, Required: true, Type: &metadata.TypeInfo{}},
	"checksums/*":  {Path: "checksums", IsDir: false, Required: false, Type: make(map[string]raw, 1)},
	"signatures/*": {Path: "signatures", IsDir: false, Required: true, Type: make(map[string]raw, 1)},
	// "scripts/pre/*":   {Path: "scripts", IsDir: false, Required: false, Type: rawReader{}},
	// "scripts/post/*":  {Path: "scripts", IsDir: false, Required: false, Type: rawReader{}},
	// "scripts/check/*": {Path: "scripts", IsDir: false, Required: false, Type: rawReader{}},
}

func (ar ArtifactsReader) readTarHeaderEntry(tar *tar.Reader,
	header map[string]metadata.DirEntry, uBucket int) error {

	for {
		hdr, err := tar.Next()
		if err == io.EOF {
			// we have reached end of archive
			log.Debug("artifacts reader: reached end of archive")
			log.Errorf("have data struct at end of parsing: %v", header)
			return nil
		}
		log.Infof("have header element: %v (%v)", hdr.Name, uBucket)

		relPath, err := filepath.Rel(filepath.Join("headers", fmt.Sprintf("%04d", uBucket)), hdr.Name)
		if err != nil {
			return err
		}

		// check for allowed archive entries
		for path, data := range header {
			match, err := filepath.Match(path, relPath)
			if err != nil {
				return err
			}
			if match {
				log.Infof("have match: %v", data)

				switch data.Type.(type) {
				case metadata.Validater:
					d := data.Type
					ar.getAndValidateJSONData(d.(metadata.Validater), tar)
					log.Errorf("have data sss: %v", d)
					header[path] = data

				case map[string]raw:
					buf, _ := ar.getRawData(tar)
					log.Infof("have raw data: %v", string(buf))
					adder := func(data []byte, entry metadata.DirEntry, s string) metadata.DirEntry {
						t := entry.Type.(map[string]raw)
						t[s] = data
						return entry
					}
					e := adder(buf, header[path], filepath.Base(hdr.Name))
					log.Errorf("have e: %v", e)
					header[path] = e

				}
				break

			}

		}
		// log.Errorf("have invalid header entry: %v", hdr.Name)
		// return errors.New("invalid entry in artifacts archive")

	}

}

func (ar ArtifactsReader) getRawData(stream io.Reader) ([]byte, error) {
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, stream); err != nil {
		return nil, err
	}
	log.Errorf("have data: %v", string(buf.Bytes()))
	return buf.Bytes(), nil
}

func (ar ArtifactsReader) getAndValidateJSONData(data metadata.Validater, stream io.Reader) error {
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, stream); err != nil {
		return err
	}
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

				if err = ar.getAndValidateJSONData(&ar.info, tr); err != nil {
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
	if _, err := ar.ReadInfo(); err != nil {
		return err
	}
	if _, err := ar.ReadHeader(); err != nil {
		return err
	}
	if err := ar.StoreData(nil); err != nil {
		return err
	}

	return nil
}
