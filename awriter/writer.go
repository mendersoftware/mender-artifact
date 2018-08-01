// Copyright 2018 Northern.tech AS
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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/handlers"
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

type Updates struct {
	U []handlers.Composer
}

// Iterate through all data files inside `upd` and calculate checksums.
func calcDataHash(s *artifact.ChecksumStore, upd *Updates) error {
	for i, u := range upd.U {
		for _, f := range u.GetUpdateFiles() {
			ch := artifact.NewWriterChecksum(ioutil.Discard)
			df, err := os.Open(f.Name)
			if err != nil {
				return errors.Wrapf(err, "writer: can not open data file: %v", f)
			}
			defer df.Close()
			if _, err := io.Copy(ch, df); err != nil {
				return errors.Wrapf(err, "writer: can not calculate checksum: %v", f)
			}
			sum := ch.Checksum()
			f.Checksum = sum
			s.Add(filepath.Join(artifact.UpdatePath(i), filepath.Base(f.Name)), sum)
		}
	}
	return nil
}

// writeTempHeader can write both the standard and the augmented header by passing in the appropriate `writeHeaderVersion`
// function. (writeHeader/writeAugmentedHeader)
func writeTempHeader(s *artifact.ChecksumStore, name string, writeHeaderVersion func(tarWriter *tar.Writer, args *WriteArtifactArgs) error, args *WriteArtifactArgs) (*os.File, error) {
	// create temporary header file
	f, err := ioutil.TempFile("", name)
	if err != nil {
		return nil, errors.New("writer: can not create temporary header file")
	}

	ch := artifact.NewWriterChecksum(f)
	// use function to make sure to close gz and tar before
	// calculating checksum
	err = func() error {
		gz := gzip.NewWriter(ch)
		defer gz.Close()

		htw := tar.NewWriter(gz)
		defer htw.Close()

		// Header differs in version 3 from version 1 and 2.
		if err = writeHeaderVersion(htw, args); err != nil {
			return errors.Wrapf(err, "writer: error writing header")
		}
		return nil
	}()

	if err != nil {
		return nil, err
	}
	s.Add(name+".tar.gz", ch.Checksum())

	return f, nil
}

func writeTempAugmentedHeader(s *artifact.ChecksumStore, headerArgs *WriteArtifactArgs) (*os.File, error) {
	// create temporary header file
	f, err := ioutil.TempFile("", "header-augment")
	if err != nil {
		return nil, errors.New("writer: can not create temporary header-augment file")
	}

	ch := artifact.NewWriterChecksum(f)
	// use function to make sure to close gz and tar before
	// calculating checksum
	err = func() error {
		gz := gzip.NewWriter(ch)
		defer gz.Close()

		htw := tar.NewWriter(gz)
		defer htw.Close()

		if err = writeAugmentedHeader(htw, headerArgs); err != nil {
			return errors.Wrapf(err, "writer: error writing header")
		}
		return nil
	}()

	if err != nil {
		return nil, err
	}
	s.Add("header-augment.tar.gz", ch.Checksum())

	return f, nil
}

func WriteSignature(tw *tar.Writer, message []byte,
	signer artifact.Signer) error {
	if signer == nil {
		return nil
	}

	sig, err := signer.Sign(message)
	if err != nil {
		return errors.Wrap(err, "writer: can not sign artifact")
	}
	sw := artifact.NewTarWriterStream(tw)
	if err := sw.Write(sig, "manifest.sig"); err != nil {
		return errors.Wrap(err, "writer: can not tar signature")
	}
	return nil
}

type WriteArtifactArgs struct {
	Format   string
	Version  int
	Devices  []string
	Name     string
	Updates  *Updates
	Scripts  *artifact.Scripts
	Depends  *artifact.ArtifactDepends
	Provides *artifact.ArtifactProvides
}

func (aw *Writer) WriteArtifact(args *WriteArtifactArgs) (err error) {
	if !(args.Version == 1 || args.Version == 2 || args.Version == 3) {
		return errors.New("Unsupported artifact version")
	}

	if args.Version == 1 && aw.signer != nil {
		return errors.New("writer: can not create version 1 signed artifact")
	}

	s := artifact.NewChecksumStore()
	// calculate checksums of all data files
	// we need this regardless of which artifact version we are writing
	if err := calcDataHash(s, args.Updates); err != nil {
		return err
	}
	// mender archive writer
	tw := tar.NewWriter(aw.w)
	defer tw.Close()

	tmpHdr, err := writeTempHeader(s, "header", writeHeader, args)

	if err != nil {
		return err
	}
	defer os.Remove(tmpHdr.Name())

	// write version file
	inf, err := artifact.ToStream(&artifact.Info{Version: args.Version, Format: args.Format})
	if err != nil {
		return err
	}
	sa := artifact.NewTarWriterStream(tw)
	if err := sa.Write(inf, "version"); err != nil {
		return errors.Wrapf(err, "writer: can not write version tar header")
	}

	if err = writeArtifactVersion(args.Version, aw.signer, tw, s, inf); err != nil {
		return errors.Wrap(err, "WriteArtifact")
	}

	// write header
	if _, err := tmpHdr.Seek(0, 0); err != nil {
		return errors.Wrapf(err, "writer: error preparing tmp header for writing")
	}
	fw := artifact.NewTarWriterFile(tw)
	if err := fw.Write(tmpHdr, "header.tar.gz"); err != nil {
		return errors.Wrapf(err, "writer: can not tar header")
	}

	// write data files
	return writeData(tw, args.Updates)
}

// writeArtifactVersion writes version specific artifact records.
func writeArtifactVersion(version int, signer artifact.Signer, tw *tar.Writer, s *artifact.ChecksumStore, artifactInfoStream []byte) error {
	switch version {
	case 2:
		// add checksum of `version`
		ch := artifact.NewWriterChecksum(ioutil.Discard)
		ch.Write(artifactInfoStream)
		s.Add("version", ch.Checksum())
		// write `manifest` file
		sw := artifact.NewTarWriterStream(tw)
		if err := sw.Write(s.GetRaw(), "manifest"); err != nil {
			return errors.Wrapf(err, "writer: can not write manifest stream")
		}
		// write signature
		if err := WriteSignature(tw, s.GetRaw(), signer); err != nil {
			return err
		}
		// header is written later on
	case 3:
		// Add checksum of `version`.
		ch := artifact.NewWriterChecksum(ioutil.Discard)
		ch.Write(artifactInfoStream)
		s.Add("version", ch.Checksum())
		// Write `manifest` file.
		sw := artifact.NewTarWriterStream(tw)
		if err := sw.Write(s.GetRaw(), "manifest"); err != nil {
			return errors.Wrapf(err, "writer: can not write manifest stream")
		}
		// Write signature.
		if err := WriteSignature(tw, s.GetRaw(), signer); err != nil {
			return err
		}
	case 1:

	default:
		return fmt.Errorf("writer: unsupported artifact version: %d", version)
	}
	return nil
}

// addAugmentedManifest adds an unsigned manifest and a header to the artifact,
func augmentArtifact(stdArtifact io.Reader, augmentedArtifact io.Writer, args *WriteArtifactArgs) error {
	augmentedManifestWritten := false
	augmentedHeaderWritten := false
	// First read the artifact.
	tarReader := tar.NewReader(stdArtifact)
	// Create a new artifact writer, and simply write anew the old artifact.
	tw := tar.NewWriter(augmentedArtifact)
	defer tw.Close()
	// Since we need the checksums, before writing the augmented manifest, get the
	// correct checksums, and store header as a tempFile.
	s := artifact.NewChecksumStore()
	tmpHdr, err := writeTempHeader(s, "header-augment", writeAugmentedHeader, args)
	if err != nil {
		return errors.Wrap(err, "augmentArtifact: failed to write tempAugmentHeader")
	}
	// File needs to be read anew.
	if _, err := tmpHdr.Seek(0, 0); err != nil {
		return errors.Wrapf(err, "addAugmentedManifest: error preparing tmp augmented-header for writing")
	}
	// Place the augmented fields at their corresponding places in the new artifact.
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // Done processing the archive.
		}
		if err != nil {
			return errors.Wrap(err, "addAugmentedManifest: tarReader.Next() failed")
		}
		// Allocate a buffer the size of a tar-record (512 Bytes).
		tarBuf := make([]byte, 512)
		// Simply write the current artifact contents anew.
		err = tw.WriteHeader(header)
		if err != nil {
			return errors.Wrap(err, "addAugmentedManifest: failed to write header")
		}
		n, err := io.CopyBuffer(tw, tarReader, tarBuf)
		if err != nil {
			return errors.Wrapf(err, "addAugmentedManifest: io.CopyBuffer failed: writeLength: %d", n)
		}
		// After the header has been written, we can append our augmented records.
		if header.Name == "manifest.sig" {
			// Add the manifest to the tarWriter.
			sa := artifact.NewTarWriterStream(tw)
			if err = sa.Write(s.GetRaw(), "manifest-augment"); err != nil {
				return errors.Wrap(err, "addAugmentedManifest: failed to write manifest-augment")
			}
			augmentedManifestWritten = true
		}
		if header.Name == "header.tar.gz" && augmentedManifestWritten {
			// Write the augmented header to the new artifact.
			fw := artifact.NewTarWriterFile(tw)
			if err := fw.Write(tmpHdr, "header-augment.tar.gz"); err != nil {
				return errors.Wrapf(err, "addAugmentedManifest: can not tar the augmented header")
			}
			augmentedHeaderWritten = true
		}
	}
	// Be sure that both the augmented records have been written
	if !(augmentedManifestWritten && augmentedHeaderWritten) {
		return fmt.Errorf(
			"augmentArtifact: AugmentError: augmented-manifest: %t, augmented-header: %t",
			augmentedManifestWritten, augmentedHeaderWritten)
	}
	return nil
}

func writeScripts(tw *tar.Writer, scr *artifact.Scripts) error {
	sw := artifact.NewTarWriterFile(tw)
	for _, script := range scr.Get() {
		f, err := os.Open(script)
		if err != nil {
			return errors.Wrapf(err, "writer: can not open script file: %s", script)
		}
		defer f.Close()

		if err :=
			sw.Write(f, filepath.Join("scripts", filepath.Base(script))); err != nil {
			return errors.Wrapf(err, "writer: can not store script: %s", script)
		}
	}
	return nil
}

func extractUpdateTypes(updates *Updates) []artifact.UpdateType {
	u := []artifact.UpdateType{}
	for _, upd := range updates.U {
		u = append(u, artifact.UpdateType{upd.GetType()})
	}
	return u
}

func writeHeader(tarWriter *tar.Writer, args *WriteArtifactArgs) error {
	// store header info
	var hInfo artifact.WriteValidator
	upds := extractUpdateTypes(args.Updates)
	switch args.Version {
	case 1, 2:
		hInfo = artifact.NewHeaderInfo(args.Name, upds, args.Devices)
	case 3:
		hInfo = artifact.NewHeaderInfoV3(upds, args.Provides, args.Depends)
	}

	sa := artifact.NewTarWriterStream(tarWriter)
	stream, err := artifact.ToStream(hInfo)
	if err != nil {
		return errors.Wrap(err, "writeHeader")
	}
	if err := sa.Write(stream, "header-info"); err != nil {
		return errors.New("writer: can not store header-info")
	}

	// write scripts
	if args.Scripts != nil {
		if err := writeScripts(tarWriter, args.Scripts); err != nil {
			return err
		}
	}

	for i, upd := range args.Updates.U {
		if err := upd.ComposeHeader(&handlers.ComposeHeaderArgs{TarWriter: tarWriter, No: i}); err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return nil
}

// WriteAugHeaderArgs is a wrapper for the arguments to the writeAugmentedHeader function.
type WriteAugHeaderArgs struct {
	Version          int
	TarWriter        *tar.Writer
	Updates          *Updates
	ArtifactDepends  *artifact.ArtifactDepends
	ArtifactProvides *artifact.ArtifactProvides
	Scripts          *artifact.Scripts
}

// writeAugmentedHeader writes the augmented header with the restrictions:
// header-info: Can only contain artifact-depends and rootfs_image_checksum.
// type-info: Can only contain artifact-depends and rootfs_image_checksum.
func writeAugmentedHeader(tarWriter *tar.Writer, args *WriteArtifactArgs) error {
	// store header info
	hInfo := new(artifact.AugmentedHeaderInfoV3)

	for _, upd := range args.Updates.U {
		hInfo.Updates =
			append(hInfo.Updates, artifact.UpdateType{Type: upd.GetType()})
	}
	// Augmented header only has artifact-depends.
	hInfo.ArtifactDepends = args.Depends

	sa := artifact.NewTarWriterStream(tarWriter)
	stream, err := artifact.ToStream(hInfo)
	if err != nil {
		return errors.Wrap(err, "writeAugmentedHeader: ")
	}
	if err := sa.Write(stream, "header-info"); err != nil {
		return errors.New("writer: can not store header-info")
	}

	for i, upd := range args.Updates.U {
		if err := upd.ComposeHeader(&handlers.ComposeHeaderArgs{TarWriter: tarWriter, No: i}); err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return nil
}

func writeData(tw *tar.Writer, updates *Updates) error {
	for i, upd := range updates.U {
		if err := upd.ComposeData(tw, i); err != nil {
			return errors.Wrapf(err, "writer: error writing data files")
		}
	}
	return nil
}
