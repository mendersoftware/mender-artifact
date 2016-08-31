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
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/log"
)

type updateChecksum string

func (uc updateChecksum) Validate() error { return nil }

type update struct {
	name         string
	path         string
	updateBucket string
	info         os.FileInfo
	checksum     updateChecksum
}

type updates []update

type updateBacket struct {
	location string
	path     string
	updates  updates
	info     os.FileInfo
	files    metadata.MetadataFiles
}

type MetadataWriter struct {
	updateLocation  string
	headerStructure metadata.MetadataArtifactHeader
	format          string
	version         int
	updates         map[string]updateBacket
}

var MetadataWriterHeaderFormat = map[string]metadata.MetadataDirEntry{
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

var MetadataWriterHeaderFormatAfter = map[string]metadata.MetadataDirEntry{
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

type Validator interface {
	Validate() error
}

func getJSON(data Validator) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	if err := data.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(data)
}

func (mv MetadataWriter) generateChecksum(upd *update) error {
	f, err := os.Open(upd.path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	log.Debugf("hash of file: %v (%x)\n", upd.path, h.Sum(nil))
	upd.checksum = updateChecksum(hex.EncodeToString(h.Sum(nil)))
	return nil
}

type streamTarReader struct {
	name   string
	data   []byte
	buffer *bytes.Buffer
}

func NewStreamTarReader(data Validator, name string) *streamTarReader {
	j, err := getJSON(data)
	if err != nil {
		return nil
	}
	return &streamTarReader{
		name:   name,
		data:   j,
		buffer: bytes.NewBuffer(j),
	}
}

func (str streamTarReader) Read(p []byte) (n int, err error) {
	return str.buffer.Read(p)
}

func (str streamTarReader) Close() error { return nil }

func (str streamTarReader) GetHeader() (*tar.Header, error) {
	hdr := &tar.Header{
		Name: str.name,
		Mode: 0600,
		Size: int64(len(str.data)),
	}
	return hdr, nil
}

func (mv MetadataWriter) moveAndCompressData(updates updateBacket) error {
	destination := filepath.Join(mv.updateLocation, "data")

	if err := os.MkdirAll(destination, os.ModeDir|os.ModePerm); err != nil {
		return err
	}

	archive, err := os.Create(filepath.Join(destination, updates.location+".tar.gz"))
	if err != nil {
		return err
	}
	defer archive.Close()

	// start with something simple for now
	gw := gzip.NewWriter(archive)
	defer gw.Close()

	tw := tar.NewWriter(gw)

	for _, update := range updates.updates {
		// we are happy with relative hdr.Name below
		hdr, err := tar.FileInfoHeader(update.info, update.info.Name())
		if err != nil {
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

	// remove original files
	if err := os.RemoveAll(filepath.Join(mv.updateLocation, updates.location, "data")); err != nil {
		return err
	}

	return nil
}

func (mv MetadataWriter) writeHeaderInfo(updates []os.FileInfo) metadata.MetadataHeaderInfo {
	// for now we have ONLY one type of update - rootfs-image
	headerInfo := metadata.MetadataHeaderInfo{}

	// TODO: should we store update name as well?
	for _ = range updates {
		headerInfo.Updates = append(headerInfo.Updates, metadata.MetadataUpdateType{Type: "rootfs-image"})
	}
	return headerInfo
}

type TarReader interface {
	io.ReadCloser
	GetHeader() (*tar.Header, error)
}

type plainFile struct {
	path string
	name string
	file *os.File
}

func NewPlainFile(path, name string) *plainFile {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	return &plainFile{file: f, name: name, path: path}
}

func (pf plainFile) Read(p []byte) (n int, err error) {
	return pf.file.Read(p)
}

func (pf plainFile) Close() error {
	return pf.file.Close()
}

func (pf plainFile) GetHeader() (*tar.Header, error) {
	info, err := os.Stat(pf.path)
	if err != nil {
		return nil, err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return nil, err
	}
	hdr.Name = pf.name
	return hdr, nil
}

func (mv MetadataWriter) moveAndCompressHeaders(updates []os.FileInfo) error {

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
	var tarContent []TarReader
	sr := NewStreamTarReader(mv.writeHeaderInfo(updates), "header-info")
	tarContent = append(tarContent, sr)

	for _, update := range updates {
		bucket, ok := mv.updates[update.Name()]

		if !ok {
			return errors.New("artifacts writer: ")
		}
		updateTarLocation := filepath.Join("headers", update.Name())
		tarContent = append(tarContent, NewStreamTarReader(bucket.files,
			filepath.Join(updateTarLocation, "files")))
		tarContent = append(tarContent, NewPlainFile(filepath.Join(bucket.path, "type-info"),
			filepath.Join(updateTarLocation, "type-info")))
		tarContent = append(tarContent, NewPlainFile(filepath.Join(bucket.path, "meta-data"),
			filepath.Join(updateTarLocation, "meta-data")))

		for _, upd := range bucket.updates {
			fileName := strings.TrimSuffix(upd.name, filepath.Ext(upd.name)) + ".sha256sum"
			tarContent = append(tarContent, NewStreamTarReader(upd.checksum,
				filepath.Join(updateTarLocation, "checksums", fileName)))
		}
		for _, upd := range bucket.updates {
			fileName := strings.TrimSuffix(upd.name, filepath.Ext(upd.name)) + ".sig"
			fr := NewPlainFile(filepath.Join(bucket.path, "signatures", fileName),
				filepath.Join(updateTarLocation, "signatures", fileName))
			fmt.Printf("should be empty: %v\n", fr)
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

func (mv MetadataWriter) writeInfo() error {
	info := metadata.MetadataInfo{
		Format:  mv.format,
		Version: mv.version,
	}

	data, err := getJSON(info)
	if err != nil {
		return err
	}

	if err :=
		ioutil.WriteFile(filepath.Join(mv.updateLocation, "info"), data, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func (mv *MetadataWriter) getScripts(bucket string) error {
	return nil
}

func (mv *MetadataWriter) processUpdateBucket(bucket string) error {

	// get list of update files
	// at this point we know that `data` exists and contains update(s)
	bucketLocation := filepath.Join(mv.updateLocation, bucket)
	dataLocation := filepath.Join(mv.updateLocation, bucket, "data")

	updateFiles, err := ioutil.ReadDir(dataLocation)
	if err != nil {
		return err
	}
	updBucket := updateBacket{}
	for _, file := range updateFiles {
		upd := update{
			name:         file.Name(),
			path:         filepath.Join(dataLocation, file.Name()),
			updateBucket: bucket,
			info:         file,
		}
		// generate needed checksums
		err = mv.generateChecksum(&upd)
		if err != nil {
			return err
		}

		// generate signatures

		// generate `file` data
		updBucket.files.Files =
			append(updBucket.files.Files, metadata.MetadataFile{File: upd.name})

		updBucket.updates = append(updBucket.updates, upd)
		updBucket.location = bucket
		updBucket.path = bucketLocation
	}

	if err = updBucket.files.Validate(); err != nil {
		return err
	}

	// move (and compress) updates from `data` to `../data/location.tar.gz`
	if err = mv.moveAndCompressData(updBucket); err != nil {
		return err
	}

	mv.updates[bucket] = updBucket

	return nil
}

func (mv *MetadataWriter) createArtifact() error {
	return nil
}

func (mv *MetadataWriter) Write() error {

	// get directories list containing updates
	entries, err := ioutil.ReadDir(mv.updateLocation)
	if err != nil {
		return err
	}

	// iterate through all directories containing updates
	for _, location := range entries {
		// check files and directories consistency
		err = mv.headerStructure.CheckHeaderStructure(
			filepath.Join(mv.updateLocation, location.Name()))
		if err != nil {
			return err
		}

		if err = mv.processUpdateBucket(location.Name()); err != nil {
			return err
		}
	}

	// move (and compress) header
	if err = mv.moveAndCompressHeaders(entries); err != nil {
		return err
	}

	// generate `info`
	if err = mv.writeInfo(); err != nil {
		return err
	}

	// (compress all)
	if err = mv.createArtifact(); err != nil {
		return err
	}

	return nil
}
