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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"syscall"

	"github.com/mendersoftware/mender-artifact/archiver"
	"github.com/mendersoftware/mender-artifact/metadata"
	"github.com/mendersoftware/mender-artifact/update"
	"github.com/pkg/errors"
)

// Writer provides on the fly writing of artifacts metadata file used by
// Mender client and server.
// Call Write to start writing artifacts file.
type Writer struct {
	format            string
	version           int
	compatibleDevices []string
	artifactName      string

	// determine if artifact should be signed or not
	signed bool

	w io.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{
		format:  "mender",
		version: 1,
		w:       w,
	}
}

func NewWriterSigned(w io.Writer) *Writer {
	return &Writer{
		format:  "mender",
		version: 1,
		w:       w,
		signed:  true,
	}
}

type Checksum struct {
	w io.Writer // underlying writer
	r io.Reader
	h hash.Hash
	c []byte // checksum
}

func NewWriterChecksum(w io.Writer) *Checksum {
	h := sha256.New()
	return &Checksum{
		w: io.MultiWriter(h, w),
		h: h,
	}
}

func NewReaderChecksum(r io.Reader) *Checksum {
	h := sha256.New()
	return &Checksum{
		r: io.TeeReader(r, h),
		h: h,
	}
}

func (c *Checksum) Write(p []byte) (int, error) {
	if c.w == nil {
		return 0, syscall.EBADF
	}
	return c.w.Write(p)
}

func (c *Checksum) Read(p []byte) (int, error) {
	if c.r == nil {
		return 0, syscall.EBADF
	}
	return c.r.Read(p)
}

func (c *Checksum) Checksum() []byte {
	sum := c.h.Sum(nil)
	checksum := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(checksum, c.h.Sum(nil))
	return checksum
}

func (aw *Writer) WriteArtifact(format string, version int,
	devices []string, name string, upd *update.Updates) error {

	f, ferr := ioutil.TempFile("", "header")
	if ferr != nil {
		return errors.New("writer: can not create temporary header file")
	}
	defer os.Remove(f.Name())

	// calculate checksums of all data files
	// we need this regardless of which artifact version we are writing
	checksums := make(map[string]([]byte), 1)

	upd.Reset()
	for {
		u, err := upd.Next()
		if err == io.EOF {
			break
		}

		for _, f := range u.GetUpdateFiles() {
			ch := NewWriterChecksum(ioutil.Discard)
			df, err := os.Open(f.Name)
			if err != nil {
				return errors.Wrapf(err, "writer: can not open data file: %v", f)
			}
			if _, err := io.Copy(ch, df); err != nil {
				return errors.Wrapf(err, "writer: can not calculate checksum: %v", f)
			}
			f.Checksum = ch.Checksum()

			// TODO:
			checksums[f.Name] = ch.Checksum()
		}
		fmt.Printf("blah: %v\n\n", err)
	}

	// write temporary header (we need to know the size before storing in tar)
	if hChecksum, err := func() (*Checksum, error) {
		ch := NewWriterChecksum(f)
		gz := gzip.NewWriter(ch)
		defer gz.Close()

		tw := tar.NewWriter(gz)
		defer tw.Close()

		if err := aw.writeHeader(tw, devices, name, upd); err != nil {
			return nil, errors.Wrapf(err, "writer: error writing header")
		}
		return ch, nil
	}(); err != nil {
		return err
	} else if aw.signed {
		checksums["header.tar.gz"] = hChecksum.Checksum()
	}

	// mender archive writer
	tw := tar.NewWriter(aw.w)
	defer tw.Close()

	// write version file
	inf := archiver.ToStream(&metadata.Info{Version: version, Format: format})

	var ch io.Writer
	// only calculate version checksum if artifact must be signed
	if aw.signed {
		ch = NewWriterChecksum(tw)
	} else {
		ch = tw
	}
	sa := archiver.NewWriterStream(tw)
	if err := sa.WriteHeader(inf, "version"); err != nil {
		return errors.Wrapf(err, "writer: can not write version tar header")
	}

	if n, err := ch.Write(inf); err != nil || n != len(inf) {
		return errors.New("writer: can not tar version")
	}

	if aw.signed {
		checksums["version"] = ch.(*Checksum).Checksum()
	}

	switch version {
	case 2:
		// write checksums

		if aw.signed {
			// write signature

		}
		fallthrough

	case 1:
		// write header
		fw := archiver.NewWriterFile(tw)
		if err := fw.WriteHeader(f.Name(), "header.tar.gz"); err != nil {
			return errors.Wrapf(err, "writer: can not tar header")
		}

		if _, err := f.Seek(0, 0); err != nil {
			return errors.Wrapf(err, "writer: error opening tmp header for reading")
		}

		if _, err := io.Copy(fw, f); err != nil {
			return errors.Wrapf(err, "writer: can not tar header")
		}

	default:
		return errors.New("writer: unsupported artifact version")
	}

	// write data files
	if err := aw.writeData(tw, upd); err != nil {
		return err
	}

	return nil
}

// func (av *Writer) write(updates []parser.UpdateData) error {
// 	av.availableUpdates = updates
//
// 	// write temporary header (we need to know the size before storing in tar)
// 	if err := av.WriteHeader(); err != nil {
// 		return err
// 	}
//
// 	// archive info
// 	info := av.getInfo()
// 	ia := archiver.NewMetadataArchiver(&info, "version")
// 	if err := ia.Archive(av.aArchiver); err != nil {
// 		return errors.Wrapf(err, "writer: error archiving info")
// 	}
//
// 	// archive signatures
// 	if av.signed {
//
// 	}
//
// 	// archive header
// 	ha := archiver.NewFileArchiver(av.hTmpFile.Name(), "header.tar.gz")
// 	if err := ha.Archive(av.aArchiver); err != nil {
// 		return errors.Wrapf(err, "writer: error archiving header")
// 	}
// 	// archive data
// 	if err := av.WriteData(); err != nil {
// 		return err
// 	}
// 	// we've been storing everything in temporary file
// 	if err := av.aArchiver.Close(); err != nil {
// 		return errors.New("writer: error closing archive")
// 	}
// 	// prevent from closing archiver twice
// 	av.aArchiver = nil
//
// 	if err := av.aTmpFile.Close(); err != nil {
// 		return errors.New("writer: error closing archive temporary file")
// 	}
// 	return os.Rename(av.aTmpFile.Name(), av.aName)
// }
//
// func (av *Writer) Write(updateDir, atrifactName string) error {
// 	if err := av.init(atrifactName); err != nil {
// 		return err
// 	}
// 	defer av.deinit()
//
// 	updates, err := av.ScanUpdateDirs(updateDir)
// 	if err != nil {
// 		return err
// 	}
// 	return av.write(updates)
// }
//
// func (av *Writer) WriteKnown(updates []parser.UpdateData, atrifactName string) error {
// 	if err := av.init(atrifactName); err != nil {
// 		return err
// 	}
// 	defer av.deinit()
//
// 	return av.write(updates)
// }

// // This reads `type-info` file in provided directory location.
// func getTypeInfo(dir string) (*metadata.TypeInfo, error) {
// 	iPath := filepath.Join(dir, "type-info")
// 	f, err := os.Open(iPath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer f.Close()
//
// 	info := new(metadata.TypeInfo)
// 	_, err = io.Copy(info, f)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	if err = info.Validate(); err != nil {
// 		return nil, err
// 	}
// 	return info, nil
// }
//
// func getDataFiles(dir string) ([]string, error) {
// 	info, err := os.Stat(dir)
// 	if os.IsNotExist(err) {
// 		// we have no data file(s) associated with given header
// 		return nil, nil
// 	} else if err != nil {
// 		return nil, errors.Wrapf(err, "writer: error reading data directory")
// 	}
// 	if info.IsDir() {
// 		updFiles, err := ioutil.ReadDir(dir)
// 		if err != nil {
// 			return nil, err
// 		}
// 		var updates []string
// 		for _, f := range updFiles {
// 			updates = append(updates, filepath.Join(dir, f.Name()))
// 		}
// 		return updates, nil
// 	}
// 	return nil, errors.New("writer: broken data directory")
// }
//
// func (av *Writer) readDirContent(dir, cur string) (*parser.UpdateData, error) {
// 	tInfo, err := getTypeInfo(filepath.Join(dir, cur))
// 	if err != nil {
// 		return nil, os.ErrInvalid
// 	}
// 	p, err := av.ParseManager.GetRegistered(tInfo.Type)
// 	if err != nil {
// 		return nil, errors.Wrapf(err, "writer: error finding parser for [%v]", tInfo.Type)
// 	}
//
// 	data, err := getDataFiles(filepath.Join(dir, cur, "data"))
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	upd := parser.UpdateData{
// 		Path:      filepath.Join(dir, cur),
// 		DataFiles: data,
// 		Type:      tInfo.Type,
// 		P:         p,
// 	}
// 	return &upd, nil
// }
//
// func (av *Writer) ScanUpdateDirs(dir string) ([]parser.UpdateData, error) {
// 	// first check  if we have update in current directory
// 	upd, err := av.readDirContent(dir, "")
// 	if err == nil {
// 		return []parser.UpdateData{*upd}, nil
// 	} else if err != os.ErrInvalid {
// 		return nil, err
// 	}
//
// 	dirs, err := ioutil.ReadDir(dir)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	updates := make([]parser.UpdateData, 0, len(dirs))
// 	for _, uDir := range dirs {
// 		if uDir.IsDir() {
// 			upd, err := av.readDirContent(dir, uDir.Name())
// 			if err == os.ErrInvalid {
// 				continue
// 			} else if err != nil {
// 				return nil, err
// 			}
// 			updates = append(updates, *upd)
// 		}
// 	}
//
// 	if len(updates) == 0 {
// 		return nil, errors.New("writer: no update data detected")
// 	}
// 	return updates, nil
// }

func (aw *Writer) writeHeader(tw *tar.Writer, devices []string, name string,
	updates *update.Updates) error {
	// store header info
	hInfo := new(metadata.HeaderInfo)

	updates.Reset()
	for {
		upd, err := updates.Next()
		if err == io.EOF {
			break
		}
		hInfo.Updates =
			append(hInfo.Updates, metadata.UpdateType{Type: upd.GetType()})
	}
	hInfo.CompatibleDevices = devices
	hInfo.ArtifactName = name

	hinf := archiver.ToStream(hInfo)
	sa := archiver.NewWriterStream(tw)
	if err := sa.WriteHeader(hinf, "header-info"); err != nil {
		return errors.Wrapf(err, "writer: can not tar header-info")
	}
	if n, err := sa.Write(hinf); err != nil || n != len(hinf) {
		return errors.New("writer: can not store header-info")
	}

	updates.Reset()
	for {
		upd, err := updates.Next()
		if err == io.EOF {
			break
		}
		if err := upd.ComposeHeader(tw); err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return nil
}

func (aw *Writer) writeData(tw *tar.Writer, updates *update.Updates) error {
	updates.Reset()
	for {
		upd, err := updates.Next()
		if err == io.EOF {
			break
		}
		if err := upd.ComposeData(tw); err != nil {
			return errors.Wrapf(err, "writer: error writing data files")
		}
	}
	return nil
}
