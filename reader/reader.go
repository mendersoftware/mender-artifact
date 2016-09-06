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
	"compress/gzip"
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
	parsers  Parsers

	info metadata.Info
	*Header
}

// func NewArtifactReader(r io.ReadCloser) *ArtifactsReader {
// 	h := map[string]interface{}{}
//
// }

type Header struct {
	hInfo   metadata.HeaderInfo
	updates map[string]ArtifactParser
}

func (h *Header) addUpdData(p ArtifactParser, upd string) {
	h.updates[upd] = p
}

func (h Header) getUpdData(upd string) (ArtifactParser, error) {
	p, ok := h.updates[upd]
	if !ok {
		return nil, os.ErrNotExist
	}
	return p, nil
}

func (h *Header) addInfo(i *metadata.HeaderInfo) {
	h.hInfo = *i
}

// header-info must be the first file in the tar archive
func readHeaderInfo(tr *tar.Reader) (*metadata.HeaderInfo, error) {
	var hInfo metadata.HeaderInfo
	hdr, err := tr.Next()
	if err != nil {
		return nil, errors.New("artifacts reader: can not read header-info")
	}
	if strings.Compare(hdr.Name, "header-info") != 0 {
		return nil, errors.New("artifacts reader: element out of order")
	}

	if _, err := io.Copy(&hInfo, tr); err != nil {
		return nil, err
	}

	log.Infof("content of header-info: %v", hInfo.Updates)
	return &hInfo, nil
}

func (ar *ArtifactsReader) readHeader(r io.Reader) error {
	log.Info("Processing header")

	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrapf(err, "artifacts reader: error opening compressed header")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	hInfo, err := readHeaderInfo(tr)
	if err != nil {
		return err
	}
	ar.Header.addInfo(hInfo)

	for cnt, uType := range hInfo.Updates {
		p, err := ar.parsers.GetParser(uType.Type)
		if err != nil {
			return errors.Wrapf(err,
				"artifacts reader: can not find updater for update type: [%v]", uType.Type)
		}
		if err := p.ParseHeader(tr, filepath.Join("headers", fmt.Sprintf("%04d", cnt))); err != nil {
			return errors.Wrapf(err, "artifacts reader: error processing header: [%v]",
				uType.Type)
		}
		ar.Header.addUpdData(p, fmt.Sprintf("%04d", cnt))
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
				log.Infof("Procesing data: %v", hdr.Name)

				for cnt := range ar.Header.hInfo.Updates {
					updater, err := ar.Header.getUpdData(fmt.Sprintf("%04d", cnt))
					if err != nil {
						return errors.Wrapf(err,
							"artifacts reader: can not find header for data file: [%v]", cnt)
					}
					if err := updater.ParseData(tr); err != nil {
						return errors.Wrapf(err, "artifacts reader: error processing data: [%v]",
							cnt)
					}
				}
			}

		default:
			log.Errorf("unsupported element (%v)", hdr)
			return errors.New("artifacts reader: unsupported element in archive")
		}
	}
	return nil
}

func (ar *ArtifactsReader) readMenderV1Header() (*Header, error) {
	ar.initTarReader()

	if err := ar.readStream(ar.tar, header); err != nil {
		return nil, err
	}
	return ar.Header, nil
}

func (ar *ArtifactsReader) initTarReader() *tar.Reader {
	if ar.tar == nil {
		ar.tar = tar.NewReader(ar.artifact)
	}
	return ar.tar
}

func (ar *ArtifactsReader) StoreData(w io.Writer) error {
	ar.initTarReader()

	if err := ar.readStream(ar.tar, data); err != nil {
		return err
	}
	return nil
}

func (ar *ArtifactsReader) ReadHeader() (*Header, error) {
	return ar.readMenderV1Header()
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
	// for now we are only supporting mender v:1
	if strings.Compare(info.Format, "mender") != 0 || info.Version != 1 {
		return errors.New("artifacts reader: unsupported artifact format or version")
	}
	_, err = ar.readMenderV1Header()
	if err != nil {
		return err
	}

	if err := ar.StoreData(nil); err != nil {
		return err
	}

	return nil
}
