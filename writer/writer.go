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
	"io"
	"io/ioutil"
	"os"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
)

// Writer provides on the fly writing of artifacts metadata file used by
// the Mender client and the server.
type Writer struct {
	w      io.Writer // underlying writer
	signed bool      // determine if artifact should be signed or no
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,
	}
}

func NewWriterSigned(w io.Writer) *Writer {
	return &Writer{
		w:      w,
		signed: true,
	}
}

func (aw *Writer) WriteArtifact(format string, version int,
	devices []string, name string, upd *artifact.Updates) error {

	f, ferr := ioutil.TempFile("", "header")
	if ferr != nil {
		return errors.New("writer: can not create temporary header file")
	}
	defer os.Remove(f.Name())

	// calculate checksums of all data files
	// we need this regardless of which artifact version we are writing
	checksums := make(map[string]([]byte), 1)

	for _, u := range upd.U {

		for _, f := range u.GetUpdateFiles() {
			ch := artifact.NewWriterChecksum(ioutil.Discard)
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
	}

	// write temporary header (we need to know the size before storing in tar)
	if hChecksum, err := func() (*artifact.Checksum, error) {
		ch := artifact.NewWriterChecksum(f)
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
	inf := artifact.ToStream(&artifact.Info{Version: version, Format: format})

	var ch io.Writer
	// only calculate version checksum if artifact must be signed
	if aw.signed {
		ch = artifact.NewWriterChecksum(tw)
	} else {
		ch = tw
	}
	sa := artifact.NewWriterStream(tw)
	if err := sa.WriteHeader(inf, "version"); err != nil {
		return errors.Wrapf(err, "writer: can not write version tar header")
	}

	if n, err := ch.Write(inf); err != nil || n != len(inf) {
		return errors.New("writer: can not tar version")
	}

	if aw.signed {
		checksums["version"] = ch.(*artifact.Checksum).Checksum()
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
		fw := artifact.NewWriterFile(tw)
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

	hinf := artifact.ToStream(hInfo)
	sa := artifact.NewWriterStream(tw)
	if err := sa.WriteHeader(hinf, "header-info"); err != nil {
		return errors.Wrapf(err, "writer: can not tar header-info")
	}
	if n, err := sa.Write(hinf); err != nil || n != len(hinf) {
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
