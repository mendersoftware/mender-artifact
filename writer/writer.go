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

package awriter

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/mendersoftware/mender-artifact/archiver"
	"github.com/mendersoftware/mender-artifact/metadata"
	"github.com/mendersoftware/mender-artifact/parser"
	"github.com/pkg/errors"
)

// Writer provides on the fly writing of artifacts metadata file used by
// Mender client and server.
// Call Write to start writing artifacts file.
type Writer struct {
	format            string
	version           int
	compatibleDevices []string
	artifactName      string
	// determine if artifact should be signed or not
	signed bool

	aName string
	*parser.ParseManager
	availableUpdates []parser.UpdateData
	aArchiver        *tar.Writer
	aTmpFile         *os.File

	*aHeader
}

type aHeader struct {
	hInfo       metadata.HeaderInfo
	hTmpFile    *os.File
	hArchiver   *tar.Writer
	hCompressor *gzip.Writer
	isClosed    bool
}

func newHeader() *aHeader {
	hFile, err := initHeaderFile()
	if err != nil {
		return nil
	}

	hComp := gzip.NewWriter(hFile)
	hArch := tar.NewWriter(hComp)

	return &aHeader{
		hCompressor: hComp,
		hArchiver:   hArch,
		hTmpFile:    hFile,
	}
}

func (av *Writer) init(path string) error {
	av.aHeader = newHeader()
	if av.aHeader == nil {
		return errors.New("writer: error initializing header")
	}
	var err error
	av.aTmpFile, err = ioutil.TempFile(filepath.Dir(path), "mender")
	if err != nil {
		return err
	}
	av.aArchiver = tar.NewWriter(av.aTmpFile)
	av.aName = path
	return nil
}

func (av *Writer) deinit() error {
	var errHeader error
	if av.aHeader != nil {
		errHeader = av.closeHeader()
		if av.hTmpFile != nil {
			os.Remove(av.hTmpFile.Name())
		}
	}

	if av.aTmpFile != nil {
		os.Remove(av.aTmpFile.Name())
	}

	var errArchiver error
	if av.aArchiver != nil {
		errArchiver = av.aArchiver.Close()
	}
	if errArchiver != nil || errHeader != nil {
		return errors.New("writer: error deinitializing")
	}
	return nil
}

func NewWriter(format string, version int, devices []string, name string,
	signed bool) *Writer {

	return &Writer{
		format:            format,
		version:           version,
		compatibleDevices: devices,
		artifactName:      name,
		signed:            signed,
		ParseManager:      parser.NewParseManager(),
	}
}

func initHeaderFile() (*os.File, error) {
	// we need to create a temporary file for storing the header data
	f, err := ioutil.TempFile("", "header")
	if err != nil {
		return nil, errors.Wrapf(err,
			"writer: error creating temp file for storing header")
	}
	return f, nil
}

type ChecksumWriter struct {
	W io.Writer // underlying writer
	h hash.Hash
	c []byte // checksum
}

func NewChecksumWriter(w io.Writer) *ChecksumWriter {
	h := sha256.New()
	return &ChecksumWriter{
		W: io.MultiWriter(h, w),
		h: h,
	}
}

func (cw *ChecksumWriter) Write(p []byte) (int, error) {
	if cw.W == nil {
		return 0, syscall.EBADF
	}
	return cw.W.Write(p)
}

func (cw *ChecksumWriter) Checksum() []byte {
	sum := cw.h.Sum(nil)
	checksum := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(checksum, cw.h.Sum(nil))
	return checksum
}

func ToStream(m metadata.WriteValidator) []byte {
	if err := m.Validate(); err != nil {
		return nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return data
}

func (aw *Writer) FixedWrite(w io.Writer, upd []parser.UpdateData) error {

	f, ferr := ioutil.TempFile("", "header")
	if ferr != nil {
		return errors.New("writer: can not create temporary header file")
	}
	defer os.Remove(f.Name())

	// write temporary header (we need to know the size before storing in tar)
	if err := func(f *os.File) error {

		hch := NewChecksumWriter(f)
		hgz := gzip.NewWriter(hch)
		htw := tar.NewWriter(hgz)

		defer f.Close()
		defer hgz.Close()
		defer htw.Close()

		if err := aw.writeHeader(htw, upd); err != nil {
			return errors.Wrapf(err, "writer: error writing header")
		}
		return nil
	}(f); err != nil {
		return err
	}

	// mender archive writer
	tw := tar.NewWriter(w)
	defer tw.Close()

	// write version file
	info := aw.getInfo()
	inf := ToStream(&info)

	ch := NewChecksumWriter(tw)
	sa := archiver.NewWriterStream(ch, inf)
	if err := sa.WriteHeader(tw, "version"); err != nil {
		return errors.Wrapf(err, "writer: can not write version tar header")
	}

	if n, err := sa.Write(inf); err != nil || n != len(inf) {
		return errors.New("writer: can not tar version")
	}

	// write header
	fw := archiver.NewWriterFile(tw)
	fw.WriteHeader(f.Name(), "header.tar.gz")

	hFile, err := os.Open(f.Name())
	if err != nil {
		return errors.Wrapf(err, "writer: error opening tmp header for reading")
	}
	defer hFile.Close()

	if _, err := io.Copy(fw, hFile); err != nil {
		return errors.Wrapf(err, "writer: can not tar header")
	}

	// write data
	// if err := aw.WriteData(); err != nil {
	// 	return err
	// }

	return nil

	// if aw.signed {
	// 	w := NewChecksumWriter(w)
	//
	// }
	//
	// switch ver {
	// case 1:
	// 	// write header
	//
	// case 2:
	// 	// write checksums
	//
	// 	if signed {
	// 		// write signature
	//
	// 	}
	//
	// 	// write header
	//
	// default:
	// 	return errors.Errorf("writer: unsupported version: %v", ver)
	// }
	//
	// // write data
}

func (av *Writer) write(updates []parser.UpdateData) error {
	av.availableUpdates = updates

	// write temporary header (we need to know the size before storing in tar)
	if err := av.WriteHeader(); err != nil {
		return err
	}

	// archive info
	info := av.getInfo()
	ia := archiver.NewMetadataArchiver(&info, "version")
	if err := ia.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "writer: error archiving info")
	}

	// archive signatures
	if av.signed {

	}

	// archive header
	ha := archiver.NewFileArchiver(av.hTmpFile.Name(), "header.tar.gz")
	if err := ha.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "writer: error archiving header")
	}
	// archive data
	if err := av.WriteData(); err != nil {
		return err
	}
	// we've been storing everything in temporary file
	if err := av.aArchiver.Close(); err != nil {
		return errors.New("writer: error closing archive")
	}
	// prevent from closing archiver twice
	av.aArchiver = nil

	if err := av.aTmpFile.Close(); err != nil {
		return errors.New("writer: error closing archive temporary file")
	}
	return os.Rename(av.aTmpFile.Name(), av.aName)
}

func (av *Writer) Write(updateDir, atrifactName string) error {
	if err := av.init(atrifactName); err != nil {
		return err
	}
	defer av.deinit()

	updates, err := av.ScanUpdateDirs(updateDir)
	if err != nil {
		return err
	}
	return av.write(updates)
}

func (av *Writer) WriteKnown(updates []parser.UpdateData, atrifactName string) error {
	if err := av.init(atrifactName); err != nil {
		return err
	}
	defer av.deinit()

	return av.write(updates)
}

// This reads `type-info` file in provided directory location.
func getTypeInfo(dir string) (*metadata.TypeInfo, error) {
	iPath := filepath.Join(dir, "type-info")
	f, err := os.Open(iPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info := new(metadata.TypeInfo)
	_, err = io.Copy(info, f)
	if err != nil {
		return nil, err
	}

	if err = info.Validate(); err != nil {
		return nil, err
	}
	return info, nil
}

func getDataFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// we have no data file(s) associated with given header
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "writer: error reading data directory")
	}
	if info.IsDir() {
		updFiles, err := ioutil.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var updates []string
		for _, f := range updFiles {
			updates = append(updates, filepath.Join(dir, f.Name()))
		}
		return updates, nil
	}
	return nil, errors.New("writer: broken data directory")
}

func (av *Writer) readDirContent(dir, cur string) (*parser.UpdateData, error) {
	tInfo, err := getTypeInfo(filepath.Join(dir, cur))
	if err != nil {
		return nil, os.ErrInvalid
	}
	p, err := av.ParseManager.GetRegistered(tInfo.Type)
	if err != nil {
		return nil, errors.Wrapf(err, "writer: error finding parser for [%v]", tInfo.Type)
	}

	data, err := getDataFiles(filepath.Join(dir, cur, "data"))
	if err != nil {
		return nil, err
	}

	upd := parser.UpdateData{
		Path:      filepath.Join(dir, cur),
		DataFiles: data,
		Type:      tInfo.Type,
		P:         p,
	}
	return &upd, nil
}

func (av *Writer) ScanUpdateDirs(dir string) ([]parser.UpdateData, error) {
	// first check  if we have update in current directory
	upd, err := av.readDirContent(dir, "")
	if err == nil {
		return []parser.UpdateData{*upd}, nil
	} else if err != os.ErrInvalid {
		return nil, err
	}

	dirs, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	updates := make([]parser.UpdateData, 0, len(dirs))
	for _, uDir := range dirs {
		if uDir.IsDir() {
			upd, err := av.readDirContent(dir, uDir.Name())
			if err == os.ErrInvalid {
				continue
			} else if err != nil {
				return nil, err
			}
			updates = append(updates, *upd)
		}
	}

	if len(updates) == 0 {
		return nil, errors.New("writer: no update data detected")
	}
	return updates, nil
}

func (h *aHeader) closeHeader() (err error) {
	// We have seen some of header components to cause crash while
	// closing. That's why we are trying to close and clean up as much
	// as possible here and recover() if crash happens.
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("error closing header")
		}
		if err == nil {
			h.isClosed = true
		}
	}()

	if !h.isClosed {
		errArch := h.hArchiver.Close()
		errComp := h.hCompressor.Close()
		errFile := h.hTmpFile.Close()

		if errArch != nil || errComp != nil || errFile != nil {
			err = errors.New("writer: error closing header")
		}
	}
	return
}

func (av *Writer) writeHeader(tw *tar.Writer, updates []parser.UpdateData) error {
	// store header info
	hInfo := new(metadata.HeaderInfo)
	for _, upd := range updates {
		hInfo.Updates =
			append(hInfo.Updates, metadata.UpdateType{Type: upd.Type})
	}
	hInfo.CompatibleDevices = av.compatibleDevices
	hInfo.ArtifactName = av.artifactName

	hinf := ToStream(hInfo)
	sa := archiver.NewWriterStream(tw, hinf)
	sa.WriteHeader(tw, "header-info")
	sa.Write(hinf)

	// hi := archiver.NewMetadataArchiver(hInfo, "header-info")
	// if err := hi.Archive(tw); err != nil {
	// 	return errors.Wrapf(err, "writer: can not store header-info")
	// }
	for cnt := 0; cnt < len(updates); cnt++ {
		err := processNextHeaderDir(tw, &updates[cnt], fmt.Sprintf("%04d", cnt))
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return nil
}

func processNextHeaderDir(tw *tar.Writer, upd *parser.UpdateData,
	order string) error {
	if err := upd.P.ArchiveHeader(tw, filepath.Join("headers", order),
		upd); err != nil {
		return err
	}
	return nil
}

func (av *Writer) WriteHeader() error {
	// store header info
	for _, upd := range av.availableUpdates {
		av.hInfo.Updates =
			append(av.hInfo.Updates, metadata.UpdateType{Type: upd.Type})
	}
	av.hInfo.CompatibleDevices = av.compatibleDevices
	av.hInfo.ArtifactName = av.artifactName

	hi := archiver.NewMetadataArchiver(&av.hInfo, "header-info")
	if err := hi.Archive(av.hArchiver); err != nil {
		return errors.Wrapf(err, "writer: can not store header-info")
	}
	for cnt := 0; cnt < len(av.availableUpdates); cnt++ {
		err := av.processNextHeaderDir(&av.availableUpdates[cnt], fmt.Sprintf("%04d", cnt))
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return av.aHeader.closeHeader()
}

func (av *Writer) processNextHeaderDir(upd *parser.UpdateData, order string) error {
	if err := upd.P.ArchiveHeader(av.hArchiver, filepath.Join("headers", order),
		upd); err != nil {
		return err
	}
	return nil
}

func (av *Writer) WriteData() error {
	for cnt := 0; cnt < len(av.availableUpdates); cnt++ {
		err := av.processNextDataDir(av.availableUpdates[cnt], fmt.Sprintf("%04d", cnt))
		if err != nil {
			return errors.Wrapf(err, "writer: error writing data files")
		}
	}
	return nil
}

func (av *Writer) processNextDataDir(upd parser.UpdateData, order string) error {
	if err := upd.P.ArchiveData(av.aArchiver,
		filepath.Join("data", order+".tar.gz")); err != nil {
		return err
	}
	return nil
}

func (av Writer) getInfo() metadata.Info {
	return metadata.Info{
		Format:  av.format,
		Version: av.version,
	}
}
