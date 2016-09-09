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

package writer

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mendersoftware/artifacts/archiver"
	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/artifacts/parser"
	"github.com/mendersoftware/log"
	"github.com/pkg/errors"
)

// ArtifactsWriter provides on the fly writing of artifacts metadata file used by
// Mender client and server.
// Call Write to start writing artifacts file.
type ArtifactsWriter struct {
	aName   string
	updDir  string
	format  string
	version int

	*parser.Parsers

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
func NewArtifactsWriter(name, path, format string, version int) *ArtifactsWriter {
	aFile, err := createArtFile(path, name)
	if err != nil {
		return nil
	}
	arch := tar.NewWriter(aFile)

	hdr := makeHeader()
	if hdr == nil {
		return nil
	}

	return &ArtifactsWriter{
		aName:     name,
		updDir:    path,
		format:    format,
		version:   version,
		aFile:     aFile,
		aArchiver: arch,

		aHeader: *hdr,
		Parsers: parser.NewParserFactory(),
	}
}

func createArtFile(dir, name string) (*os.File, error) {
	// here we should have header stored in temporary location
	fPath := filepath.Join(dir, name)
	f, err := os.Create(fPath)
	if err != nil {
		log.Errorf("writer: error creating artifact file: %v", fPath)
		return nil, errors.Wrapf(err, "reader: can not create artifact file")
	}
	return f, nil
}

func initHeaderFile() (*os.File, error) {
	// we need to create a file for storing header
	f, err := ioutil.TempFile("", "header")
	if err != nil {
		return nil, errors.Wrapf(err,
			"reader: error creating temp file for storing header")
	}

	return f, nil
}

func (av *ArtifactsWriter) Write() error {
	log.Infof("reading update files from: %v", av.updDir)

	if err := av.InitWriting(); err != nil {
		return err
	}

	// archive info
	info := av.getInfo()
	ia := archiver.NewMetadataArchiver(&info, "info")
	if err := ia.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "reader: error archiving info")
	}

	// scan header
	if err := av.ProcessHeader(); err != nil {
		return err
	}
	// archive header
	ha := archiver.NewFileArchiver(av.hTmpFilePath, "header.tar.gz")
	if err := ha.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "reader: error archiving header")
	}
	// archive data
	if err := av.ProcessData(); err != nil {
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

func (av *ArtifactsWriter) Close() (err error) {
	//finalize header
	av.closeHeader()

	if av.hTmpFilePath != "" {
		os.Remove(av.hTmpFilePath)
	}

	if av.aArchiver != nil {
		err = av.aArchiver.Close()
		if err != nil {
			log.Errorf("reader: errro closing archive: %v", err)
		}
	}
	if av.aFile != nil {
		err = av.aFile.Close()
		if err != nil {
			log.Errorf("reader: errro closing artifact file: %v", err)
		}
	}
	return err
}

func (av *ArtifactsWriter) setParsers() error {
	dirs, err := ioutil.ReadDir(av.updDir)
	if err != nil {
		return err
	}

	for _, uDir := range dirs {
		if uDir.IsDir() {
			log.Infof("reader: scanning dir: %v", uDir.Name())
			tInfo, err := getTypeInfo(filepath.Join(av.updDir, uDir.Name()))
			if err != nil {
				return err
			}
			p, err := av.GetParser(tInfo.Type)
			if err != nil {
				return errors.Wrapf(err, "writer: error finding parser for [%v]", tInfo.Type)
			}
			av.Parsers.PushParser(p, uDir.Name())
			av.hInfo.Updates =
				append(av.hInfo.Updates, metadata.UpdateType{Type: tInfo.Type})
		}
	}
	return nil
}

func (av *ArtifactsWriter) InitWriting() error {

	if err := av.setParsers(); err != nil {
		return err
	}
	return nil
}

func (h *aHeader) closeHeader() (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error closing: %v", r)
			err = errors.New("error closing header")
		}
		if err == nil {
			h.isClosed = true
		}
	}()

	if !h.isClosed {
		errArch := h.hArchiver.Close()
		if errArch != nil {
			log.Error("reader: error clossing header archive")
		}
		errComp := h.hCompressor.Close()
		if errComp != nil {
			log.Error("reader: error clossing header compressor")
		}
		errFile := h.hTmpFile.Close()
		if errFile != nil {
			log.Error("reader: error clossing header temp file")
		}

		if errArch != nil || errComp != nil || errFile != nil {
			err = errors.New("reader: error closing header")
		}
	}

	return err
}

func (av *ArtifactsWriter) ProcessHeader() error {
	// store header info
	hi := archiver.NewMetadataArchiver(&av.hInfo, "header-info")
	if err := hi.Archive(av.hArchiver); err != nil {
		return errors.Wrapf(err, "writer: can not store header-info")
	}

	// make sure we are iterating form the beginning
	av.Parsers.Reset()

	for {
		err := av.ProcessNextHeaderDir()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "reader: error processing update directory")
		}
	}
	return nil
}

func (av *ArtifactsWriter) ProcessNextHeaderDir() error {
	p, upd, err := av.Parsers.Next()
	if err == io.EOF {
		log.Infof("reader: reached header EOF")
		if err = av.closeHeader(); err != nil {
			return errors.Wrapf(err, "error closing header")
		}
		return io.EOF
	}
	log.Infof("processing update dir: %v [%+v]", upd, p)

	if err := p.ArchiveHeader(av.hArchiver, filepath.Join(av.updDir, upd),
		filepath.Join("headers", upd)); err != nil {
		return err
	}
	return nil
}

func (av *ArtifactsWriter) ProcessData() error {
	// first make sure we are iterating form the beginning
	av.Parsers.Reset()

	for {
		err := av.ProcessNextDataDir()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "reader: error processing data files")
		}
	}
	return nil
}

func (av *ArtifactsWriter) ProcessNextDataDir() error {
	p, upd, err := av.Parsers.Next()
	if err == io.EOF {
		log.Infof("reader: reached data EOF")
		return io.EOF
	}

	log.Infof("processing data: %v [%+v]", upd, p)
	if err := p.ArchiveData(av.aArchiver, filepath.Join(av.updDir, upd),
		filepath.Join("data", upd+".tar.gz")); err != nil {
		return err
	}
	return nil
}

func (av ArtifactsWriter) getInfo() metadata.Info {
	return metadata.Info{
		Format:  av.format,
		Version: av.version,
	}
}
