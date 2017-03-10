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
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendersoftware/mender-artifact/metadata"
	"github.com/mendersoftware/mender-artifact/parser"
	"github.com/pkg/errors"
)

type Reader struct {
	*parser.ParseManager

	CompatibleDevicesCallback func([]string) error

	tReader *tar.Reader
	signed  bool

	hInfo *metadata.HeaderInfo
	info  *metadata.Info
}

func NewReader() *Reader {
	return &Reader{
		ParseManager: parser.NewParseManager(),
	}
}

func (ar *Reader) isSigned() bool {
	return ar.signed
}

// TODO: implement me
func verifySignature(sig []byte) error {
	return nil
}

type checksums map[string]([]byte)

func readChecksums(r *tar.Reader) (checksums, []byte, error) {
	buf := bytes.NewBuffer(nil)

	sum, err := readNextWithChecksum(r, buf, "checksums")
	if err != nil {
		return nil, nil, err
	}

	// we should have at least version, checksums, header and data files
	chs := make(checksums, 4)

	s := bufio.NewScanner(buf)
	for s.Scan() {
		l := strings.Split(s.Text(), " ")
		if len(l) != 2 {
			return nil, nil, errors.New("")
		}
		// add element to map
		chs[l[0]] = []byte(l[1])
	}

	if err := s.Err(); err != nil {
		return nil, nil, err
	}
	return chs, sum, nil
}

//
// example implementation of checking device compatibility
//
// func checkDevice(devices []string) error {
// 	for _, dev := range devices {
// 		if dev == "beaglebone" {
// 			return nil
// 		}
// 	}
// 	return errors.New("artifact not compatible with device")
// }

func (ar *Reader) ReadHeader() error {
	hdr, err := getNext(ar.tReader)
	if err != nil {
		return errors.New("reader: error reading header")
	}
	if !strings.HasPrefix(hdr.Name, "header.tar.") {
		return errors.New("reader: invalid header name or elemet out of order")
	}

	// header MUST always be compressed
	gz, err := gzip.NewReader(ar.tReader)
	if err != nil {
		return errors.Wrapf(err, "reader: error opening compressed header")
	}
	defer gz.Close()

	var h hash.Hash
	var tr *tar.Reader

	// If artifact is signed we need to calculate header checksum to be
	// able to validate it later.
	if ar.isSigned() {
		h = sha256.New()
		// use tee reader to pass read data for checksum calculation
		teeReader := io.TeeReader(gz, h)
		tr = tar.NewReader(teeReader)
	} else {
		tr = tar.NewReader(gz)
	}

	// first part of header must always be header-info
	hInfo := new(metadata.HeaderInfo)
	if err = readNext(tr, hInfo, "header-info"); err != nil {
		return err
	}
	ar.hInfo = hInfo

	// after reading header-info we can check device compatibility
	if ar.CompatibleDevicesCallback != nil {
		if err = ar.CompatibleDevicesCallback(hInfo.CompatibleDevices); err != nil {
			return err
		}
	}

	// Next step is setting correct parsers based on update types being
	// part of the artifact.
	if err = setWorkers(ar.ParseManager, hInfo.Updates); err != nil {
		return err
	}

	// At the end read rest of the header using correct parsers.
	if err := readHeader(tr, ar.ParseManager); err != nil {
		return err
	}

	// calculate whole header checksum
	if ar.isSigned() {
		sum := h.Sum(nil)
		hSum := make([]byte, hex.EncodedLen(len(sum)))
		hex.Encode(hSum, h.Sum(nil))
	}

	return nil
}

func readVersion(r *tar.Reader) (*metadata.Info, []byte, error) {
	info := new(metadata.Info)

	// read version file and calculate checksum
	sum, err := readNextWithChecksum(r, info, "version")
	if err != nil {
		return nil, nil, err
	}
	return info, sum, nil
}

func (ar *Reader) Read(r io.Reader) error {
	ar.tReader = tar.NewReader(r)

	// first file inside the artifact MUST be version
	ver, _, err := readVersion(ar.tReader)
	if err != nil {
		return err
	}
	ar.info = ver

	switch ver.Version {
	case 1:
		if err = ar.ReadHeader(); err != nil {
			return err
		}

	case 2:
		// first file after version MUST contains all the checksums
		//chcksums, sum, err := readChecksums(ar.tReader)
		_, _, err := readChecksums(ar.tReader)
		if err != nil {
			return err
		}

		// check what is the next file in the artifact
		// depending if artifact is signed or not we can have header or signature
		hdr, err := getNext(ar.tReader)
		if err != nil {
			return errors.Wrapf(err, "reader: error reading file after checksums")
		}

		// check if artifact is signed
		fName := hdr.FileInfo().Name()
		switch {
		case strings.HasPrefix(fName, "signature"):
			ar.signed = true

			// first read signature...
			sig := bytes.NewBuffer(nil)
			if err = readNext(ar.tReader, sig, "signature"); err != nil {
				return errors.Wrapf(err, "reader: can not read signature file")
			}

			// here we can varify signature and the checksum of header
			if err = verifySignature(sig.Bytes()); err != nil {
				return err
			}

			// verify checksums of the checksums file and the header

			// ...and then header
			if err = ar.ReadHeader(); err != nil {
				return err
			}

		case strings.HasPrefix(fName, "header.tar."):
			if err = ar.ReadHeader(); err != nil {
				return err
			}

		default:
			return errors.Errorf("reader: found unexpected file: %v", fName)
		}

	default:
		return errors.Errorf("reader: unsupported version: %d", ver.Version)
	}

	if err := ar.ReadData(); err != nil {
		return err
	}

	return nil
}

func (ar *Reader) GetCompatibleDevices() []string {
	return ar.hInfo.CompatibleDevices
}

func (ar *Reader) GetArtifactName() string {
	return ar.hInfo.ArtifactName
}

func (ar *Reader) GetInfo() metadata.Info {
	return *ar.info
}

func setWorkers(p *parser.ParseManager, u []metadata.UpdateType) error {
	for cnt, update := range u {
		// firsrt check if we have worker for given update
		w, err := p.GetWorker(fmt.Sprintf("%04d", cnt))

		if err == nil {
			if w.GetUpdateType().Type == update.Type || w.GetUpdateType().Type == "generic" {
				continue
			}

			return errors.New("reader: wrong worker for given update type")
		}
		// if not just register worker for given update type
		par, err := p.GetRegistered(update.Type)
		if err != nil {
			// if there is no registered one; check if we can use generic
			par = p.GetGeneric(update.Type)
			if par == nil {
				return errors.Wrapf(err,
					"reader: can not find parser for update type: [%v]", update.Type)
			}
		}
		p.PushWorker(par, fmt.Sprintf("%04d", cnt))
	}
	return nil
}

// should be `headers/0000/file` format
func getUpdateFromHdr(hdr string) string {
	r := strings.Split(hdr, string(os.PathSeparator))
	if len(r) < 2 {
		return ""
	}
	return r[1]
}

func processNextHeader(tr *tar.Reader, p *parser.ParseManager,
	upd string, hdr *tar.Header) error {

	par, err := p.GetWorker(upd)
	if err != nil {
		return errors.Wrapf(err, "reader: can not find parser for update: %v", upd)
	}

	// TODO: refactor ParseHeader
	if err := par.ParseHeader(tr, hdr, filepath.Join("headers", upd)); err != nil {
		return err
	}
	return nil
}

func readHeader(r *tar.Reader, p *parser.ParseManager) error {
	for {
		hdr, err := getNext(r)

		if err == io.EOF {
			return nil
		} else if err != nil {
			return errors.Wrapf(err,
				"reader: can not read artifact header file: %v", hdr)
		}
		if err := processNextHeader(r, p, getUpdateFromHdr(hdr.Name), hdr); err != nil {
			return err
		}
	}
}

func getDataFileUpdate(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".tar.gz")
}

func readNextDataFile(r *tar.Reader, p *parser.ParseManager) error {
	hdr, err := getNext(r)
	if err == io.EOF {
		return io.EOF
	} else if err != nil {
		return errors.Wrapf(err, "reader: error reading update file: [%v]", hdr)
	}
	if filepath.Dir(hdr.Name) != "data" {
		return errors.New("reader: invalid data file name: " + hdr.Name)
	}
	par, err := p.GetWorker(getDataFileUpdate(hdr.Name))
	if err != nil {
		return errors.Wrapf(err,
			"reader: can not find parser for parsing data file [%v]", hdr.Name)
	}
	err = par.ParseData(r)
	if err != nil {
		return err
	}
	return nil
}

func (ar *Reader) ReadData() error {
	for {
		err := readNextDataFile(ar.tReader, ar.ParseManager)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	return nil
}

func readNext(tr *tar.Reader, w io.Writer, elem string) error {
	_, err := readNextElem(tr, w, elem, false)
	return err
}

func readNextWithChecksum(tr *tar.Reader, w io.Writer,
	elem string) ([]byte, error) {
	return readNextElem(tr, w, elem, true)
}

func readNextElem(tr *tar.Reader, w io.Writer, elem string,
	getSum bool) ([]byte, error) {
	if tr == nil {
		return nil, errors.New("reader: read next called on invalid stream")
	}
	hdr, err := getNext(tr)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(hdr.Name, elem) {
		var h hash.Hash
		if getSum {
			h = sha256.New()
			// use tee reader to pass read data for checksum calculation
			teer := io.TeeReader(tr, h)
			_, err = io.Copy(w, teer)
		} else {
			_, err = io.Copy(w, tr)
		}

		if err != nil {
			return nil, errors.Wrapf(err, "reader: error reading")
		}

		if getSum {
			sum := h.Sum(nil)
			hSum := make([]byte, hex.EncodedLen(len(sum)))
			hex.Encode(hSum, h.Sum(nil))
			return hSum, nil
		}
		return nil, nil
	}
	return nil, os.ErrInvalid
}

func getNext(tr *tar.Reader) (*tar.Header, error) {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we've reached end of archive
			return hdr, err
		} else if err != nil {
			return nil, errors.Wrapf(err, "reader: error reading archive")
		}
		return hdr, nil
	}
}
