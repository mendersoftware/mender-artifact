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

type Reader struct {
	CompatibleDevicesCallback func([]string) error
	VerifySignatureCallback   func(message, sig []byte) error

	tReader *tar.Reader
	signed  bool

	hInfo *artifact.HeaderInfo
	info  *artifact.Info

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

func (ar *Reader) ReadHeader() ([]byte, error) {

	var r io.Reader
	if ar.isSigned() {
		// If artifact is signed we need to calculate header checksum to be
		// able to validate it later.
		r = artifact.NewReaderChecksum(ar.tReader)
	} else {
		r = ar.tReader
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
	// TODO:
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
	if err := ar.readHeader(tr); err != nil {
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
	if _, ok := ar.handlers[handler.GetType()]; ok {
		return os.ErrExist
	}
	ar.handlers[handler.GetType()] = handler
	return nil
}

func (ar *Reader) GetInstallers() map[int]artifact.Installer {
	return ar.installers
}

func (ar *Reader) ReadArtifact() error {
	// each artifact is tar archive
	// TODO: do we need ar.tReader
	ar.tReader = tar.NewReader(ar.r)

	// first file inside the artifact MUST be version
	ver, vsum, err := readVersion(ar.tReader)
	if err != nil {
		return err
	}
	ar.info = ver

	var manifest *artifact.Manifest

	switch ver.Version {
	case 1:
		hdr, err := getNext(ar.tReader)
		if err != nil {
			return errors.New("reader: error reading header")
		}
		if !strings.HasPrefix(hdr.Name, "header.tar.") {
			return errors.Errorf("reader: invalid header element: %v", hdr.Name)
		}

		if _, err = ar.ReadHeader(); err != nil {
			return err
		}

	case 2:
		// first file after version MUST contains all the checksums
		hdr, err := getNext(ar.tReader)
		if err != nil || hdr.Name != "manifest" {
			return errors.Wrapf(err, "reader: error reading manifest header")
		}

		manifestBuf := bytes.NewBuffer(nil)
		manifest = artifact.NewReaderManifest(manifestBuf)

		if _, err = io.Copy(manifestBuf, ar.tReader); err != nil {
			return errors.Wrap(err, "reader: can not buffer manifest")
		}
		if err = manifest.ReadAll(); err != nil {
			return errors.Wrap(err, "reader: can not read manifest")
		}

		// check what is the next file in the artifact
		// depending if artifact is signed or not we can have header or signature
		hdr, err = getNext(ar.tReader)
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
			if _, err = io.Copy(sig, ar.tReader); err != nil {
				return errors.Wrapf(err, "reader: can not read signature file")
			}

			// verify signature
			if ar.VerifySignatureCallback == nil {
				return errors.New("reader: verify signature callback not registered")
			} else if err = ar.VerifySignatureCallback(manifestBuf.Bytes(), sig.Bytes()); err != nil {
				return errors.Wrap(err, "reader: invalid signature")
			}

			// verify checksums of version
			vc, err := manifest.GetChecksum("version")
			if err != nil {
				return err
			}
			if bytes.Compare(vc, vsum) != 0 {
				return errors.Errorf("reader: invalid 'version' checksum [%s]:[%s]",
					vc, vsum)
			}

			// ...and then header
			hdr, err := getNext(ar.tReader)
			if err != nil {
				return errors.New("reader: error reading header")
			}
			if !strings.HasPrefix(hdr.Name, "header.tar.") {
				return errors.Errorf("reader: invalid header element: %v", hdr.Name)
			}
			fallthrough

		case strings.HasPrefix(fName, "header.tar."):
			hSum, err := ar.ReadHeader()
			if err != nil {
				return err
			}
			if hSum != nil {
				// verify checksums of header
				hc, err := manifest.GetChecksum("header.tar.gz")
				if err != nil {
					return err
				}
				if bytes.Compare(hc, hSum) != 0 {
					return errors.Errorf("reader: invalid 'version' checksum [%s]:[%s]",
						hc, hSum)
				}
			}

		default:
			return errors.Errorf("reader: found unexpected file: %v", fName)
		}

	default:
		return errors.Errorf("reader: unsupported version: %d", ver.Version)
	}

	if err := ar.ReadData(manifest); err != nil {
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
		if err := inst.SetFromHeader(tr, hdr.Name); err != nil {
			return errors.Wrap(err, "reader: can not read header")
		}
	}
}

func (ar *Reader) readNextDataFile(tr *tar.Reader,
	manifest *artifact.Manifest) error {
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

func (ar *Reader) ReadData(manifest *artifact.Manifest) error {
	for {
		err := ar.readNextDataFile(ar.tReader, manifest)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	return nil
}

// TODO: refactor
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
