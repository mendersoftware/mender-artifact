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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mendersoftware/artifacts/archiver"
	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/artifacts/parser"
	"github.com/pkg/errors"
)

// ArtifactsWriter provides on the fly writing of artifacts metadata file used by
// Mender client and server.
// Call Write to start writing artifacts file.
type Writer struct {
	aName  string
	updDir string

	format  string
	version int

	*parser.ParseManager
	updateDirs []string

	aArchiver *tar.Writer
	aFile     *os.File
	aHeader
}

type aHeader struct {
	hInfo        metadata.HeaderInfo
	hTmpFile     *os.File
	hTmpFilePath string
	hArchiver    *tar.Writer
	hCompressor  *gzip.Writer
	isClosed     bool
}

func makeHeader() *aHeader {
	hFile, err := initHeaderFile()
	if err != nil {
		return nil
	}

	hComp := gzip.NewWriter(hFile)
	hArch := tar.NewWriter(hComp)

	return &aHeader{
		hCompressor:  hComp,
		hArchiver:    hArch,
		hTmpFile:     hFile,
		hTmpFilePath: hFile.Name(),
	}
}

// NewArtifactsWriter creates a new ArtifactsWriter providing a location
// of Mender metadata artifacts, format of the update and version.
func NewWriter(name, path, format string, version int) *Writer {
	aFile, err := createArtFile(path, "artifact.mender")
	if err != nil {
		return nil
	}
	arch := tar.NewWriter(aFile)

	hdr := makeHeader()
	if hdr == nil {
		return nil
	}

	return &Writer{
		aName:     name,
		updDir:    path,
		format:    format,
		version:   version,
		aFile:     aFile,
		aArchiver: arch,

		aHeader:      *hdr,
		ParseManager: parser.NewParseManager(),
	}
}

func createArtFile(dir, name string) (*os.File, error) {
	// here we should have header stored in temporary location
	fPath := filepath.Join(dir, name)
	f, err := os.Create(fPath)
	if err != nil {
		return nil, errors.Wrapf(err, "writer: can not create artifact file: %v", fPath)
	}
	return f, nil
}

func initHeaderFile() (*os.File, error) {
	// we need to create a file for storing header
	f, err := ioutil.TempFile("", "header")
	if err != nil {
		return nil, errors.Wrapf(err,
			"writer: error creating temp file for storing header")
	}

	return f, nil
}

func (av *Writer) Write() error {
	if err := av.ScanUpdateDirs(); err != nil {
		return err
	}
	// scan header
	if err := av.WriteHeader(); err != nil {
		return err
	}

	// archive info
	info := av.getInfo()
	ia := archiver.NewMetadataArchiver(&info, "info")
	if err := ia.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "writer: error archiving info")
	}
	// archive header
	ha := archiver.NewFileArchiver(av.hTmpFilePath, "header.tar.gz")
	if err := ha.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "writer: error archiving header")
	}
	// archive data
	if err := av.WriteData(); err != nil {
		return err
	}
	return nil
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

func (av *Writer) Close() (err error) {
	av.closeHeader()

	if av.hTmpFilePath != "" {
		os.Remove(av.hTmpFilePath)
	}

	var errArch error
	if av.aArchiver != nil {
		errArch = av.aArchiver.Close()
	}

	var errFile error
	if av.aFile != nil {
		errFile = av.aFile.Close()
	}

	if errArch != nil || errFile != nil {
		err = errors.New("writer: close error")
	} else {
		os.Rename(av.aFile.Name(), filepath.Join(av.updDir, av.aName))
	}
	return
}

func (av *Writer) readDirContent(dir string) error {
	tInfo, err := getTypeInfo(filepath.Join(av.updDir, dir))
	if err != nil {
		return os.ErrInvalid
	}
	p, err := av.ParseManager.GetRegistered(tInfo.Type)
	if err != nil {
		return errors.Wrapf(err, "writer: error finding parser for [%v]", tInfo.Type)
	}
	av.ParseManager.PushWorker(p, dir)
	av.hInfo.Updates =
		append(av.hInfo.Updates, metadata.UpdateType{Type: tInfo.Type})
	av.updateDirs = append(av.updateDirs, dir)
	return nil
}

func (av *Writer) ScanUpdateDirs() error {
	// first check  if we have plain dir update
	if err := av.readDirContent(""); err == nil {
		return nil
	} else if err != os.ErrInvalid {
		return err
	}

	dirs, err := ioutil.ReadDir(av.updDir)
	if err != nil {
		return err
	}

	for _, uDir := range dirs {
		if uDir.IsDir() {
			err := av.readDirContent(uDir.Name())
			if err == os.ErrInvalid {
				continue
			} else if err != nil {
				return err
			}
		}
	}

	if len(av.updateDirs) == 0 {
		return errors.New("writer: no update data detected")
	}
	return nil
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

func (av *Writer) WriteHeader() error {
	// store header info
	hi := archiver.NewMetadataArchiver(&av.hInfo, "header-info")
	if err := hi.Archive(av.hArchiver); err != nil {
		return errors.Wrapf(err, "writer: can not store header-info")
	}
	for cnt := 0; cnt < len(av.updateDirs); cnt++ {
		err := av.processNextHeaderDir(av.updateDirs[cnt], fmt.Sprintf("%04d", cnt))
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return av.aHeader.closeHeader()
}

func (av *Writer) processNextHeaderDir(update, order string) error {
	p, err := av.ParseManager.GetWorker(update)
	if err != nil {
		return errors.Wrapf(err, "writer: can not find header parser: %v", update)
	}

	if err := p.ArchiveHeader(av.hArchiver, filepath.Join(av.updDir, update),
		filepath.Join("headers", order)); err != nil {
		return err
	}
	return nil
}

func (av *Writer) WriteData() error {
	for cnt := 0; cnt < len(av.updateDirs); cnt++ {
		err := av.processNextDataDir(av.updateDirs[cnt], fmt.Sprintf("%04d", cnt))
		if err != nil {
			return errors.Wrapf(err, "writer: error writing data files")
		}
	}
	return av.Close()
}

func (av *Writer) processNextDataDir(update, order string) error {
	p, err := av.ParseManager.GetWorker(update)
	if err != nil {
		return errors.Wrapf(err, "witer: can not find data parser: %v", update)
	}

	if err := p.ArchiveData(av.aArchiver, filepath.Join(av.updDir, update),
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
