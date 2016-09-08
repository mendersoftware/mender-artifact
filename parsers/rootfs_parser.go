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

package parsers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/mendersoftware/log"
	"github.com/pkg/errors"
)

type rootfsFile struct {
	name      string
	path      string
	size      int64
	date      time.Time
	checksum  []byte
	signature []byte
}

type RootfsParser struct {
	files    metadata.Files
	tInfo    metadata.TypeInfo
	metadata metadata.Metadata
	updates  map[string]rootfsFile
	order    string

	sStore string
	dStore io.Writer
}

func NewRootfsParser(sStoreDir string, w io.Writer) RootfsParser {
	return RootfsParser{
		sStore:  sStoreDir,
		dStore:  w,
		updates: map[string]rootfsFile{}}
}

func (rp *RootfsParser) GetOrder() string {
	return rp.order
}

func (rp *RootfsParser) SetOrder(ord string) error {
	rp.order = ord
	return nil
}

func (rp RootfsParser) NeedsDataFile() bool {
	return true
}

func withoutExt(name string) string {
	bName := filepath.Base(name)
	return strings.TrimSuffix(bName, filepath.Ext(bName))
}

func (rp *RootfsParser) ArchiveData(tw *tar.Writer, srcDir, dst string) error {
	f, err := os.Create("/tmp/my_data.tar.gz")
	if err != nil {
		return errors.Wrapf(err, "parser: can not create tmp data file")
	}
	//defer os.Remove("/tmp/my_data.tar.gz")
	gz := gzip.NewWriter(f)
	defer gz.Close()
	dtw := tar.NewWriter(gz)
	defer tw.Close()

	for _, data := range rp.updates {
		log.Infof("processing data file: %v [%v]", data.path, data.name)
		a := metadata.NewFileArchiver(data.path, data.name)
		if err := a.Archive(dtw); err != nil {
			return err
		}
	}
	//TODO
	dtw.Close()
	gz.Close()
	f.Close()

	a := metadata.NewFileArchiver(f.Name(), dst)
	if err := a.Archive(tw); err != nil {
		return err
	}

	return nil
}

func archiveFiles(tw *tar.Writer, upd []os.FileInfo, dir string) error {
	files := new(metadata.Files)
	for _, u := range upd {
		files.File = append(files.File, filepath.Base(u.Name()))
	}
	a := metadata.NewJSONStreamArchiver(files, filepath.Join(dir, "files"))
	return a.Archive(tw)
}

func calcChecksum(file string) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil,
			errors.Wrapf(err, "can not open file for calculating checksum")
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, errors.Wrapf(err, "error calculating checksum")
	}

	sum := h.Sum(nil)
	checksum := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(checksum, h.Sum(nil))
	log.Infof("hash of file: %v (%x)\n", file, checksum)
	return checksum, nil
}

func archiveChecksums(tw *tar.Writer, upd []os.FileInfo, src, dir string) error {
	for _, u := range upd {
		log.Infof("calculating checksum for: %v", u.Name())
		sum, err := calcChecksum(filepath.Join(src, u.Name()))
		if err != nil {
			return err
		}
		log.Infof("checksum for: %v [%v]", u.Name(), string(sum))
		a := metadata.NewStreamArchiver(sum, filepath.Join(dir, withoutExt(u.Name())+".sha256sum"))
		if err := a.Archive(tw); err != nil {
			return errors.Wrapf(err, "reader: error storing checksum")
		}
	}
	return nil
}

func (rp *RootfsParser) ArchiveHeader(tw *tar.Writer, srcDir, dstDir string) error {
	log.Infof("processing header: %v [%v]", srcDir, dstDir)
	if err := hFormatPreWrite.CheckHeaderStructure(srcDir); err != nil {
		return err
	}

	// here we should get list of all update files which are
	// part of current update
	updFiles, err := ioutil.ReadDir(filepath.Join(srcDir, "data"))
	if err != nil {
		return err
	}

	// store data files
	for _, f := range updFiles {
		rp.updates[withoutExt(f.Name())] =
			rootfsFile{
				name: f.Name(),
				path: filepath.Join(srcDir, "data", f.Name()),
			}
	}

	log.Infof("update files: %+v", updFiles)

	//TODO: use stored data
	if err = archiveFiles(tw, updFiles, dstDir); err != nil {
		return errors.Wrapf(err, "parser: can not store files")
	}

	a := metadata.NewFileArchiver(filepath.Join(srcDir, "type-info"),
		filepath.Join(dstDir, "type-info"))
	if err := a.Archive(tw); err != nil {
		return errors.Wrapf(err, "parser: can not store type-info")
	}

	a = metadata.NewFileArchiver(filepath.Join(srcDir, "meta-data"),
		filepath.Join(dstDir, "meta-data"))
	if err := a.Archive(tw); err != nil {
		return errors.Wrapf(err, "parser: can not store meta-data")
	}

	if err := archiveChecksums(tw, updFiles,
		filepath.Join(srcDir, "data"),
		filepath.Join(dstDir, "checksums")); err != nil {
		return err
	}

	//TODO: get rid of bad Joins
	for _, u := range updFiles {
		a = metadata.NewFileArchiver(filepath.Join(srcDir, "signatures", withoutExt(u.Name())+".sig"),
			filepath.Join(dstDir, "signatures", withoutExt(u.Name())+".sig"))
		if err := a.Archive(tw); err != nil {
			return errors.Wrapf(err, "parser: can not store signatures")
		}
	}

	//TODO: scripts

	return nil
}

func (rp *RootfsParser) ParseHeader(tr *tar.Reader, hPath string) error {
	// iterate through tar archive untill some error occurs or we will
	log.Info("processing rootfs image header")

	if tr == nil {
		return errors.New("rootfs updater: uninitialized tar reader")
	}
	// reach end of archive
	for i := 0; ; i++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			// we have reached end of archive
			log.Debug("rootfs updater: reached end of archive")
			return nil
		}

		relPath, err := filepath.Rel(hPath, hdr.Name)
		if err != nil {
			return err
		}

		log.Infof("processing rootfs image header file: %v %v", hdr.Name, hdr.Linkname)

		switch {
		case i == 0 && strings.Compare(relPath, "files") == 0:

			if _, err = io.Copy(&rp.files, tr); err != nil {
				return errors.Wrapf(err, "rootfs updater: error reading files")
			}
			for _, file := range rp.files.File {
				rp.updates[withoutExt(file)] = rootfsFile{name: file}
			}
		case i == 1 && strings.Compare(relPath, "type-info") == 0:
			// we can skip this one for now
		case i == 2 && strings.Compare(relPath, "meta-data") == 0:
			if _, err = io.Copy(&rp.metadata, tr); err != nil {
				return errors.Wrapf(err, "rootfs updater: error reading metadata")
			}
		case strings.HasPrefix(relPath, "checksums"):
			update, ok := rp.updates[withoutExt(hdr.Name)]
			if !ok {
				return errors.New("rootfs updater: found signature for non existing update file")
			}
			buf := bytes.NewBuffer(nil)
			if _, err = io.Copy(buf, tr); err != nil {
				return errors.Wrapf(err, "rootfs updater: error reading checksum")
			}
			update.checksum = buf.Bytes()
			rp.updates[withoutExt(hdr.Name)] = update
		case strings.HasPrefix(relPath, "signatures"):
			//TODO:
		case strings.HasPrefix(relPath, "scripts"):
			//TODO
		default:
			log.Errorf("rootfs updater: found unsupported element: %v", relPath)
			return errors.New("rootfs updater: unsupported element")
		}
	}
}

// data files are stored in tar.gz format
func (rp *RootfsParser) ParseData(r io.Reader) error {
	if r == nil {
		return errors.New("rootfs updater: uninitialized tar reader")
	}
	//[data.tar].gz
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	//data[.tar].gz
	tar := tar.NewReader(gz)
	// iterate over the files in tar archive
	for {
		hdr, err := tar.Next()
		if err == io.EOF {
			// once we reach end of archive break the loop
			break
		}
		log.Infof("processing data file: %v", hdr.Name)
		fh, ok := rp.updates[withoutExt(hdr.Name)]
		if !ok {
			return errors.New("rootfs updater: can not find header info for data file")
		}

		// for calculating hash
		h := sha256.New()

		var w io.Writer
		// if we don't want to store the file just calculate checksum
		if rp.dStore == nil {
			log.Info("skipping storing data file as write location is empty")
			w = h
		} else {
			w = io.MultiWriter(h, rp.dStore)
		}

		if _, err := io.Copy(w, tar); err != nil {
			return err
		}
		sum := h.Sum(nil)
		hSum := make([]byte, hex.EncodedLen(len(sum)))
		hex.Encode(hSum, h.Sum(nil))

		log.Infof("hash of file: %v (%v)", string(hSum), string(fh.checksum))
		if bytes.Compare(hSum, fh.checksum) != 0 {
			return errors.New("rootfs updater: invalid data file checksum")
		}

		fh.date = hdr.ModTime
		fh.size = hdr.Size
		rp.updates[withoutExt(hdr.Name)] = fh
	}
	return nil
}

var hFormatPreWrite = metadata.ArtifactHeader{
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
