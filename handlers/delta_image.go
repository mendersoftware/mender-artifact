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
package handlers

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"

	"github.com/pkg/errors"
)

// Deltafs is an installer type for delta-image updates.
// It is a wrapper around rootfs with an accompanying checksum
// of the final installed partition.
type Deltafs struct {
	*Rootfs
	IPart string // Inactive partition
}

func NewDeltafsInstaller(inactiveParition string) *Deltafs {
	return &Deltafs{
		Rootfs: NewRootfsInstaller(),
		IPart:  inactiveParition,
	}
}

// GetType returns the type of update, so that the artifact reader/installer
// can differentiate upon which install handler to use.
func (d *Deltafs) GetType() string {
	return "delta-image"
}

func (d *Deltafs) Install(r io.Reader, info *os.FileInfo) error {
	if d.InstallHandler != nil {
		if err := d.InstallHandler(r, d.update); err != nil {
			return errors.Wrap(err, "delta-installation failed")
		}
	}
	// Get the checksum of the newly installed partition
	partf, err := os.OpenFile(d.IPart, os.O_RDONLY, os.ModeDevice)
	h := sha256.New()
	if _, err = io.Copy(h, partf); err != nil {
		return errors.Wrapf(err, "failed to calculate the checksum of the inactive partition")
	}
	if !bytes.Equal(h.Sum(nil), d.update.Checksum) {
		return errors.New("the image checksum and the artifact checksum do not match")
	}
	return nil
}
