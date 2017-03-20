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
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
)

type SignatureVerifyFn func(message, sig []byte) error
type DevicesCompatibleFn func([]string) error

type Reader struct {
	CompatibleDevicesCallback DevicesCompatibleFn
	VerifySignatureCallback   SignatureVerifyFn

	signed     bool
	hInfo      *artifact.HeaderInfo
	info       *artifact.Info
	r          io.Reader
	handlers   map[string]artifact.Installer
	installers map[int]artifact.Installer
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		r:          r,
		handlers:   make(map[string]artifact.Installer, 1),
		installers: make(map[int]artifact.Installer, 1),
	}
}

func (ar *Reader) isSigned() bool {
	return ar.signed
}

func (ar *Reader) readHeader(tReader io.Reader) ([]byte, error) {

	var r io.Reader
	if ar.isSigned() {
		// If artifact is signed we need to calculate header checksum to be
		// able to validate it later.
		r = artifact.NewReaderChecksum(tReader)
	} else {
		r = tReader
	}
	// header MUST be compressed
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, errors.Wrapf(err, "reader: error opening compressed header")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	// first part of header must always be header-info
	hInfo := new(artifact.HeaderInfo)
	if err = readNext(tr, hInfo, "header-info"); err != nil {
		return nil, err
	}
	ar.hInfo = hInfo

	// after reading header-info we can check device compatibility
	if ar.CompatibleDevicesCallback != nil {
		if err = ar.CompatibleDevicesCallback(hInfo.CompatibleDevices); err != nil {
			return nil, err
		}
	}

	// Next step is setting correct installers based on update types being
	// part of the artifact.
	if err = ar.setInstallers(hInfo.Updates); err != nil {
		return nil, err
	}

	// At the end read rest of the header using correct installers.
	if err := ar.readHeaderUpdate(tr); err != nil {
		return nil, err
	}

	// calculate whole header checksum
	if ar.isSigned() {
		sum := r.(*artifact.Checksum).Checksum()
		return sum, nil
	}
	return nil, nil
}

func readVersion(tr *tar.Reader) (*artifact.Info, []byte, error) {
	info := new(artifact.Info)

	// read version file and calculate checksum
	sum, err := readNextWithChecksum(tr, info, "version")
	if err != nil {
		return nil, nil, err
	}
	return info, sum, nil
}

func (ar *Reader) RegisterHandler(handler artifact.Installer) error {
	if handler == nil {
		return errors.New("reader: invalid handler")
	}
	if _, ok := ar.handlers[handler.GetType()]; ok {
		return os.ErrExist
	}
	ar.handlers[handler.GetType()] = handler
	return nil
}

func (ar *Reader) GetHandlers() map[int]artifact.Installer {
	return ar.installers
}

func (ar *Reader) readHeaderV1(tReader *tar.Reader) error {
	hdr, err := getNext(tReader)
	if err != nil {
		return errors.New("reader: error reading header")
	}
	if !strings.HasPrefix(hdr.Name, "header.tar.") {
		return errors.Errorf("reader: invalid header element: %v", hdr.Name)
	}

	if _, err = ar.readHeader(tReader); err != nil {
		return err
	}
	return nil
}

func readManifest(tReader *tar.Reader) (*artifact.ChecksumStore, error) {
	buf := bytes.NewBuffer(nil)
	if err := readNext(tReader, buf, "manifest"); err != nil {
		return nil, errors.Wrap(err, "reader: can not buffer manifest")
	}
	manifest := artifact.NewChecksumStore()
	if err := manifest.ReadRaw(buf.Bytes()); err != nil {
		return nil, errors.Wrap(err, "reader: can not read manifest")
	}
	return manifest, nil
}

func signartureReadAndVerify(tReader *tar.Reader, message []byte,
	verify SignatureVerifyFn) error {
	// first read signature...
	sig := bytes.NewBuffer(nil)
	if _, err := io.Copy(sig, tReader); err != nil {
		return errors.Wrapf(err, "reader: can not read signature file")
	}

	// verify signature
	if verify == nil {
		return errors.New("reader: verify signature callback not registered")
	} else if err := verify(message, sig.Bytes()); err != nil {
		return errors.Wrap(err, "reader: invalid signature")
	}
	return nil
}

func verifyVersion(vsum []byte, manifest *artifact.ChecksumStore) error {
	vc, err := manifest.Get("version")
	if err != nil {
		return err
	}
	if bytes.Compare(vc, vsum) != 0 {
		return errors.Errorf("reader: invalid 'version' checksum [%s]:[%s]",
			vc, vsum)
	}
	return nil
}

func (ar *Reader) readHeaderV2(tReader *tar.Reader,
	vsum []byte) (*artifact.ChecksumStore, error) {
	// first file after version MUST contain all the checksums
	manifest, err := readManifest(tReader)
	if err != nil {
		return nil, err
	}

	// check what is the next file in the artifact
	// depending if artifact is signed or not we can have
	// either header or signature file
	hdr, err := getNext(tReader)
	if err != nil {
		return nil, errors.Wrapf(err, "reader: error reading file after checksums")
	}

	switch hdr.FileInfo().Name() {
	case "manifest.sig":
		// firs read and verify signature
		ar.signed = true
		if err := signartureReadAndVerify(tReader, manifest.GetRaw(),
			ar.VerifySignatureCallback); err != nil {
			return nil, err
		}
		// verify checksums of version
		if err := verifyVersion(vsum, manifest); err != nil {
			return nil, err
		}

		// ...and then header
		hdr, err := getNext(tReader)
		if err != nil {
			return nil, errors.New("reader: error reading header")
		}
		if !strings.HasPrefix(hdr.Name, "header.tar.gz") {
			return nil, errors.Errorf("reader: invalid header element: %v", hdr.Name)
		}
		fallthrough

	case "header.tar.gz":
		hSum, err := ar.readHeader(tReader)
		if err != nil {
			return nil, err
		}
		if hSum != nil {
			// verify checksums of header
			hc, err := manifest.Get("header.tar.gz")

			if err != nil {
				return nil, err
			}
			if bytes.Compare(hc, hSum) != 0 {
				return nil, errors.Errorf("reader: invalid header checksum [%s]:[%s]",
					hc, hSum)
			}
		}
	default:
		return nil, errors.Errorf("reader: found unexpected file in artifact: %v",
			hdr.FileInfo().Name())
	}
	return manifest, nil
}

func (ar *Reader) ReadArtifact() error {
	// each artifact is tar archive
	tReader := tar.NewReader(ar.r)

	// first file inside the artifact MUST be version
	ver, vsum, err := readVersion(tReader)
	if err != nil {
		return err
	}
	ar.info = ver

	var s *artifact.ChecksumStore

	switch ver.Version {
	case 1:
		if err = ar.readHeaderV1(tReader); err != nil {
			return err
		}
	case 2:
		s, err = ar.readHeaderV2(tReader, vsum)
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("reader: unsupported version: %d", ver.Version)
	}
	return ar.readData(tReader, s)
}

func (ar *Reader) GetCompatibleDevices() []string {
	return ar.hInfo.CompatibleDevices
}

func (ar *Reader) GetArtifactName() string {
	return ar.hInfo.ArtifactName
}

func (ar *Reader) GetInfo() artifact.Info {
	return *ar.info
}

func (ar *Reader) setInstallers(upd []artifact.UpdateType) error {
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
		ar.installers[i] = handlers.NewGeneric(update.Type)
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

// should be 0000.tar.gz
func getUpdateNoFromDataPath(path string) (int, error) {
	no := strings.TrimSuffix(filepath.Base(path), ".tar.gz")
	return strconv.Atoi(no)
}

func (ar *Reader) readHeaderUpdate(tr *tar.Reader) error {
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
		if err := inst.ReadHeader(tr, hdr.Name); err != nil {
			return errors.Wrap(err, "reader: can not read header")
		}
	}
}

func (ar *Reader) readNextDataFile(tr *tar.Reader,
	manifest *artifact.ChecksumStore) error {
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
	return artifact.ReadAndInstall(tr, inst, manifest, updNo)
}

func (ar *Reader) readData(tr *tar.Reader, manifest *artifact.ChecksumStore) error {
	for {
		err := ar.readNextDataFile(tr, manifest)
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
		var ch io.Reader
		if getSum {
			ch = artifact.NewReaderChecksum(tr)
		} else {
			ch = tr
		}

		if _, err := io.Copy(w, ch); err != nil {
			return nil, errors.Wrapf(err, "reader: error reading")
		}

		if getSum {
			return ch.(*artifact.Checksum).Checksum(), nil
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
