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
	"github.com/mendersoftware/artifacts/parser"
	"github.com/mendersoftware/log"
	"github.com/pkg/errors"
)

type ArtifactsReader struct {
	artifact io.ReadCloser
	tReader  *tar.Reader
	*parser.Parsers

	info metadata.Info
	*header
}

type header struct {
	hInfo metadata.HeaderInfo

	hReader     *tar.Reader
	hGzipReader *gzip.Reader
}

func NewArtifactsReader(r io.ReadCloser) *ArtifactsReader {
	return &ArtifactsReader{
		artifact: r,
		Parsers:  parser.NewParserFactory(),
		header:   &header{},
	}
}

func (ar *ArtifactsReader) Read() error {
	if _, err := ar.GetUpdates(); err != nil {
		return err
	}
	if _, err := ar.ReadHeader(); err != nil {
		return err
	}
	if err := ar.ProcessUpdateFiles(); err != nil {
		return err
	}
	return nil
}

func (ar *ArtifactsReader) Close() error {
	if ar.hGzipReader != nil {
		ar.hGzipReader.Close()
	}
	return nil
}

func (ar *ArtifactsReader) getReader() *tar.Reader {
	if ar.tReader == nil {
		ar.tReader = tar.NewReader(ar.artifact)
	}
	return ar.tReader
}

// This reads next element in main artifact tar structure.
// In v1 there are only info, header and data files available.
func (ar *ArtifactsReader) readNext(w io.Writer, elem string) error {
	tr := ar.getReader()
	return readNext(tr, w, elem)
}

func (ar *ArtifactsReader) getNext() (io.Reader, *tar.Header, error) {
	tr := ar.getReader()
	return getNext(tr)
}

// header-info must be the first file in the tar archive
func (ar *ArtifactsReader) initHeaderForReading() error {
	r, hdr, err := ar.getNext()
	if err != nil {
		return errors.New("reader: error initializing header")
	}
	if !strings.HasPrefix(hdr.Name, "header.tar.") {
		return errors.New("reader: invalid header name or elemet out of order")
	}

	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrapf(err, "reader: error opening compressed header")
	}
	ar.hGzipReader = gz
	tr := tar.NewReader(gz)
	ar.hReader = tr

	hInfo := new(metadata.HeaderInfo)
	if err := readNext(tr, hInfo, "header-info"); err != nil {
		return err
	}
	ar.hInfo = *hInfo
	return nil
}

func (ar *ArtifactsReader) readInfo() (*metadata.Info, error) {
	info := new(metadata.Info)
	err := ar.readNext(info, "info")
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (ar *ArtifactsReader) GetUpdates() (*parser.Parsers, error) {
	ar.Parsers.Reset()

	info, err := ar.readInfo()
	if err != nil {
		return nil, err
	}
	// so far we are supporing only v1
	switch info.Version {
	case 1:
		// we know that in v1 header goes just after info
		err := ar.initHeaderForReading()
		if err != nil {
			return nil, err
		}
		log.Infof("parsed header info: %v", ar.hInfo)
		for cnt, update := range ar.hInfo.Updates {
			p, err := ar.GetParser(update.Type)
			if err != nil {
				return nil, errors.Wrapf(err,
					"reader: can not find parser for update type: [%v]", update.Type)
			}
			ar.PushParser(p, fmt.Sprintf("%04d", cnt))
		}
		return ar.Parsers, nil
	default:
		return nil, errors.New("reader: unsupported artifact version")
	}
}

func (ar *ArtifactsReader) ReadHeader() (*parser.Parsers, error) {
	ar.Parsers.Reset()

	for cnt := 0; ; cnt++ {
		p, _, err := ar.Parsers.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if err := p.ParseHeader(ar.hReader,
			filepath.Join("headers", fmt.Sprintf("%04d", cnt))); err != nil {
			return nil, err
		}
	}
	// at the end close gzip
	if ar.hGzipReader != nil {
		if err := ar.hGzipReader.Close(); err != nil {
			return nil, errors.Wrapf(err, "reader: error closing gzip reader")
		}
	}
	return ar.Parsers, nil
}

func (ar *ArtifactsReader) ProcessUpdateFiles() error {
	ar.Parsers.Reset()

	for cnt := 0; ; cnt++ {
		p, upd, err := ar.Parsers.Next()
		log.Debug("parsing update: %d [%+v] %v", cnt, p, upd)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if p.NeedsDataFile() {
			r, hdr, err := ar.getNext()
			log.Infof("parsing update file: %v [%+v] %v", r, hdr, err)
			if err != nil {
				return errors.Wrapf(err, "reader: no data file to parse")
			}

			if strings.Compare(filepath.Dir(hdr.Name), "data") != 0 ||
				!strings.HasPrefix(filepath.Base(hdr.Name), upd) {
				return errors.New("reader: invalid data file name or elemet out of order")
			}
			p.ParseData(r)
		} else {
			p.ParseData(nil)
		}
	}
	return nil
}

func readNext(tr *tar.Reader, w io.Writer, elem string) error {
	if tr == nil {
		return errors.New("reader: red next called on invalid stream")
	}
	r, hdr, err := getNext(tr)
	if err != nil {
		return err
	}
	if strings.HasPrefix(hdr.Name, elem) {
		_, err = io.Copy(w, r)
		if err != nil {
			return errors.Wrapf(err, "reader: read next error")
		}
		return nil
	}
	return os.ErrInvalid
}

func getNext(tr *tar.Reader) (io.Reader, *tar.Header, error) {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we've reached end of archive
			return nil, hdr, err
		} else if err != nil {
			return nil, nil, errors.New("reader: error reading archive")
		}
		log.Infof("reader: processing file: %v", hdr.Name)
		return tr, hdr, nil
	}
}
