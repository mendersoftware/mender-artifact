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

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/artifacts/parsers"
	"github.com/mendersoftware/log"
	"github.com/pkg/errors"
)

// ArtifactsWriter provides on the fly writing of artifacts metadata file used by
// Mender client and server.
// Call Write to start writing artifacts file.
type ArtifactsWriter struct {
	artifactName string
	updDir       string
	format       string
	version      int

	*parsers.Parsers
	*uIterator

	aArchiver *tar.Writer

	hInfo       metadata.HeaderInfo
	hTmpFile    *os.File
	hArchiver   *tar.Writer
	hCompressor *gzip.Writer
}

//TODO:
type upd struct {
	path string
	t    string
	p    parsers.ArtifactParser
}

type uIterator struct {
	cnt    int
	update []upd
}

func (i *uIterator) push(path, t string, p parsers.ArtifactParser) {
	i.update = append(i.update, upd{path, t, p})
}

func (i *uIterator) getNext() (*upd, error) {
	if len(i.update) <= i.cnt {
		return nil, io.EOF
	}
	defer func() { i.cnt++ }()
	return &i.update[i.cnt], nil
}

func (i *uIterator) reset() {
	i.cnt = 0
}

// NewArtifactsWriter creates a new ArtifactsWriter providing a location
// of Mender metadata artifacts, format of the update and version.
func NewArtifactsWriter(name, path, format string, version int) *ArtifactsWriter {
	return &ArtifactsWriter{
		artifactName: name,
		updDir:       path,
		format:       format,
		version:      version,

		Parsers:   parsers.NewParserFactory(),
		uIterator: new(uIterator),
	}
}

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

func (av *ArtifactsWriter) GetUpdateDirs() ([]os.FileInfo, error) {
	dirs, err := ioutil.ReadDir(av.updDir)
	if err != nil {
		return nil, err
	}

	for _, uDir := range dirs {
		log.Infof("reader: scanning dir: %v", uDir.Name())

		tInfo, err := getTypeInfo(filepath.Join(av.updDir, uDir.Name()))
		if err != nil {
			return nil, err
		}

		//TODO:
		av.push(uDir.Name(), tInfo.Type, nil)
		av.hInfo.Updates = append(av.hInfo.Updates, metadata.UpdateType{Type: tInfo.Type})
	}

	return dirs, nil
}

func (av *ArtifactsWriter) createTmpHdrFile() error {
	//TODO:
	f, err := os.Create("/tmp/my_header.tar.gz") //ioutil.TempFile("", "header")
	log.Errorf("temp dir for storing header: %v", f.Name())
	if err != nil {
		return err
	}
	av.hTmpFile = f
	return nil
}

func (av *ArtifactsWriter) Close() error {
	if av.hArchiver != nil {
		av.hArchiver.Close()
	}
	if av.hCompressor != nil {
		av.hCompressor.Close()
	}
	if av.hTmpFile != nil {
		av.hTmpFile.Close()
		//TODO:
		//os.Remove(av.hTmpFile.Name())
	}

	if av.aArchiver != nil {
		av.aArchiver.Close()
	}

	return nil
}

func (av *ArtifactsWriter) initWritingHeader() error {
	// first we need to create a file for storing header
	if err := av.createTmpHdrFile(); err != nil {
		return errors.Wrapf(err,
			"reader: error creating temp file for storing header")
	}
	log.Infof("temp file for storing header: %v", av.hTmpFile.Name())

	av.hCompressor = gzip.NewWriter(av.hTmpFile)
	av.hArchiver = tar.NewWriter(av.hCompressor)

	hi := metadata.NewJSONStreamArchiver(av.hInfo, "header-info")
	if err := hi.Archive(av.hArchiver); err != nil {
		return errors.Wrapf(err, "writer: can not store header-info")
	}
	return nil
}

func (av *ArtifactsWriter) ProcessNextHeaderDir() error {
	u, err := av.getNext()
	if err == io.EOF {
		log.Infof("reader: empty updates list")
		return io.EOF
	}

	log.Infof("processing update dir: %v [%v]", u.path, u.t)

	p, err := av.GetParser(u.t)
	if err != nil {
		return errors.Wrapf(err,
			"writer: can not find parser for update type: [%v]", u.t)
	}
	if err := p.ArchiveHeader(av.hArchiver, filepath.Join(av.updDir, u.path),
		filepath.Join("headers", u.path)); err != nil {
		return err
	}
	u.p = p

	return nil
}

func (av *ArtifactsWriter) ProcessNextDataDir() error {
	u, err := av.getNext()
	if err == io.EOF {
		log.Infof("reader: empty updates list")
		return io.EOF
	}

	log.Infof("processing update file: %v [%v:%v]", u.path, u.t, u.p)

	_, err = av.GetParser(u.t)
	if err != nil {
		return errors.Wrapf(err,
			"writer: can not find parser for update type: [%v]", u.t)
	}
	if err := u.p.ArchiveData(av.aArchiver, filepath.Join(av.updDir, u.path),
		filepath.Join("data", u.path+".tar.gz")); err != nil {
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

func (av *ArtifactsWriter) write() error {
	log.Infof("reading update files from: %v", av.updDir)
	if _, err := av.GetUpdateDirs(); err != nil {
		return err
	}
	if err := av.initWritingHeader(); err != nil {
		return err
	}

	for {
		err := av.ProcessNextHeaderDir()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "reader: error processing update directory")
		}
	}

	//TODO: isolate heared writing into separate struct
	av.hArchiver.Close()
	av.hCompressor.Close()
	av.hTmpFile.Close()

	// here we should have header stored in temporary location
	f, err := os.Create(filepath.Join(av.updDir, av.artifactName))
	if err != nil {
		return errors.Wrapf(err, "reader: can not create artifact file")
	}
	defer f.Close()
	av.aArchiver = tar.NewWriter(f)

	// archive info
	info := av.getInfo()
	if err := info.Validate(); err != nil {
		return errors.New("reader: invalid info")
	}
	ia := metadata.NewJSONStreamArchiver(&info, "info")
	if err := ia.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "reader: error archiving info")
	}
	// archive header
	ha := metadata.NewFileArchiver(av.hTmpFile.Name(), "header.tar.gz")
	if err := ha.Archive(av.aArchiver); err != nil {
		return errors.Wrapf(err, "reader: error archiving header")
	}
	//os.Remove(av.hTmpFile)

	// archive data
	av.reset()
	for {
		log.Errorf("process upd dir")
		err := av.ProcessNextDataDir()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrapf(err, "reader: error processing data files")
		}
	}
	return nil
}

func (av *ArtifactsWriter) Write() error {
	return av.write()
}
