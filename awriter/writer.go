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

func writeTempHeader(s *artifact.ChecksumStore, devices []string, name string,
	upd *Updates, scr *artifact.Scripts) (*os.File, error) {
	// create temporary header file
	f, err := ioutil.TempFile("", "header")
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

		if err = writeHeader(htw, devices, name, upd, scr); err != nil {
			return errors.Wrapf(err, "writer: error writing header")
		}
		return nil
	}()

	if err != nil {
		return nil, err
	}
	s.Add("header.tar.gz", ch.Checksum())

	return f, nil
}

func writeTempAugmentedHeader(s *artifact.ChecksumStore, headerArgs *writeAugHeaderArgs) (*os.File, error) {
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

		headerArgs.tw = htw
		if err = writeAugmentedHeader(headerArgs); err != nil {
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

// TODO - refactor parameterlist into a struct.
func (aw *Writer) WriteArtifact(args *WriteArtifactArgs) error {

	if args.Version == 1 && aw.signer != nil {
		return errors.New("writer: can not create version 1 signed artifact")
	}

	s := artifact.NewChecksumStore()
	// calculate checksums of all data files
	// we need this regardless of which artifact version we are writing
	if err := calcDataHash(s, args.Updates); err != nil {
		return err
	}

	// write temporary header (we need to know the size before storing in tar)
	tmpHdr, err := writeTempHeader(s, args.Devices, args.Name, args.Updates, args.Scripts)
	if err != nil {
		return err
	}
	// var augHdr *os.File
	// if version == 3 {
	// 	augHdr, err = writeTempAugmentedHeader(s, &writeAugHeaderArgs{
	// 		updates: upd,
	// 		artifactDepends: &artifact.ArtifactDepends{
	// 			ArtifactName:      "ArtifactNameDependsDummy",
	// 			CompatibleDevices: devices,
	// 		},
	// 		artifactProvides: &artifact.ArtifactProvides{
	// 			ArtifactName:         name,
	// 			ArtifactGroup:        "ArtifactGroupDummy",
	// 			SupportedUpdateTypes: []string{},
	// 		},
	// 	})
	// 	if err != nil {
	// 		return errors.Wrap(err, "writer: failed to write the augmented header")
	// 	}
	// }
	defer os.Remove(tmpHdr.Name())

	// mender archive writer
	tw := tar.NewWriter(aw.w)
	defer tw.Close()

	// write version file
	inf := artifact.ToStream(&artifact.Info{Version: args.Version, Format: args.Format})
	sa := artifact.NewTarWriterStream(tw)
	if err := sa.Write(inf, "version"); err != nil {
		return errors.Wrapf(err, "writer: can not write version tar header")
	}

	switch args.Version {
	case 3:
		// Add checksum of `version`.
		ch := artifact.NewWriterChecksum(ioutil.Discard)
		ch.Write(inf)
		s.Add("version", ch.Checksum())

		// Write `manifest` file.
		sw := artifact.NewTarWriterStream(tw)
		if err := sw.Write(s.GetRaw(), "manifest"); err != nil {
			return errors.Wrapf(err, "writer: can not write manifest stream")
		}

		// Write signature.
		if err := WriteSignature(tw, s.GetRaw(), aw.signer); err != nil {
			return err
		}

		// // FIXME - only write manifest augment if needed.
		// // Write manifest-augment.
		// // Manifest augment contains the files that are not signed.
		// // Because all the files in manifest has to be signed.
		// sw = artifact.NewTarWriterStream(tw)
		// // TODO - is this the right checksumStore to write? (s.GetRaw).
		// if err := sw.Write(s.GetRaw(), "manifest-augment"); err != nil {
		// 	return errors.Wrap(err, "writer: cannot write manifest-augment stream")
		// }
		// // Header and augmented-header is written later on if needed.
	case 2:
		// add checksum of `version`
		ch := artifact.NewWriterChecksum(ioutil.Discard)
		ch.Write(inf)
		s.Add("version", ch.Checksum())

		// write `manifest` file
		sw := artifact.NewTarWriterStream(tw)
		if err := sw.Write(s.GetRaw(), "manifest"); err != nil {
			return errors.Wrapf(err, "writer: can not write manifest stream")
		}

		// write signature
		if err := WriteSignature(tw, s.GetRaw(), aw.signer); err != nil {
			return err
		}
		// header is written later on

	case 1:
		// header is written later on

	default:
		return errors.New("writer: unsupported artifact version")
	}

	// write header
	if _, err := tmpHdr.Seek(0, 0); err != nil {
		return errors.Wrapf(err, "writer: error preparing tmp header for writing")
	}
	fw := artifact.NewTarWriterFile(tw)
	if err := fw.Write(tmpHdr, "header.tar.gz"); err != nil {
		return errors.Wrapf(err, "writer: can not tar header")
	}

	// Artifact version3 has an augmented header.
	if args.Version == 3 {
		// // TODO - which header to write?
		// // FIXME - only write the header-augment if needed.
		// augfw := artifact.NewTarWriterFile(tw)
		// // write the augmented-header
		// if _, err := augHdr.Seek(0, 0); err != nil {
		// 	return errors.Wrapf(err, "writer: error preparing tmp aug-header for writing")
		// }
		// if err := augfw.Write(augHdr, "header-augment.tar.gz"); err != nil {
		// 	return errors.Wrap(err, "writer: cannot tar augment header")
		// }
	}

	// write data files
	return writeData(tw, args.Updates)
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

func writeHeader(tw *tar.Writer, devices []string, name string,
	updates *Updates, scr *artifact.Scripts) error {
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

	// write scripts
	if scr != nil {
		if err := writeScripts(tw, scr); err != nil {
			return err
		}
	}

	for i, upd := range updates.U {
		if err := upd.ComposeHeader(&handlers.ComposeHeaderArgs{TarWriter: tw, No: i}); err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return nil
}

func writeHeaderV3(args *writeAugHeaderArgs) error {
	// store header info
	hInfo := new(artifact.HeaderInfoV3)

	for _, upd := range args.updates.U {
		hInfo.Updates =
			append(hInfo.Updates, artifact.UpdateType{Type: upd.GetType()})
	}
	// Artifact provides and depends needs to be user provided.
	hInfo.ArtifactProvides = args.artifactProvides
	hInfo.ArtifactDepends = args.artifactDepends

	sa := artifact.NewTarWriterStream(args.tw)
	if err := sa.Write(artifact.ToStream(hInfo), "header-info"); err != nil {
		return errors.New("writer: can not store header-info")
	}

	// write scripts
	if args.scripts != nil {
		if err := writeScripts(args.tw, args.scripts); err != nil {
			return err
		}
	}

	for i, upd := range args.updates.U {
		if err := upd.ComposeHeader(&handlers.ComposeHeaderArgs{TarWriter: args.tw, No: i}); err != nil {
			return errors.Wrapf(err, "writer: error processing update directory")
		}
	}
	return nil
}

// writeAugHeaderArgs is a wrapper for the arguments to the writeAugmentedHeader function.
type writeAugHeaderArgs struct {
	tw               *tar.Writer
	updates          *Updates
	artifactDepends  *artifact.ArtifactDepends
	artifactProvides *artifact.ArtifactProvides
	scripts          *artifact.Scripts
}

// writeAugmentedHeader writes the augmented header with the restrictions:
// header-info: Can only contain artifact-depends and rootfs_image_checksum.
// type-info: Can only contain artifact-depends and rootfs_image_checksum.
func writeAugmentedHeader(args *writeAugHeaderArgs) error {
	// store header info
	hInfo := new(artifact.AugmentedHeaderInfoV3)

	for _, upd := range args.updates.U {
		hInfo.Updates =
			append(hInfo.Updates, artifact.UpdateType{Type: upd.GetType()})
	}
	// Augmented header only has artifact-depends.
	hInfo.ArtifactDepends = args.artifactDepends

	sa := artifact.NewTarWriterStream(args.tw)
	if err := sa.Write(artifact.ToStream(hInfo), "header-info"); err != nil {
		return errors.New("writer: can not store header-info")
	}

	for i, upd := range args.updates.U {
		if err := upd.ComposeHeader(&handlers.ComposeHeaderArgs{TarWriter: args.tw, No: i}); err != nil {
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
