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
	tReader  *tar.Reader
	*Parsers

	info metadata.Info
	*header
}

type header struct {
	hInfo  metadata.HeaderInfo
	pStack []ArtifactParser

	hReader     *tar.Reader
	hGzipReader *gzip.Reader
}

func NewArtifactsReader(r io.ReadCloser) *ArtifactsReader {
	return &ArtifactsReader{
		artifact: r,
		Parsers:  NewParserFactory(),
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

func (ar *ArtifactsReader) pushParser(p ArtifactParser, num string) {
	p.SetOrder(num)
	ar.pStack = append(ar.pStack, p)
}

func (ar *ArtifactsReader) GetUpdates() ([]ArtifactParser, error) {
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
			ar.pushParser(p, fmt.Sprintf("%04d", cnt))
		}
		return ar.pStack, nil
	default:
		return nil, errors.New("reader: unsupported artifact version")
	}
}

func (ar *ArtifactsReader) ReadHeader() ([]ArtifactParser, error) {
	for cnt, p := range ar.pStack {
		if err := p.ParseHeader(ar.hReader, filepath.Join("headers", fmt.Sprintf("%04d", cnt))); err != nil {
			return nil, err
		}
	}
	// at the end close gzip
	ar.hGzipReader.Close()

	return ar.pStack, nil
}

func (ar *ArtifactsReader) ProcessUpdateFiles() error {
	for _, p := range ar.pStack {
		// some updates won't contain data files
		if p.NeedsDataFile() {
			r, hdr, err := ar.getNext()
			if err != nil {
				return err
			}
			log.Infof("processing data file: %v, %v", hdr.Name, withoutExt(hdr.Name))

			if strings.Compare(filepath.Dir(hdr.Name), "data") != 0 ||
				!strings.HasPrefix(filepath.Base(hdr.Name), p.GetOrder()) {
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
	r, hdr, err := getNext(tr)
	if err != nil {
		return err
	}
	if strings.HasPrefix(hdr.Name, elem) {
		_, err = io.Copy(w, r)
		return err
	}
	return os.ErrInvalid
}

func getNext(tr *tar.Reader) (io.Reader, *tar.Header, error) {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we've reached end of archive
			return nil, hdr, err
		}
		log.Infof("reader: processing file: %v", hdr.Name)
		return tr, hdr, nil
	}
}
