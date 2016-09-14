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

package areader

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
	"github.com/pkg/errors"
)

type Reader struct {
	r io.Reader
	*parser.ParseManager

	info    metadata.Info
	tReader *tar.Reader
	*headerReader
}

type headerReader struct {
	hInfo metadata.HeaderInfo

	hReader     *tar.Reader
	hGzipReader *gzip.Reader
}

func NewReader(r io.Reader) *Reader {
	ar := Reader{
		r:            r,
		ParseManager: parser.NewParseManager(),
		headerReader: &headerReader{},
	}
	//TODO:
	//ar.Register(parser, parsingType)
	return &ar
}

func (ar *Reader) Read() error {
	if _, err := ar.GetUpdates(); err != nil {
		return err
	}
	if _, err := ar.ReadHeader(); err != nil {
		return err
	}
	if err := ar.ReadUpdates(); err != nil {
		return err
	}
	return nil
}

func (ar *Reader) Close() error {
	if ar.hGzipReader != nil {
		return ar.hGzipReader.Close()
	}
	return nil
}

func (ar *Reader) getTarReader() *tar.Reader {
	if ar.tReader == nil {
		ar.tReader = tar.NewReader(ar.r)
	}
	return ar.tReader
}

// This reads next element in main artifact tar structure.
// In v1 there are only info, header and data files available.
func (ar *Reader) readNext(w io.Writer, elem string) error {
	tr := ar.getTarReader()
	return readNext(tr, w, elem)
}

func (ar *Reader) getNext() (io.Reader, *tar.Header, error) {
	tr := ar.getTarReader()
	return getNext(tr)
}

func (ar *Reader) initHeaderReading() error {
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

func (ar *Reader) readInfo() (*metadata.Info, error) {
	info := new(metadata.Info)
	err := ar.readNext(info, "info")
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (ar *Reader) GetUpdates() (parser.Workers, error) {
	info, err := ar.readInfo()
	if err != nil {
		return nil, err
	}
	// so far we are supporing only v1
	switch info.Version {
	case 1:
		// we know that in v1 header goes just after info
		err := ar.initHeaderReading()
		if err != nil {
			return nil, err
		}
		for cnt, update := range ar.hInfo.Updates {
			p, err := ar.ParseManager.GetRegistered(update.Type)
			if err != nil {
				p = ar.ParseManager.GetGeneric()
				if p == nil {
					return nil, errors.Wrapf(err,
						"reader: can not find parser for update type: [%v]", update.Type)
				}
			}
			ar.ParseManager.PushWorker(p, fmt.Sprintf("%04d", cnt))
		}
		return ar.ParseManager.GetWorkers(), nil
	default:
		return nil, errors.New("reader: unsupported artifact version")
	}
}

//TODO
//func getUpdateFromHeaderName()

func (ar *Reader) ReadHeader() (parser.Workers, error) {
	r := ar.hReader
	for {
		// TODO: make sure we are reading first header file
		p, err := ar.ParseManager.GetWorker("0000")
		if err != nil {
			return nil, errors.Wrapf(err, "reader: can not find parser for update: %v", "0000")
		}
		if err = p.ParseHeader(r,
			filepath.Join("headers", fmt.Sprintf("%04d", 0))); err != nil {
			return nil, err
		}

		r, _, err = getNext(ar.hReader)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
	}

	// at the end close gzip
	if ar.hGzipReader != nil {
		if err := ar.hGzipReader.Close(); err != nil {
			return nil, errors.Wrapf(err, "reader: error closing gzip reader")
		}
	}
	return ar.ParseManager.GetWorkers(), nil
}

func getDataFileUpdate(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".tar.gz")
}

func (ar *Reader) ReadUpdates() error {
	for {
		r, hdr, err := ar.getNext()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "reader: error reading update file: "+hdr.Name)
		}
		if strings.Compare(filepath.Dir(hdr.Name), "data") != 0 {
			return errors.New("reader: invalid data file name: " + hdr.Name)
		}
		p, err := ar.ParseManager.GetWorker(getDataFileUpdate(hdr.Name))
		if err != nil {
			return errors.Wrapf(err,
				"reader: can not find parser for parsing data file [%v]", hdr.Name)
		}
		p.ParseData(r)

	}
	return nil
}

func readNext(tr *tar.Reader, w io.Writer, elem string) error {
	if tr == nil {
		return errors.New("reader: read next called on invalid stream")
	}
	r, hdr, err := getNext(tr)
	if err != nil {
		return err
	}
	if strings.HasPrefix(hdr.Name, elem) {
		_, err = io.Copy(w, r)
		if err != nil {
			return errors.Wrapf(err, "reader: error reading")
		}
		return nil
	}
	return os.ErrInvalid
}

func getNext(tr *tar.Reader) (*tar.Reader, *tar.Header, error) {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we've reached end of archive
			return nil, hdr, err
		} else if err != nil {
			return nil, nil, errors.New("reader: error reading archive")
		}
		return tr, hdr, nil
	}
}
