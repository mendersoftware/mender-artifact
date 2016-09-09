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
}

// NewArtifactsWriter creates a new ArtifactsWriter providing a location
// of Mender metadata artifacts, format of the update and version.
func NewArtifactsWriter(name, path, format string, version int) *ArtifactsWriter {
	return &ArtifactsWriter{
		aName:   name,
		updDir:  path,
		format:  format,
		version: version,

		Parsers: parser.NewParserFactory(),
	}
}

func (av *ArtifactsWriter) Write() error {
	log.Infof("reading update files from: %v", av.updDir)

	if err := av.InitWriting(); err != nil {
		return err
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

func (av *ArtifactsWriter) createTmpHdrFile() error {
	f, err := ioutil.TempFile("", "header")
	if err != nil {
		return err
	}
	av.hTmpFile = f
	av.hTmpFilePath = f.Name()
	return nil
}

func (av *ArtifactsWriter) Close() error {
	if err := av.CloseHeader(); err != nil {
		return err
	}
	// remove header file
	os.Remove(av.hTmpFilePath)

	if av.aArchiver != nil {
		if err := av.aArchiver.Close(); err != nil {
			return err
		}
	}
	if av.aFile != nil {
		if err := av.aFile.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (av *ArtifactsWriter) setParsers() error {
	dirs, err := ioutil.ReadDir(av.updDir)
	if err != nil {
		return err
	}

	for _, uDir := range dirs {
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

	return nil
}

func (av *ArtifactsWriter) initWritingArtifact() error {
	// here we should have header stored in temporary location
	f, err := os.Create(filepath.Join(av.updDir, av.aName))
	if err != nil {
		return errors.Wrapf(err, "reader: can not create artifact file")
	}
	av.aFile = f
	av.aArchiver = tar.NewWriter(f)

	// archive info
	info := av.getInfo()
	if err := info.Validate(); err != nil {
		return errors.New("reader: invalid info")
	}
	ia := archiver.NewMetadataArchiver(&info, "info")
	if err := ia.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "reader: error archiving info")
	}
	return nil
}

func (av *ArtifactsWriter) initWritingHeader() error {
	// we need to create a file for storing header
	if err := av.createTmpHdrFile(); err != nil {
		return errors.Wrapf(err,
			"reader: error creating temp file for storing header")
	}
	log.Infof("temp file for storing header: %v", av.hTmpFile.Name())

	av.hCompressor = gzip.NewWriter(av.hTmpFile)
	av.hArchiver = tar.NewWriter(av.hCompressor)

	hi := archiver.NewMetadataArchiver(av.hInfo, "header-info")
	if err := hi.Archive(av.hArchiver); err != nil {
		return errors.Wrapf(err, "writer: can not store header-info")
	}
	return nil
}

func (av *ArtifactsWriter) InitWriting() error {
	if err := av.setParsers(); err != nil {
		return err
	}
	if err := av.initWritingHeader(); err != nil {
		return err
	}
	if err := av.initWritingArtifact(); err != nil {
		return err
	}
	return nil
}

func (av *ArtifactsWriter) CloseHeader() error {
	if av.hArchiver != nil {
		if err := av.hArchiver.Close(); err != nil {
			return errors.Wrapf(err, "reader: error closing header archiver")
		}
		av.hArchiver = nil
	}
	if av.hCompressor != nil {
		if err := av.hCompressor.Close(); err != nil {
			return errors.Wrapf(err, "reader: error closing header compression")
		}
		av.hCompressor = nil
	}
	if av.hTmpFile != nil {
		if err := av.hTmpFile.Close(); err != nil {
			return errors.Wrapf(err, "reader: error closing header file: %v", av.hTmpFile.Name())
		}
		av.hTmpFile = nil
	}
	return nil
}

func (av *ArtifactsWriter) ProcessHeader() error {
	// first make sure we are iterating form the beginning
	av.Parsers.Reset()

	for {
		err := av.ProcessNextHeaderDir()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "reader: error processing update directory")
		}
	}
	return av.CloseHeader()
}

func (av *ArtifactsWriter) ProcessNextHeaderDir() error {
	p, upd, err := av.Parsers.Next()
	if err == io.EOF {
		log.Infof("reader: reached header EOF")
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
