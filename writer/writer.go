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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/log"
)

type MetadataWriter struct {
	updateLocation  string
	headerStructure metadata.MetadataArtifactHeader
	format          string
	version         int
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

func (mv MetadataWriter) generateChecksums(updateDir string, updates []os.FileInfo) (map[string]string, error) {
	updateHashes := make(map[string]string, 1)
	for _, update := range updates {
		updateFile := filepath.Join(updateDir, update.Name())
		f, err := os.Open(updateFile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return nil, err
		}
		log.Debugf("hash of file: %v (%x)\n", updateFile, h.Sum(nil))
		updateHashes[update.Name()] = hex.EncodeToString(h.Sum(nil))
	}
	return updateHashes, nil
}

func (mv MetadataWriter) writeChecksums(updateDir string, checksums map[string]string) error {
	// first check if `checksums` directory exists
	checksumsDir := filepath.Join(updateDir, "checksums")
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

	for file, checksum := range checksums {
		fileName := strings.TrimSuffix(file, filepath.Ext(file)) + ".sha256sum"
		if err :=
			ioutil.WriteFile(filepath.Join(checksumsDir, fileName), []byte(checksum), os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

func (mv MetadataWriter) writeFiles(updateDir string, updates map[string]string) error {
	files := metadata.MetadataFiles{}
	for file := range updates {
		files.Files = append(files.Files, metadata.MetadataFile{File: file})
	}

	if err := files.Validate(); err != nil {
		return err
	}

	data, err := getJSON(files)
	if err != nil {
		return err
	}

	if err :=
		ioutil.WriteFile(filepath.Join(updateDir, "files"), data, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func (mv MetadataWriter) moveData(source string, update string) error {
	destination := filepath.Join(mv.updateLocation, "data", update)

	if err := os.MkdirAll(destination, os.ModeDir|os.ModePerm); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
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

func (mv MetadataWriter) moveHeaders(updates []os.FileInfo) error {
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

func (mv MetadataWriter) Write() error {

	// get directories list containing updates
	entries, err := ioutil.ReadDir(mv.updateLocation)
	if err != nil {
		return err
	}

	// iterate through all directories containing updates
	for _, location := range entries {
		// check files and directories consistency
		updateDir := filepath.Join(mv.updateLocation, location.Name())
		err := mv.headerStructure.CheckHeaderStructure(updateDir)
		if err != nil {
			return err
		}

		// get list of update files
		// at this point we know that `data` exists and contains update(s)
		updatesLocation := filepath.Join(updateDir, "data")
		updates, err := ioutil.ReadDir(updatesLocation)
		if err != nil {
			return err
		}

		// generate `checksums` directory and needed checksums
		checksums, err := mv.generateChecksums(updatesLocation, updates)
		if err != nil {
			return err
		}
		if err = mv.writeChecksums(updateDir, checksums); err != nil {
			return err
		}

		// generate signatures

		// generate `files` file
		if err = mv.writeFiles(updateDir, checksums); err != nil {
			return err
		}

		// move (and compress) updates from `data` to `../data/location.zip`
		if err = mv.moveData(updatesLocation, location.Name()); err != nil {
			return err
		}
	}

	// generate header info
	if err = mv.writeHeaderInfo(entries); err != nil {
		return err
	}

	// move (and compress) header
	if err = mv.moveHeaders(entries); err != nil {
		return err
	}

	// generate `info`
	if err = mv.writeInfo(); err != nil {
		return err
	}

	// (compress all)

	return nil
}
