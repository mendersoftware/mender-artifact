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
	"strconv"
	"strings"
	"syscall"

	"github.com/mendersoftware/mender-artifact/metadata"
	"github.com/mendersoftware/mender-artifact/parser"
	"github.com/mendersoftware/mender-artifact/update"
	"github.com/pkg/errors"
)

type Reader struct {
	*parser.ParseManager

	CompatibleDevicesCallback func([]string) error

	tReader *tar.Reader
	signed  bool

	hInfo *metadata.HeaderInfo
	info  *metadata.Info

	// new
	r          io.Reader
	handlers   map[string]update.Installer
	installers map[int]update.Installer
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		r:            r,
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

func (ar *Reader) ReadHeader() error {
	hdr, err := getNext(ar.tReader)
	if err != nil {
		return errors.New("reader: error reading header")
	}
	if !strings.HasPrefix(hdr.Name, "header.tar.") {
		return errors.New("reader: invalid header name or elemet out of order")
	}

	// header MUST be compressed
	gz, err := gzip.NewReader(ar.tReader)
	if err != nil {
		return errors.Wrapf(err, "reader: error opening compressed header")
	}
	defer gz.Close()

	var tr *tar.Reader
	ch := NewReaderChecksum(gz)

	// If artifact is signed we need to calculate header checksum to be
	// able to validate it later.
	if ar.isSigned() {
		tr = tar.NewReader(ch)
	} else {
		tr = tar.NewReader(gz)
	}

	// first part of header must always be header-info
	hInfo := new(metadata.HeaderInfo)
	if err = readNext(tr, hInfo, "header-info"); err != nil {
		return err
	}
	// TODO:
	ar.hInfo = hInfo

	// after reading header-info we can check device compatibility
	if ar.CompatibleDevicesCallback != nil {
		if err = ar.CompatibleDevicesCallback(hInfo.CompatibleDevices); err != nil {
			return err
		}
	}

	// Next step is setting correct installers based on update types being
	// part of the artifact.
	if err = ar.setInstallers(hInfo.Updates); err != nil {
		return err
	}

	// At the end read rest of the header using correct installers.
	if err := ar.readHeader(tr); err != nil {
		return err
	}

	// calculate whole header checksum
	if ar.isSigned() {
		sum := ch.Checksum()
		// TODO:
		fmt.Printf("have sum: %s\n", sum)
	}

	return nil
}

func readVersion(tr *tar.Reader) (*metadata.Info, []byte, error) {
	info := new(metadata.Info)

	// read version file and calculate checksum
	sum, err := readNextWithChecksum(tr, info, "version")
	if err != nil {
		return nil, nil, err
	}
	return info, sum, nil
}

type Checksum struct {
	w io.Writer // underlying writer
	r io.Reader
	h hash.Hash
	c []byte // checksum
}

func NewWriterChecksum(w io.Writer) *Checksum {
	h := sha256.New()
	return &Checksum{
		w: io.MultiWriter(h, w),
		h: h,
	}
}

func NewReaderChecksum(r io.Reader) *Checksum {
	h := sha256.New()
	return &Checksum{
		r: io.TeeReader(r, h),
		h: h,
	}
}

func (c *Checksum) Write(p []byte) (int, error) {
	if c.w == nil {
		return 0, syscall.EBADF
	}
	return c.w.Write(p)
}

func (c *Checksum) Read(p []byte) (int, error) {
	if c.r == nil {
		return 0, syscall.EBADF
	}
	return c.r.Read(p)
}

func (c *Checksum) Checksum() []byte {
	sum := c.h.Sum(nil)
	checksum := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(checksum, c.h.Sum(nil))
	return checksum
}

func (ar *Reader) RegisterHandler(handler update.Installer) error {
	if _, ok := ar.handlers[handler.GetType()]; ok {
		return os.ErrExist
	}
	ar.handlers[handler.GetType()] = handler
	return nil
}

func (ar *Reader) ReadArtifact() error {
	// each artifact is tar archive
	ar.tReader = tar.NewReader(ar.r)

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
			fallthrough

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

func (ar *Reader) setInstallers(upd []metadata.UpdateType) error {
	for i, update := range upd {
		// firsrt check if we have worker for given update
		if w, ok := ar.installers[i]; ok {
			if w.GetType() == update.Type || w.GetType() == "generic" {
				continue
			}
			return errors.New("reader: invalid worker for given update type")
		}

		// if not just set installer for given update type
		if w, ok := ar.handlers[update.Type]; ok {
			ar.installers[i] = w.Copy()
			continue
		}

		// if nothing else worked set generic installer for given update
		//ar.installers[i] =

		// par, err := p.GetRegistered(update.Type)
		// if err != nil {
		// 	// if there is no registered one; check if we can use generic
		// 	par = p.GetGeneric(update.Type)
		// 	if par == nil {
		// 		return errors.Wrapf(err,
		// 			"reader: can not find parser for update type: [%v]", update.Type)
		// 	}
		// }
		// p.PushWorker(par, fmt.Sprintf("%04d", cnt))
	}
	return nil
}

// should be `headers/0000/file` format
func getUpdateNoFromHeaderPath(path string) (int, error) {
	split := strings.Split(path, string(os.PathSeparator))
	if len(split) < 3 {
		return 0, errors.New("can not get update order from tar path")
	}
	return strconv.Atoi(split[1])
}

func getUpdateNoFromDataPath(path string) (int, error) {
	no := strings.TrimSuffix(filepath.Base(path), ".tar.gz")
	return strconv.Atoi(no)
}

func (ar *Reader) readHeader(tr *tar.Reader) error {
	for {
		hdr, err := getNext(tr)

		if err == io.EOF {
			return nil
		} else if err != nil {
			return errors.Wrapf(err,
				"reader: can not read artifact header file: %v", hdr)
		}
		updNo, err := getUpdateNoFromHeaderPath(hdr.Name)
		if err != nil {
			return errors.Wrapf(err, "reader: error getting header update number")
		}

		inst, ok := ar.installers[updNo]
		if !ok {
			return errors.Errorf("reader: can not find parser for update: %v", hdr.Name)
		}
		return inst.SetFromHeader(tr, hdr.Name)
	}
}

func (ar *Reader) readNextDataFile(tr *tar.Reader) error {
	hdr, err := getNext(tr)
	if err == io.EOF {
		return io.EOF
	} else if err != nil {
		return errors.Wrapf(err, "reader: error reading update file: [%v]", hdr)
	}
	if filepath.Dir(hdr.Name) != "data" {
		return errors.New("reader: invalid data file name: " + hdr.Name)
	}
	updNo, err := getUpdateNoFromDataPath(hdr.Name)
	if err != nil {
		return errors.Wrapf(err, "reader: error getting data update number")
	}
	inst, ok := ar.installers[updNo]
	if !ok {
		return errors.Wrapf(err,
			"reader: can not find parser for parsing data file [%v]", hdr.Name)
	}

	return inst.Install(tr)
}

func (ar *Reader) ReadData() error {
	for {
		err := ar.readNextDataFile(ar.tReader)
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
