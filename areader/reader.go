// Copyright 2019 Northern.tech AS
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
	"fmt"
	"io"
	"io/ioutil"
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
type ScriptsReadFn func(io.Reader, os.FileInfo) error

type Reader struct {
	CompatibleDevicesCallback DevicesCompatibleFn
	ScriptsReadCallback       ScriptsReadFn
	VerifySignatureCallback   SignatureVerifyFn
	IsSigned                  bool
	ForbidUnknownHandlers     bool

	shouldBeSigned bool
	hInfo          artifact.HeaderInfoer
	augmentedhInfo artifact.HeaderInfoer
	info           *artifact.Info
	r              io.Reader
	files          []handlers.DataFile
	augmentFiles   []handlers.DataFile
	handlers       map[string]handlers.Installer
	installers     map[int]handlers.Installer
	updateStorers  map[int]handlers.UpdateStorer
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		r:             r,
		handlers:      make(map[string]handlers.Installer, 1),
		installers:    make(map[int]handlers.Installer, 1),
		updateStorers: make(map[int]handlers.UpdateStorer),
	}
}

func NewReaderSigned(r io.Reader) *Reader {
	return &Reader{
		r:              r,
		shouldBeSigned: true,
		handlers:       make(map[string]handlers.Installer, 1),
		installers:     make(map[int]handlers.Installer, 1),
		updateStorers:  make(map[int]handlers.UpdateStorer),
	}
}

func getReader(tReader io.Reader, headerSum []byte) io.Reader {

	if headerSum != nil {
		// If artifact is signed we need to calculate header checksum to be
		// able to validate it later.
		return artifact.NewReaderChecksum(tReader, headerSum)
	}
	return tReader
}

func readStateScripts(tr *tar.Reader, header *tar.Header, cb ScriptsReadFn) error {

	for {
		hdr, err := getNext(tr)
		if err == io.EOF {
			return nil
		} else if err != nil {
			return errors.Wrapf(err,
				"reader: error reading artifact header file: %v", hdr)
		}
		if filepath.Dir(hdr.Name) == "scripts" {
			if cb != nil {
				if err = cb(tr, hdr.FileInfo()); err != nil {
					return err
				}
			}
		} else {
			// if there are no more scripts to read leave the loop
			*header = *hdr
			break
		}
	}

	return nil
}

func (ar *Reader) readHeader(tReader io.Reader, headerSum []byte) error {

	r := getReader(tReader, headerSum)
	// header MUST be compressed
	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrapf(err, "readHeader: error opening compressed header")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	// Populate the artifact info fields.
	if err = ar.populateArtifactInfo(ar.info.Version, tr); err != nil {
		return errors.Wrap(err, "readHeader")
	}
	// after reading header-info we can check device compatibility
	if ar.CompatibleDevicesCallback != nil {
		if err = ar.CompatibleDevicesCallback(ar.GetCompatibleDevices()); err != nil {
			return err
		}
	}

	var hdr tar.Header

	// Next we need to read and process state scripts.
	if err = readStateScripts(tr, &hdr, ar.ScriptsReadCallback); err != nil {
		return err
	}

	// Next step is setting correct installers based on update types being
	// part of the artifact.
	if err = ar.setInstallers(ar.GetUpdates(), false); err != nil {
		return err
	}

	// At the end read rest of the header using correct installers.
	if err = ar.readHeaderUpdate(tr, &hdr, false); err != nil {
		return err
	}

	// Check if header checksum is correct.
	if cr, ok := r.(*artifact.Checksum); ok {
		if err = cr.Verify(); err != nil {
			return errors.Wrap(err, "reader: reading header error")
		}
	}

	return nil
}

func (ar *Reader) populateArtifactInfo(version int, tr *tar.Reader) error {
	var hInfo artifact.HeaderInfoer
	switch version {
	case 1, 2:
		hInfo = new(artifact.HeaderInfo)
	case 3:
		hInfo = new(artifact.HeaderInfoV3)
	}
	// first part of header must always be header-info
	if err := readNext(tr, hInfo, "header-info"); err != nil {
		return err
	}
	ar.hInfo = hInfo
	return nil
}

func (ar *Reader) readAugmentedHeader(tReader io.Reader, headerSum []byte) error {
	r := getReader(tReader, headerSum)
	// header MUST be compressed
	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrapf(err, "reader: error opening compressed header")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	// first part of header must always be header-info
	hInfo := new(artifact.HeaderInfoV3)
	err = readNext(tr, hInfo, "header-info")
	if err != nil {
		return errors.Wrap(err, "readAugmentedHeader")
	}
	ar.augmentedhInfo = hInfo

	hdr, err := getNext(tr)
	if err != nil {
		return errors.Wrap(err, "readAugmentedHeader")
	}

	// Next step is setting correct installers based on update types being
	// part of the artifact.
	if err = ar.setInstallers(hInfo.Updates, true); err != nil {
		return errors.Wrap(err, "readAugmentedHeader")
	}

	// At the end read rest of the header using correct installers.
	if err = ar.readHeaderUpdate(tr, hdr, true); err != nil {
		return errors.Wrap(err, "readAugmentedHeader")
	}

	// Check if header checksum is correct.
	if cr, ok := r.(*artifact.Checksum); ok {
		if err = cr.Verify(); err != nil {
			return errors.Wrap(err, "reader: reading header error")
		}
	}

	return nil
}

func ReadVersion(tr *tar.Reader) (*artifact.Info, []byte, error) {
	buf := bytes.NewBuffer(nil)
	// read version file and calculate checksum
	if err := readNext(tr, buf, "version"); err != nil {
		return nil, nil, err
	}
	raw := buf.Bytes()
	info := new(artifact.Info)
	if _, err := io.Copy(info, buf); err != nil {
		return nil, nil, err
	}
	return info, raw, nil
}

func (ar *Reader) RegisterHandler(handler handlers.Installer) error {
	if handler == nil {
		return errors.New("reader: invalid handler")
	}
	if _, ok := ar.handlers[handler.GetUpdateType()]; ok {
		return os.ErrExist
	}
	ar.handlers[handler.GetUpdateType()] = handler
	return nil
}

func (ar *Reader) GetHandlers() map[int]handlers.Installer {
	return ar.installers
}

func (ar *Reader) readHeaderV1(tReader *tar.Reader) error {
	if ar.shouldBeSigned {
		return errors.New("reader: expecting signed artifact; " +
			"v1 is not supporting signatures")
	}
	hdr, err := getNext(tReader)
	if err != nil {
		return errors.New("reader: error reading header")
	}
	if !strings.HasPrefix(hdr.Name, "header.tar.") {
		return errors.Errorf("reader: invalid header element: %v", hdr.Name)
	}

	if err = ar.readHeader(tReader, nil); err != nil {
		return err
	}
	return nil
}

func readManifest(tReader *tar.Reader, name string) (*artifact.ChecksumStore, error) {
	buf := bytes.NewBuffer(nil)
	if err := readNext(tReader, buf, name); err != nil {
		return nil, errors.Wrap(err, "reader: can not buffer manifest")
	}
	manifest := artifact.NewChecksumStore()
	if err := manifest.ReadRaw(buf.Bytes()); err != nil {
		return nil, errors.Wrap(err, "reader: can not read manifest")
	}
	return manifest, nil
}

func signatureReadAndVerify(tReader *tar.Reader, message []byte,
	verify SignatureVerifyFn, signed bool) error {
	// verify signature
	if verify == nil && signed {
		return errors.New("reader: verify signature callback not registered")
	} else if verify != nil {
		// first read signature...
		sig := bytes.NewBuffer(nil)
		if _, err := io.Copy(sig, tReader); err != nil {
			return errors.Wrapf(err, "reader: can not read signature file")
		}
		if err := verify(message, sig.Bytes()); err != nil {
			return errors.Wrapf(err, "reader: invalid signature")
		}
	}
	return nil
}

func verifyVersion(ver []byte, manifest *artifact.ChecksumStore) error {
	verSum, err := manifest.GetAndMark("version")
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(ver)
	c := artifact.NewReaderChecksum(buf, verSum)
	_, err = io.Copy(ioutil.Discard, c)
	return err
}

// The artifact parser needs to take one of the paths listed below when reading
// the artifact version 3.
var artifactV3ParseGrammar = [][]string{
	// Version is already read in ReadArtifact().
	{"manifest", "header.tar.gz"},                                                              // Unsigned.
	{"manifest", "manifest.sig", "header.tar.gz"},                                              // Signed.
	{"manifest", "manifest-augment", "header.tar.gz", "header-augment.tar.gz"},                 // Unsigned, with augment header
	{"manifest", "manifest.sig", "manifest-augment", "header.tar.gz", "header-augment.tar.gz"}, // Signed, with augment header
	// Data is processed in ReadArtifact()
}

var errParseOrder = errors.New("Parse error: The artifact seems to have the wrong structure")

// verifyParseOrder compares the parseOrder against the allowed parse paths through an artifact.
func verifyParseOrder(parseOrder []string) (validToken string, validPath bool, err error) {
	// Do a substring search for the parseOrder sent in on each of the valid grammars.
	for _, validPath := range artifactV3ParseGrammar {
		if len(parseOrder) > len(validPath) {
			continue
		}
		// Check for a submatch in the current validPath.
		for i := range parseOrder {
			if validPath[i] != parseOrder[i] {
				break // Check the next validPath against the parseOrder.
			}
			// We have a submatch. Check if the entire length matches.
			if i == len(parseOrder)-1 {
				if len(parseOrder) == len(validPath) {
					return parseOrder[i], true, nil // Full match.
				}
				return parseOrder[i], false, nil
			}
		}
	}
	return "", false, errParseOrder
}

func (ar *Reader) readHeaderV3(tReader *tar.Reader,
	version []byte) (*artifact.ChecksumStore, error) {
	manifestChecksumStore := artifact.NewChecksumStore()
	parsePath := []string{}

	for {
		hdr, err := tReader.Next()
		if err == io.EOF {
			return nil, errors.New("The artifact does not contain all required fields")
		}
		if err != nil {
			return nil, errors.Wrap(err, "readHeaderV3")
		}
		parsePath = append(parsePath, hdr.Name)
		nextParseToken, validPath, err := verifyParseOrder(parsePath)
		// Only error returned is errParseOrder.
		if err != nil {
			return nil, fmt.Errorf("Invalid structure: %s, wrong element: %s", parsePath, parsePath[len(parsePath)-1])
		}
		err = ar.handleHeaderReads(nextParseToken, tReader, manifestChecksumStore, version)
		if err != nil {
			return nil, errors.Wrap(err, "readHeaderV3")
		}
		if validPath {
			// Artifact should be signed, but isn't, so do not process the update.
			if ar.shouldBeSigned && !ar.IsSigned {
				return nil,
					errors.New("reader: expecting signed artifact, but no signature file found")
			}
			break // return and process the /data records in ReadArtifact()
		}
	}

	// Now assign all the files we got in the manifest to the correct
	// installers. The files are indexed by their `data/xxxx` prefix.
	if err := ar.assignUpdateFiles(); err != nil {
		return nil, err
	}

	return manifestChecksumStore, nil
}

func (ar *Reader) handleHeaderReads(headerName string, tReader *tar.Reader, manifestChecksumStore *artifact.ChecksumStore, version []byte) error {
	var err error
	switch headerName {
	case "manifest":
		// Get the data from the manifest.
		ar.files, err = readManifestHeader(ar, tReader, manifestChecksumStore)
		// verify checksums of version
		if err = verifyVersion(version, manifestChecksumStore); err != nil {
			return err
		}
		return err
	case "manifest.sig":
		ar.IsSigned = true
		// First read and verify signature
		if err = signatureReadAndVerify(tReader, manifestChecksumStore.GetRaw(),
			ar.VerifySignatureCallback, ar.shouldBeSigned); err != nil {
			return err
		}
	case "manifest-augment":
		// Get the data from the augmented manifest.
		ar.augmentFiles, err = readManifestHeader(ar, tReader, manifestChecksumStore)
		return err
	case "header.tar.gz":
		// Get and verify checksums of header.
		hc, err := manifestChecksumStore.GetAndMark("header.tar.gz")
		if err != nil {
			return err
		}

		if err := ar.readHeader(tReader, hc); err != nil {
			return errors.Wrap(err, "handleHeaderReads")
		}
	case "header-augment.tar.gz":
		// Get and verify checksums of the augmented header.
		hc, err := manifestChecksumStore.GetAndMark("header-augment.tar.gz")
		if err != nil {
			return err
		}
		if err := ar.readAugmentedHeader(tReader, hc); err != nil {
			return errors.Wrap(err, "handleHeaderReads: Failed to read the augmented header")
		}
	default:
		return errors.Errorf("reader: found unexpected file in artifact: %v",
			headerName)
	}
	return nil
}

func readManifestHeader(ar *Reader, tReader *tar.Reader, manifestChecksumStore *artifact.ChecksumStore) ([]handlers.DataFile, error) {
	buf := bytes.NewBuffer(nil)
	_, err := io.Copy(buf, tReader)
	if err != nil {
		return nil, errors.Wrap(err, "readHeaderV3: Failed to copy to the byte buffer, from the tar reader")
	}
	err = manifestChecksumStore.ReadRaw(buf.Bytes())
	if err != nil {
		return nil, errors.Wrap(err, "readHeaderV3: Failed to populate the manifest's checksum store")
	}
	lines := bytes.Split(buf.Bytes(), []byte("\n"))
	files := make([]handlers.DataFile, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		split := bytes.SplitN(line, []byte("  "), 2)
		if len(split) != 2 {
			return nil, fmt.Errorf("Garbled entry in manifest: '%s'", line)
		}
		files = append(files, handlers.DataFile{Name: string(split[1])})
	}
	return files, nil
}

func (ar *Reader) readHeaderV2(tReader *tar.Reader,
	version []byte) (*artifact.ChecksumStore, error) {
	// first file after version MUST contain all the checksums
	manifest, err := readManifest(tReader, "manifest")
	if err != nil {
		return nil, err
	}

	// check what is the next file in the artifact
	// depending if artifact is signed or not we can have
	// either header or signature file
	hdr, err := getNext(tReader)
	if err != nil {
		return nil, errors.Wrapf(err, "reader: error reading file after manifest")
	}

	// we are expecting to have a signed artifact, but the signature is missing
	if ar.shouldBeSigned && (hdr.FileInfo().Name() != "manifest.sig") {
		return nil,
			errors.New("reader: expecting signed artifact, but no signature file found")
	}

	switch hdr.FileInfo().Name() {
	case "manifest.sig":
		ar.IsSigned = true
		// firs read and verify signature
		if err = signatureReadAndVerify(tReader, manifest.GetRaw(),
			ar.VerifySignatureCallback, ar.shouldBeSigned); err != nil {
			return nil, err
		}
		// verify checksums of version
		if err = verifyVersion(version, manifest); err != nil {
			return nil, err
		}

		// ...and then header
		hdr, err = getNext(tReader)
		if err != nil {
			return nil, errors.New("reader: error reading header")
		}
		if !strings.HasPrefix(hdr.Name, "header.tar.gz") {
			return nil, errors.Errorf("reader: invalid header element: %v", hdr.Name)
		}
		fallthrough

	case "header.tar.gz":
		// get and verify checksums of header
		hc, err := manifest.GetAndMark("header.tar.gz")
		if err != nil {
			return nil, err
		}

		// verify checksums of version
		if err = verifyVersion(version, manifest); err != nil {
			return nil, err
		}

		if err := ar.readHeader(tReader, hc); err != nil {
			return nil, err
		}

	default:
		return nil, errors.Errorf("reader: found unexpected file in artifact: %v",
			hdr.FileInfo().Name())
	}
	return manifest, nil
}

func (ar *Reader) ReadArtifact() error {
	// each artifact is tar archive
	if ar.r == nil {
		return errors.New("reader: read artifact called on invalid stream")
	}
	tReader := tar.NewReader(ar.r)

	// first file inside the artifact MUST be version
	ver, vRaw, err := ReadVersion(tReader)
	if err != nil {
		return errors.Wrapf(err, "reader: can not read version file")
	}
	ar.info = ver

	var s *artifact.ChecksumStore

	switch ver.Version {
	case 1:
		if err = ar.readHeaderV1(tReader); err != nil {
			return err
		}
	case 2:
		s, err = ar.readHeaderV2(tReader, vRaw)
		if err != nil {
			return err
		}
	case 3:
		s, err = ar.readHeaderV3(tReader, vRaw)
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("reader: unsupported version: %d", ver.Version)
	}
	err = ar.readData(tReader, s)
	if err != nil {
		return err
	}
	if s != nil {
		notMarked := s.FilesNotMarked()
		if len(notMarked) > 0 {
			return fmt.Errorf("Files found in manifest(s), that were not part of artifact: %s", strings.Join(notMarked, ", "))
		}
	}

	return nil
}

func (ar *Reader) GetCompatibleDevices() []string {
	if ar.hInfo == nil {
		return nil
	}
	return ar.hInfo.GetCompatibleDevices()
}

func (ar *Reader) GetArtifactName() string {
	if ar.hInfo == nil {
		return ""
	}
	return ar.hInfo.GetArtifactName()
}

func (ar *Reader) GetInfo() artifact.Info {
	return *ar.info
}

func (ar *Reader) GetUpdates() []artifact.UpdateType {
	if ar.hInfo == nil {
		return nil
	}
	return ar.hInfo.GetUpdates()
}

// GetArtifactProvides is version 3 specific.
func (ar *Reader) GetArtifactProvides() *artifact.ArtifactProvides {
	return ar.hInfo.GetArtifactProvides()
}

// GetArtifactDepends is version 3 specific.
func (ar *Reader) GetArtifactDepends() *artifact.ArtifactDepends {
	return ar.hInfo.GetArtifactDepends()
}

func (ar *Reader) setInstallers(upd []artifact.UpdateType, augmented bool) error {
	for i, update := range upd {
		// set installer for given update type
		if update.Type == "" {
			if augmented {
				// Just skip empty augmented entries, which
				// means there is no augment override.
				continue
			} else {
				return errors.New("Unexpected empty Payload type")
			}
		} else if w, ok := ar.handlers[update.Type]; ok {
			if augmented {
				var err error
				ar.installers[i], err = w.NewAugmentedInstance(ar.installers[i])
				if err != nil {
					return err
				}
			} else {
				ar.installers[i] = w.NewInstance()
			}
		} else if ar.ForbidUnknownHandlers {
			return fmt.Errorf("Cannot load handler for unknown Payload type '%s'",
				update.Type)
		} else if ar.info.Version >= 3 {
			// For version 3 onwards, use modules for unknown update
			// types.
			if augmented {
				ar.installers[i] = handlers.NewAugmentedModuleImage(ar.installers[i], update.Type)
			} else {
				ar.installers[i] = handlers.NewModuleImage(update.Type)
			}
		} else {
			if augmented {
				return errors.New("augmented set when constructing Generic update. Should not happen")
			}
			// For older versions, use GenericV1V2, which is a stub.
			ar.installers[i] = handlers.NewGenericV1V2(update.Type)
		}
	}
	return nil
}

func (ar *Reader) buildInstallerIndexedFileLists(files []handlers.DataFile) ([][]*handlers.DataFile, error) {
	fileLists := make([][](*handlers.DataFile), len(ar.installers))
	for _, file := range files {
		if !strings.HasPrefix(file.Name, "data"+string(os.PathSeparator)) {
			continue
		}
		index, baseName, err := getUpdateNoFromManifestPath(file.Name)
		if err != nil {
			return nil, err
		}
		if index < 0 || index >= len(ar.installers) {
			return nil, fmt.Errorf("File in manifest does not belong to any Payload: %s", file.Name)
		}

		fileLists[index] = append(fileLists[index], &handlers.DataFile{Name: baseName})
	}
	return fileLists, nil
}

func (ar *Reader) assignUpdateFiles() error {
	fileLists, err := ar.buildInstallerIndexedFileLists(ar.files)
	if err != nil {
		return err
	}
	augmentedFileLists, err := ar.buildInstallerIndexedFileLists(ar.augmentFiles)
	if err != nil {
		return err
	}

	for n, inst := range ar.installers {
		if err := inst.SetUpdateFiles(fileLists[n]); err != nil {
			return err
		}
		if err := inst.SetUpdateAugmentFiles(augmentedFileLists[n]); err != nil {
			return err
		}
	}

	return nil
}

// should be `headers/0000/file` format
func getUpdateNoFromHeaderPath(path string) (int, error) {
	split := strings.Split(path, string(os.PathSeparator))
	if len(split) < 3 {
		return 0, errors.New("can not get Payload order from tar path")
	}
	return strconv.Atoi(split[1])
}

// should be 0000.tar.gz
func getUpdateNoFromDataPath(path string) (int, error) {
	no := strings.TrimSuffix(filepath.Base(path), ".tar.gz")
	return strconv.Atoi(no)
}

// should be data/0000/file
// Returns the index of the data file, converted to int, as well as the
// file name.
func getUpdateNoFromManifestPath(path string) (int, string, error) {
	components := strings.Split(path, string(os.PathSeparator))
	if len(components) != 3 || components[0] != "data" {
		return 0, "", fmt.Errorf("Malformed manifest entry: '%s'", path)
	}
	if len(components[1]) != 4 {
		return 0, "", fmt.Errorf("Manifest entry does not contain four digits: '%s'", path)
	}
	index, err := strconv.Atoi(components[1])
	if err != nil {
		return 0, "", errors.Wrapf(err, "Invalid index in manifest entry: '%s'", path)
	}
	return index, components[2], nil
}

func (ar *Reader) readHeaderUpdate(tr *tar.Reader, hdr *tar.Header, augmented bool) error {
	for {
		// Skip pure directories. mender-artifact doesn't create them,
		// but they may exist if another tool was used to create the
		// artifact.
		if hdr.Typeflag != tar.TypeDir {
			updNo, err := getUpdateNoFromHeaderPath(hdr.Name)
			if err != nil {
				return errors.Wrapf(err, "reader: error getting header Payload number")
			}

			inst, ok := ar.installers[updNo]
			if !ok {
				return errors.Errorf("reader: can not find parser for Payload: %v", hdr.Name)
			}
			if hErr := inst.ReadHeader(tr, hdr.Name, ar.info.Version, augmented); hErr != nil {
				return errors.Wrap(hErr, "reader: can not read header")
			}
		}

		var err error
		hdr, err = getNext(tr)
		if err == io.EOF {
			return nil
		} else if err != nil {
			return errors.Wrapf(err,
				"reader: can not read artifact header file: %v", hdr)
		}
	}
}

func (ar *Reader) readNextDataFile(tr *tar.Reader,
	manifest *artifact.ChecksumStore) error {
	hdr, err := getNext(tr)
	if err == io.EOF {
		return io.EOF
	} else if err != nil {
		return errors.Wrapf(err, "reader: error reading Payload file: [%v]", hdr)
	}
	if filepath.Dir(hdr.Name) != "data" {
		return errors.New("reader: invalid data file name: " + hdr.Name)
	}
	updNo, err := getUpdateNoFromDataPath(hdr.Name)
	if err != nil {
		return errors.Wrapf(err, "reader: error getting data Payload number")
	}
	inst, ok := ar.installers[updNo]
	if !ok {
		return errors.Wrapf(err,
			"reader: can not find parser for parsing data file [%v]", hdr.Name)
	}
	return ar.readAndInstall(tr, inst, manifest, updNo)
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
	if tr == nil {
		return errors.New("reader: read next called on invalid stream")
	}
	hdr, err := getNext(tr)
	if err != nil {
		return errors.Wrap(err, "readNext: Failed to get next header")
	}
	if strings.HasPrefix(hdr.Name, elem) {
		_, err := io.Copy(w, tr)
		return errors.Wrap(err, "readNext: Failed to copy from tarReader to the writer")
	}
	return os.ErrInvalid
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

func getDataFile(i handlers.Installer, name string) *handlers.DataFile {
	for _, file := range i.GetUpdateAllFiles() {
		if name == file.Name {
			return file
		}
	}
	return nil
}

func (ar *Reader) readAndInstall(r io.Reader, i handlers.Installer,
	manifest *artifact.ChecksumStore, no int) error {
	// each data file is stored in tar.gz format
	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrapf(err, "Payload: can not open gz for reading data")
	}
	defer gz.Close()

	updateStorer, err := i.NewUpdateStorer(i.GetUpdateType(), no)
	if err != nil {
		return err
	}
	ar.updateStorers[no] = updateStorer

	tar := tar.NewReader(gz)

	for {
		hdr, err := tar.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "Payload: error reading Artifact file header")
		}

		df := getDataFile(i, hdr.Name)
		if df == nil {
			return errors.Errorf("Payload: can not find data file: %s", hdr.Name)
		}

		// fill in needed data
		info := hdr.FileInfo()
		df.Size = info.Size()
		df.Date = info.ModTime()

		// we need to have a checksum either in manifest file (v2 artifact)
		// or it needs to be pre-filled after reading header
		// all the names of the data files in manifest are written with the
		// archive relative path: data/0000/update.ext4
		if manifest != nil {
			df.Checksum, err = manifest.GetAndMark(filepath.Join(artifact.UpdatePath(no),
				hdr.FileInfo().Name()))
			if err != nil {
				return errors.Wrapf(err, "Payload: checksum missing")
			}
		}
		if df.Checksum == nil {
			return errors.Errorf("Payload: checksum missing for file: %s", hdr.Name)
		}

		// check checksum
		ch := artifact.NewReaderChecksum(tar, df.Checksum)

		if err = updateStorer.StoreUpdate(ch, info); err != nil {
			return errors.Wrapf(err, "Payload: can not install Payload: %v", hdr)
		}

		if err = ch.Verify(); err != nil {
			return errors.Wrap(err, "reader: error reading data")
		}
	}

	return nil
}

func (ar *Reader) GetUpdateStorers() ([]handlers.UpdateStorer, error) {
	length := len(ar.updateStorers)
	list := make([]handlers.UpdateStorer, length)

	for i := range ar.updateStorers {
		if i >= length {
			return []handlers.UpdateStorer{}, errors.New("Update payload numbers are not in strictly increasing numbers from zero")
		}
		list[i] = ar.updateStorers[i]
	}

	return list, nil
}
