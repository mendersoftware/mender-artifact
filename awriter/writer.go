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
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
)

// Writer provides on the fly writing of artifacts metadata file used by
// the Mender client and the server.
type Writer struct {
	w      io.Writer // underlying writer
	signer artifact.Signer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,
	}
}

func NewWriterSigned(w io.Writer, s artifact.Signer) *Writer {
	return &Writer{
		w:      w,
		signer: s,
	}
}

func (aw *Writer) WriteArtifact(format string, version int,
	devices []string, name string, upd *artifact.Updates) error {

	if version == 1 && aw.signer != nil {
		return errors.New("writer: can not create version 1 signed artifact")
	}

	f, ferr := ioutil.TempFile("", "header")
	if ferr != nil {
		return errors.New("writer: can not create temporary header file")
	}
	defer os.Remove(f.Name())

	// mender archive writer
	tw := tar.NewWriter(aw.w)
	defer tw.Close()

	manifestBuf := bytes.NewBuffer(nil)
	mw := artifact.NewWriterManifest(manifestBuf)

	// calculate checksums of all data files
	// we need this regardless of which artifact version we are writing
	for i, u := range upd.U {
		for _, f := range u.GetUpdateFiles() {
			ch := artifact.NewWriterChecksum(ioutil.Discard)
			df, err := os.Open(f.Name)
			if err != nil {
				return errors.Wrapf(err, "writer: can not open data file: %v", f)
			}
			if _, err := io.Copy(ch, df); err != nil {
				return errors.Wrapf(err, "writer: can not calculate checksum: %v", f)
			}
			sum := ch.Checksum()
			f.Checksum = sum

			mw.AddChecksum(filepath.Join(artifact.UpdatePath(i), filepath.Base(f.Name)), sum)
		}
	}

	// write temporary header (we need to know the size before storing in tar)
	if hChecksum, err := func() (*artifact.Checksum, error) {
		ch := artifact.NewWriterChecksum(f)
		gz := gzip.NewWriter(ch)
		defer gz.Close()

		htw := tar.NewWriter(gz)
		defer htw.Close()

		if err := aw.writeHeader(htw, devices, name, upd); err != nil {
			return nil, errors.Wrapf(err, "writer: error writing header")
		}
		return ch, nil
	}(); err != nil {
		return err
	} else if aw.signer != nil {
		mw.AddChecksum("header.tar.gz", hChecksum.Checksum())
	}

	// write version file
	inf := artifact.ToStream(&artifact.Info{Version: version, Format: format})
	sa := artifact.NewTarWriterStream(tw)
	if err := sa.Write(inf, "version"); err != nil {
		return errors.Wrapf(err, "writer: can not write version tar header")
	}

	if aw.signer != nil {
		ch := artifact.NewWriterChecksum(ioutil.Discard)
		ch.Write(inf)
		mw.AddChecksum("version", ch.Checksum())
	}

	switch version {
	case 2:
		// write manifest file
		if err := mw.WriteAll("2.0"); err != nil {
			return errors.Wrapf(err, "writer: can not buffer manifest stream")
		}
		sw := artifact.NewTarWriterStream(tw)
		if err := sw.Write(manifestBuf.Bytes(), "manifest"); err != nil {
			return errors.Wrapf(err, "writer: can not write manifest stream")
		}

		if aw.signer != nil {
			// write signature
			sig, err := aw.signer.Sign(manifestBuf.Bytes())
			if err != nil {
				return errors.Wrap(err, "writer: can not sign artifact")
			}
			sw := artifact.NewTarWriterStream(tw)
			if err := sw.Write(sig, "signature"); err != nil {
				return errors.Wrapf(err, "writer: can not tar signature")
			}

		}
		fallthrough

	case 1:
		// write header
		if _, err := f.Seek(0, 0); err != nil {
			return errors.Wrapf(err, "writer: error opening tmp header for reading")
		}

		fw := artifact.NewTarWriterFile(tw)
		if err := fw.Write(f, "header.tar.gz"); err != nil {
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

func (aw *Writer) writeHeader(tw *tar.Writer, devices []string, name string,
	updates *artifact.Updates) error {
	// store header info
	hInfo := new(artifact.HeaderInfo)

	for _, upd := range updates.U {
		hInfo.Updates =
			append(hInfo.Updates, artifact.UpdateType{Type: upd.GetType()})
	}
	hInfo.CompatibleDevices = devices
	hInfo.ArtifactName = name

	sa := artifact.NewTarWriterStream(tw)
	if err := sa.Write(artifact.ToStream(hInfo), "header-info"); err != nil {
		return errors.New("writer: can not store header-info")
	}

	for i, upd := range updates.U {
		if err := upd.ComposeHeader(tw, i); err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return nil
}

func (aw *Writer) writeData(tw *tar.Writer, updates *artifact.Updates) error {
	for i, upd := range updates.U {
		if err := upd.ComposeData(tw, i); err != nil {
			return errors.Wrapf(err, "writer: error writing data files")
		}
	}
	return nil
}
