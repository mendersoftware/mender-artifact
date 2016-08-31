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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/log"
)

// ArtifactsWriter provides on the fly writing of artifacts metadata file used by
// Mender client and server.
// Call Write to start writing artifacts file.
type ArtifactsWriter struct {
	updateLocation  string
	headerStructure metadata.MetadataArtifactHeader
	format          string
	version         int
	updates         map[string]updateBucket
}

// ArtifactsHeaderFormat provides the structure of the files and
// directories required for creating artifacts file.
// Some of the files are optional and will be created while creating
// artifacts archive.
var ArtifactsHeaderFormat = map[string]metadata.MetadataDirEntry{
	// while calling filepath.Walk() `.` (root) directory is included
	// when iterating throug entries in the tree
	".":               {Path: ".", IsDir: true, Required: false},
	"files":           {Path: "files", IsDir: false, Required: false},
	"meta-data":       {Path: "meta-data", IsDir: false, Required: true},
	"type-info":       {Path: "type-info", IsDir: false, Required: true},
	"checksums":       {Path: "checksums", IsDir: true, Required: false},
	"checksums/*":     {Path: "checksums", IsDir: false, Required: false},
	"signatures":      {Path: "signatures", IsDir: true, Required: true},
	"signatures/*":    {Path: "signatures", IsDir: false, Required: true},
	"scripts":         {Path: "scripts", IsDir: true, Required: false},
	"scripts/pre":     {Path: "scripts/pre", IsDir: true, Required: false},
	"scripts/pre/*":   {Path: "scripts/pre", IsDir: false, Required: false},
	"scripts/post":    {Path: "scripts/post", IsDir: true, Required: false},
	"scripts/post/*":  {Path: "scripts/post", IsDir: false, Required: false},
	"scripts/check":   {Path: "scripts/check", IsDir: true, Required: false},
	"scripts/check/*": {Path: "scripts/check/*", IsDir: false, Required: false},
	// we must have data directory containing update
	"data":   {Path: "data", IsDir: true, Required: true},
	"data/*": {Path: "data/*", IsDir: false, Required: true},
}

// NewArtifactWritter creates a new ArtifactsWriter providing a location
// of Mender metadata artifacts, format of the update and version.
func NewArtifactWritter(path string, format string, version int) *ArtifactsWriter {
	return &ArtifactsWriter{
		updateLocation:  path,
		headerStructure: metadata.MetadataArtifactHeader{Artifacts: ArtifactsHeaderFormat},
		format:          format,
		version:         version,
		updates:         make(map[string]updateBucket),
	}
}

type updateArtifact struct {
	name         string
	path         string
	updateBucket string
	info         os.FileInfo
	checksum     updateChecksum
}

// We need special type to implement metadata.Validater interface
type updateChecksum string

func (uc updateChecksum) Validate() error { return nil }

type updateBucket struct {
	location        string
	path            string
	archivedPath    string
	updateArtifacts []updateArtifact
	files           metadata.MetadataFiles
}

// ReadArchiver provides interface for reading files or streams and preparing
// those to be written to tar archive.
// GetHeader returns Header to be written to the crrent entry it the tar archive.
type ReadArchiver interface {
	io.ReadCloser
	GetHeader() (*tar.Header, error)
}

func (av ArtifactsWriter) calculateChecksum(upd *updateArtifact) error {
	f, err := os.Open(upd.path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	upd.checksum = updateChecksum(hex.EncodeToString(h.Sum(nil)))
	log.Debugf("hash of file: %v (%x)\n", upd.path, upd.checksum)
	return nil
}

func (av ArtifactsWriter) getHeaderInfo(updates []os.FileInfo) metadata.MetadataHeaderInfo {
	// for now we have ONLY one type of update - rootfs-image
	headerInfo := metadata.MetadataHeaderInfo{}

	// TODO: should we store update name as well?
	for _ = range updates {
		headerInfo.Updates = append(headerInfo.Updates, metadata.MetadataUpdateType{Type: "rootfs-image"})
	}
	return headerInfo
}

func (av ArtifactsWriter) getInfo() metadata.MetadataInfo {
	return metadata.MetadataInfo{
		Format:  av.format,
		Version: av.version,
	}
}

func (av ArtifactsWriter) archiveData(updates *updateBucket) error {
	destination := filepath.Join(av.updateLocation, "data")

	// create directory and file for archiving data
	if err := os.MkdirAll(destination, os.ModeDir|os.ModePerm); err != nil {
		return err
	}
	dataArchive, err := os.Create(filepath.Join(destination, updates.location+".tar.gz"))
	if err != nil {
		return err
	}
	defer dataArchive.Close()

	// TODO: start with something simple for now (gzip)
	gw := gzip.NewWriter(dataArchive)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	// don't defer close here as we need to make sure closing was successful

	for _, update := range updates.updateArtifacts {
		// usually it is a good idea to change hdr.Name, but
		// we are happy with relative hdr.Name below
		hdr, err := tar.FileInfoHeader(update.info, update.info.Name())
		if err != nil {
			tw.Close()
			return err
		}

		if err = tw.WriteHeader(hdr); err != nil {
			log.Fatalln(err)
			return err
		}
		f, err := os.Open(update.path)
		if err != nil {
			return err
		}
		defer f.Close()

		// on the fly copy
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

	}
	// Make sure to check the error on Close.
	if err := tw.Close(); err != nil {
		log.Fatalln(err)
		return err
	}

	updates.archivedPath = dataArchive.Name()

	return nil
}

func (mv ArtifactsWriter) createCompressedHeader(updates []os.FileInfo) error {

	archive, err := os.Create(filepath.Join(mv.updateLocation, "header.tar.gz"))
	if err != nil {
		return err
	}
	defer archive.Close()

	// start with something simple for now
	gw := gzip.NewWriter(archive)
	defer gw.Close()

	tw := tar.NewWriter(gw)

	// we need to ensure correct ordering of files
	var tarContent []ReadArchiver
	sr := NewStreamArchiver(mv.getHeaderInfo(updates), "header-info")
	tarContent = append(tarContent, sr)

	for _, update := range updates {
		bucket, ok := mv.updates[update.Name()]

		if !ok {
			return errors.New("artifacts writer: ")
		}
		updateTarLocation := filepath.Join("headers", update.Name())
		tarContent = append(tarContent, NewStreamArchiver(bucket.files,
			filepath.Join(updateTarLocation, "files")))
		tarContent = append(tarContent, NewFileArchiver(filepath.Join(bucket.path, "type-info"),
			filepath.Join(updateTarLocation, "type-info")))
		tarContent = append(tarContent, NewFileArchiver(filepath.Join(bucket.path, "meta-data"),
			filepath.Join(updateTarLocation, "meta-data")))

		for _, upd := range bucket.updateArtifacts {
			fileName := strings.TrimSuffix(upd.name, filepath.Ext(upd.name)) + ".sha256sum"
			tarContent = append(tarContent, NewStreamArchiver(upd.checksum,
				filepath.Join(updateTarLocation, "checksums", fileName)))
		}
		for _, upd := range bucket.updateArtifacts {
			fileName := strings.TrimSuffix(upd.name, filepath.Ext(upd.name)) + ".sig"
			fr := NewFileArchiver(filepath.Join(bucket.path, "signatures", fileName),
				filepath.Join(updateTarLocation, "signatures", fileName))
			tarContent = append(tarContent, fr)
		}
		// TODO: scripts
		//tarContent = append(tarContent, NewPlainFile(filepath.Join(bucket.path, "scripts"), filepath.Join("headers", update.Name(), "scripts")))

	}
	for _, file := range tarContent {
		v := reflect.ValueOf(file)
		if v.IsNil() {
			log.Errorf("artifacts writer: broken entry %v", v)
			return err
		}
		defer file.Close()
		hdr, err := file.GetHeader()
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		// on the fly copy
		if _, err := io.Copy(tw, file); err != nil {
			return err
		}
	}
	// Make sure to check the error on Close.
	if err := tw.Close(); err != nil {
		log.Fatalln(err)
		return err
	}
	return nil
}

func (mv *ArtifactsWriter) getScripts(bucket string) error {
	// TODO:
	return nil
}

func (mv *ArtifactsWriter) processUpdateBucket(bucket string) error {

	// get list of update files
	// at this point we know that `data` exists and contains update(s)
	bucketLocation := filepath.Join(mv.updateLocation, bucket)
	dataLocation := filepath.Join(mv.updateLocation, bucket, "data")

	updateFiles, err := ioutil.ReadDir(dataLocation)
	if err != nil {
		return err
	}
	updBucket := updateBucket{}
	for _, file := range updateFiles {
		upd := updateArtifact{
			name:         file.Name(),
			path:         filepath.Join(dataLocation, file.Name()),
			updateBucket: bucket,
			info:         file,
		}
		// generate needed checksums
		err = mv.calculateChecksum(&upd)
		if err != nil {
			return err
		}

		// generate signatures

		// generate `file` data
		updBucket.files.Files =
			append(updBucket.files.Files, metadata.MetadataFile{File: upd.name})

		updBucket.updateArtifacts = append(updBucket.updateArtifacts, upd)
		updBucket.location = bucket
		updBucket.path = bucketLocation
	}

	if err = updBucket.files.Validate(); err != nil {
		return err
	}

	// move (and compress) updates from `data` to `../data/location.tar.gz`
	if err = mv.archiveData(&updBucket); err != nil {
		return err
	}

	mv.updates[bucket] = updBucket

	return nil
}

func (mv *ArtifactsWriter) writeArchive(compressed bool) error {
	return nil
}

func (mv *ArtifactsWriter) createArtifact(files []os.FileInfo) error {
	artifact, err := os.Create(filepath.Join(mv.updateLocation, "artifact.mender"))
	if err != nil {
		return err
	}
	defer artifact.Close()

	// start with something simple for now
	tw := tar.NewWriter(artifact)

	// we need to ensure correct ordering of files
	var artifactContent []ReadArchiver

	aInfo := NewStreamArchiver(mv.getInfo(), "info")
	artifactContent = append(artifactContent, aInfo)
	aHdr := NewFileArchiver(filepath.Join(mv.updateLocation, "header.tar.gz"), "header.tar.gz")
	artifactContent = append(artifactContent, aHdr)

	for _, artifact := range files {
		bucket, ok := mv.updates[artifact.Name()]

		if !ok {
			return errors.New("artifacts writer: ")
		}
		aData := NewFileArchiver(bucket.archivedPath, "data/0000.tar.gz")
		artifactContent = append(artifactContent, aData)
	}

	for _, file := range artifactContent {
		v := reflect.ValueOf(file)
		if v.IsNil() {
			log.Errorf("artifacts writer: broken entry %v", v)
			return err
		}
		defer file.Close()
		hdr, err := file.GetHeader()
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		// on the fly copy
		if _, err := io.Copy(tw, file); err != nil {
			return err
		}
	}

	// Make sure to check the error on Close.
	if err := tw.Close(); err != nil {
		log.Fatalln(err)
		return err
	}

	return nil
}

func (av *ArtifactsWriter) removeCompressedHeader() error {
	// remove temporary header file
	return os.Remove(filepath.Join(av.updateLocation, "header.tar.gz"))
}

func (av *ArtifactsWriter) Write() error {
	// get directories list containing updates
	entries, err := ioutil.ReadDir(av.updateLocation)
	if err != nil {
		return err
	}

	// iterate through all directories containing updates
	for _, location := range entries {
		// check files and directories consistency
		err = av.headerStructure.CheckHeaderStructure(
			filepath.Join(av.updateLocation, location.Name()))
		if err != nil {
			return err
		}

		if err = av.processUpdateBucket(location.Name()); err != nil {
			return err
		}
	}

	// create compressed header; the intermediate step is needed as we
	// can not create tar archive containing files compressed on the fly
	if err = av.createCompressedHeader(entries); err != nil {
		return err
	}

	if err = av.createArtifact(entries); err != nil {
		return err
	}

	// remove header which copy is now part of artifact
	if err = av.removeCompressedHeader(); err != nil {
		return err
	}

	return nil
}
