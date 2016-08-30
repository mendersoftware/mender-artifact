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
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/log"
)

type update struct {
	name         string
	path         string
	updateBucket string
	info         os.FileInfo
	checksum     string
}

type updates []update

type updateBacket struct {
	location string
	path     string
	updates  updates
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
	json, err := json.Marshal(data)
	if err != nil || json == nil {
		return nil, err
	}
	return json, nil
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
	upd.checksum = hex.EncodeToString(h.Sum(nil))
	return nil
}

func (mv MetadataWriter) writeChecksums(updates updateBacket) error {
	// first check if `checksums` directory exists
	checksumsDir := filepath.Join(updates.path, "checksums")
	if _, err := os.Stat(checksumsDir); os.IsNotExist(err) {
		// do nothing here; create directlry later
	} else {
		if err := os.RemoveAll(checksumsDir); err != nil {
			return err
		}
	}

	if err := os.Mkdir(checksumsDir, os.ModeDir|os.ModePerm); err != nil {
		return err
	}

	for _, update := range updates.updates {
		fileName := strings.TrimSuffix(update.name, filepath.Ext(update.name)) + ".sha256sum"
		if len(update.checksum) == 0 {
			log.Errorf("blah: %v\n", update)
			return errors.New("blah")
		}
		if err :=
			ioutil.WriteFile(filepath.Join(checksumsDir, fileName), []byte(update.checksum), os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

func (mv MetadataWriter) writeFiles(updates updateBacket) error {
	if err := updates.files.Validate(); err != nil {
		return err
	}

	data, err := getJSON(updates.files)
	if err != nil {
		return err
	}

	if err :=
		ioutil.WriteFile(filepath.Join(updates.path, "files"), data, os.ModePerm); err != nil {
		return err
	}
	return nil
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

func (mv MetadataWriter) writeHeaderInfo(updates []os.FileInfo) error {
	// for now we have ONLY one type of update - rootfs-image
	headerInfo := metadata.MetadataHeaderInfo{}

	if err := os.MkdirAll(filepath.Join(mv.updateLocation, "header"), os.ModeDir|os.ModePerm); err != nil {
		return err
	}

	// TODO: should we store update name as well?
	for _ = range updates {
		headerInfo.Updates = append(headerInfo.Updates, metadata.MetadataUpdateType{Type: "rootfs-image"})
	}

	if err := headerInfo.Validate(); err != nil {
		return err
	}

	data, err := getJSON(headerInfo)
	if err != nil {
		return err
	}

	if err :=
		ioutil.WriteFile(filepath.Join(mv.updateLocation, "header", "header-info"), data, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func (mv MetadataWriter) moveAndCompressHeaders(updates []os.FileInfo) error {
	destination := filepath.Join(mv.updateLocation, "header", "headers")

	if err := os.MkdirAll(destination, os.ModeDir|os.ModePerm); err != nil {
		return err
	}

	for _, update := range updates {
		if err := os.Rename(filepath.Join(mv.updateLocation, update.Name()), filepath.Join(destination, update.Name())); err != nil {
			return err
		}
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

	// generate `files` file
	if err = mv.writeFiles(updBucket); err != nil {
		return err
	}

	if err = mv.writeChecksums(updBucket); err != nil {
		return err
	}

	// move (and compress) updates from `data` to `../data/location.tar.gz`
	if err = mv.moveAndCompressData(updBucket); err != nil {
		return err
	}

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

	// generate header info
	if err = mv.writeHeaderInfo(entries); err != nil {
		return err
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

	return nil
}
