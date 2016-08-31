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

// NewArtifactsWriter creates a new ArtifactsWriter providing a location
// of Mender metadata artifacts, format of the update and version.
func NewArtifactsWriter(path string, format string, version int) *ArtifactsWriter {
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
	checksum     []byte
}

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

	checksum := h.Sum(nil)
	upd.checksum = make([]byte, hex.EncodedLen(len(checksum)))
	hex.Encode(upd.checksum, h.Sum(nil))
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

func (av *ArtifactsWriter) writeArchive(destination io.WriteCloser, content []ReadArchiver, compressed bool) error {
	if len(content) == 0 {
		return errors.New("artifacts writer: empty content")
	}

	var tw *tar.Writer
	if compressed {
		// start with something simple for now
		gz := gzip.NewWriter(destination)
		//defer gz.Close()
		tw = tar.NewWriter(gz)
	} else {
		tw = tar.NewWriter(destination)
	}

	for _, arch := range content {
		// TODO:
		v := reflect.ValueOf(arch)
		if v.IsNil() {
			log.Errorf("artifacts writer: broken entry %v", v)
			return errors.New("artifacts writer: invalid archiver entry")
		}
		defer arch.Close()
		hdr, err := arch.GetHeader()
		if err != nil || hdr == nil {
			tw.Close()
			return errors.New("artifacts writer: broken or empty header")
		}

		if err := tw.WriteHeader(hdr); err != nil {
			tw.Close()
			return err
		}
		// on the fly copy
		if _, err := io.Copy(tw, arch); err != nil {
			tw.Close()
			return err
		}
	}
	// make sure to check the error on Close
	if err := tw.Close(); err != nil {
		return err
	}
	return nil
}

func (av *ArtifactsWriter) archiveData(updates *updateBucket) error {
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

	// we need to ensure correct ordering of files
	var dataContent []ReadArchiver

	for _, update := range updates.updateArtifacts {
		dataContent = append(dataContent,
			NewFileArchiver(update.path, update.info.Name()))
	}
	updates.archivedPath = dataArchive.Name()
	return av.writeArchive(dataArchive, dataContent, true)
}

func (av ArtifactsWriter) archiveHeader(updates []os.FileInfo) error {
	archive, err := os.Create(filepath.Join(av.updateLocation, "header.tar.gz"))
	if err != nil {
		return err
	}
	defer archive.Close()

	// we need to ensure correct ordering of files
	var hCnt []ReadArchiver
	// header-info
	sr := NewJSONStreamArchiver(av.getHeaderInfo(updates), "header-info")
	hCnt = append(hCnt, sr)

	for _, update := range updates {
		bucket, ok := av.updates[update.Name()]

		if !ok {
			return errors.New("artifacts writer: invalid update bucket")
		}
		updateTarLocation := filepath.Join("headers", update.Name())

		// files
		hCnt = append(hCnt, NewJSONStreamArchiver(bucket.files,
			filepath.Join(updateTarLocation, "files")))
		// type-info
		hCnt = append(hCnt, NewFileArchiver(filepath.Join(bucket.path, "type-info"),
			filepath.Join(updateTarLocation, "type-info")))
		// meta-data
		hCnt = append(hCnt, NewFileArchiver(filepath.Join(bucket.path, "meta-data"),
			filepath.Join(updateTarLocation, "meta-data")))
		// checksums
		for _, upd := range bucket.updateArtifacts {
			fileName := strings.TrimSuffix(upd.name, filepath.Ext(upd.name)) + ".sha256sum"
			hCnt = append(hCnt, NewStreamArchiver(upd.checksum,
				filepath.Join(updateTarLocation, "checksums", fileName)))
		}
		// signatures
		for _, upd := range bucket.updateArtifacts {
			fileName := strings.TrimSuffix(upd.name, filepath.Ext(upd.name)) + ".sig"
			fr := NewFileArchiver(filepath.Join(bucket.path, "signatures", fileName),
				filepath.Join(updateTarLocation, "signatures", fileName))
			hCnt = append(hCnt, fr)
		}
		// TODO: scripts
		//tarContent = append(tarContent, NewPlainFile(filepath.Join(bucket.path, "scripts"), filepath.Join("headers", update.Name(), "scripts")))
	}
	return av.writeArchive(archive, hCnt, true)
}

func (av *ArtifactsWriter) removeCompressedHeader() error {
	// remove temporary header file
	return os.Remove(filepath.Join(av.updateLocation, "header.tar.gz"))
}

func (av *ArtifactsWriter) createArtifact(files []os.FileInfo) error {
	artifact, err := os.Create(filepath.Join(av.updateLocation, "artifact.mender"))
	if err != nil {
		return err
	}
	defer artifact.Close()

	// we need to ensure correct ordering of files
	var artifactContent []ReadArchiver

	aInfo := NewJSONStreamArchiver(av.getInfo(), "info")
	artifactContent = append(artifactContent, aInfo)
	aHdr := NewFileArchiver(filepath.Join(av.updateLocation, "header.tar.gz"), "header.tar.gz")
	artifactContent = append(artifactContent, aHdr)

	for _, artifact := range files {
		bucket, ok := av.updates[artifact.Name()]

		if !ok {
			return errors.New("artifacts writer: can not find data file")
		}
		aData := NewFileArchiver(bucket.archivedPath, "data/0000.tar.gz")
		artifactContent = append(artifactContent, aData)
	}
	return av.writeArchive(artifact, artifactContent, false)
}

func (av *ArtifactsWriter) storeFile(bucket *updateBucket, upd updateArtifact) error {
	bucket.files.Files =
		append(bucket.files.Files, metadata.MetadataFile{File: upd.name})
	return nil
}

func (av *ArtifactsWriter) processUpdateBucket(bucket string) error {
	// get list of update files
	// at this point we know that `data` exists and contains update(s)
	bucketLocation := filepath.Join(av.updateLocation, bucket)
	dataLocation := filepath.Join(av.updateLocation, bucket, "data")

	updateFiles, err := ioutil.ReadDir(dataLocation)
	if err != nil {
		return err
	}

	updBucket := updateBucket{}
	// iterate through all data files
	for _, file := range updateFiles {
		upd := updateArtifact{
			name:         file.Name(),
			path:         filepath.Join(dataLocation, file.Name()),
			updateBucket: bucket,
			info:         file,
		}
		// generate checksums
		err = av.calculateChecksum(&upd)
		if err != nil {
			return err
		}
		// TODO: generate signatures

		// store `file` data
		if err = av.storeFile(&updBucket, upd); err != nil {
			return err
		}
		updBucket.updateArtifacts = append(updBucket.updateArtifacts, upd)
		updBucket.location = bucket
		updBucket.path = bucketLocation
	}

	// move (and compress) updates from `data` to `../data/location.tar.gz`
	if err = av.archiveData(&updBucket); err != nil {
		return err
	}

	av.updates[bucket] = updBucket
	return nil
}

// Write writes Mender artifacts metadata compressed archive
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
	if err = av.archiveHeader(entries); err != nil {
		return err
	}
	// crate whole artifacts file
	if err = av.createArtifact(entries); err != nil {
		return err
	}
	// remove header which copy is now part of artifact
	if err = av.removeCompressedHeader(); err != nil {
		return err
	}
	return nil
}
