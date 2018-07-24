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

package artifact

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/pkg/errors"
)

// WriteValidator is the interface that wraps the io.Writer interface and
// Validate method.
type WriteValidator interface {
	io.Writer
	Validate() error
}

// ErrValidatingData is an error returned by Validate() in case of
// invalid data.
var ErrValidatingData = errors.New("error validating data")

// Info contains the information about the format and the version
// of artifact archive.
type Info struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
}

// Validate performs sanity checks on artifact info.
func (i Info) Validate() error {
	if len(i.Format) == 0 || i.Version == 0 {
		return ErrValidatingData
	}
	return nil
}

func decode(p []byte, data WriteValidator) error {
	if len(p) == 0 {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(p))
	for {
		if err := dec.Decode(data); err != io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (i *Info) Write(p []byte) (n int, err error) {
	if err := decode(p, i); err != nil {
		return 0, err
	}
	return len(p), nil
}

// UpdateType provides information about the type of update.
// At the moment we are supporting only "rootfs-image" type.
type UpdateType struct {
	Type string `json:"type"`
}

// HeaderInfo contains information of number and type of update files
// archived in Mender metadata archive.
type HeaderInfo struct {
	ArtifactName      string       `json:"artifact_name"`
	Updates           []UpdateType `json:"updates"`
	CompatibleDevices []string     `json:"device_types_compatible"`
}

// Validate checks if header-info structure is correct.
func (hi HeaderInfo) Validate() error {
	if len(hi.Updates) == 0 || len(hi.CompatibleDevices) == 0 || len(hi.ArtifactName) == 0 {
		return ErrValidatingData
	}
	for _, update := range hi.Updates {
		if update == (UpdateType{}) {
			return ErrValidatingData
		}
	}
	return nil
}

func (hi *HeaderInfo) Write(p []byte) (n int, err error) {
	if err := decode(p, hi); err != nil {
		return 0, err
	}
	return len(p), nil
}

type HeaderInfoV3 struct {
	Updates          []UpdateType      `json:"updates"`
	ArtifactProvides *ArtifactProvides `json:"artifact_provides"` // Has its own json marshaller tags.
	ArtifactDepends  *ArtifactDepends  `json:"artifact_depends"`  // Has its own json marshaller  function.
}

// Validate validates the correctness of the header version3.
func (hi *HeaderInfoV3) Validate() error {
	// Artifact must have an update with them.
	if len(hi.Updates) == 0 {
		return ErrValidatingData
	}
	// Updates cannot be empty.
	for _, update := range hi.Updates {
		if update == (UpdateType{}) {
			return ErrValidatingData
		}
	}
	//////////////////////////////////
	// All required Artifact-provides:
	//////////////////////////////////
	/* Artifact-provides cannot be empty. */
	if hi.ArtifactProvides == nil {
		return ErrValidatingData
	}
	/* Artifact must have a name. */
	if len(hi.ArtifactProvides.ArtifactName) == 0 {
		return ErrValidatingData
	}
	/* Artifact must have a group */
	if len(hi.ArtifactProvides.ArtifactGroup) == 0 {
		return ErrValidatingData
	}
	/* Artifact must have at least one supported update type. */
	if len(hi.ArtifactProvides.SupportedUpdateTypes) == 0 {
		return ErrValidatingData
	}
	///////////////////////////////////////
	// Artifact-depends can be empty, thus:
	///////////////////////////////////////
	/* Artifact must not depend on a name. */
	/* Artifact must not depend on a device. */
	return nil
}

func (hi *HeaderInfoV3) Write(p []byte) (n int, err error) {
	if err := decode(p, hi); err != nil {
		return 0, err
	}
	return len(p), nil
}

// AugmentedHeaderInfoV3 has some limitations as compared to the HeaderInfoV3.
// This header can only contain:
// - The type of the update.
// - Artifact depends.
type AugmentedHeaderInfoV3 struct {
	Updates         []UpdateType     `json:"updates"`
	ArtifactDepends *ArtifactDepends // Has its own json marshaller  function.
}

// Validate validates the correctness of the augmented-header version3.
func (hi *AugmentedHeaderInfoV3) Validate() error {
	// Artifact must have an update with them.
	if len(hi.Updates) == 0 {
		return ErrValidatingData
	}
	// No empty updates.
	for _, update := range hi.Updates {
		if update == (UpdateType{}) {
			return ErrValidatingData
		}
	}
	///////////////////////////////////////
	// Artifact-depends can be empty, thus:
	///////////////////////////////////////
	/* Artifact must not depend on a name. */
	/* Artifact must not depend on a device. */
	return nil
}

func (hi *AugmentedHeaderInfoV3) Write(p []byte) (n int, err error) {
	if err := decode(p, hi); err != nil {
		return 0, err
	}
	return len(p), nil
}

type ArtifactProvides struct {
	ArtifactName         string   `json:"artifact_name"`
	ArtifactGroup        string   `json:"artifact_group"`
	SupportedUpdateTypes []string `json:"update_types_supported"` // e.g. rootfs, delta.
}

// TODO - can this replace TypeInfoDepends and provides?
type ArtifactDepends struct {
	ArtifactName      string   `json:"artifact_name"`
	CompatibleDevices []string `json:"device_type"`
}

// TypeInfo provides information of type of individual updates
// archived in artifacts archive.
type TypeInfo struct {
	Type string `json:"type"`
}

// Validate validates corectness of TypeInfo.
func (ti TypeInfo) Validate() error {
	if len(ti.Type) == 0 {
		return ErrValidatingData
	}
	return nil
}

func (ti *TypeInfo) Write(p []byte) (n int, err error) {
	if err := decode(p, ti); err != nil {
		return 0, err
	}
	return len(p), nil
}

type TypeInfoDepends struct {
	RootfsChecksum string `json:"rootfs_image_checksum"`
}

type TypeInfoProvides struct {
	RootfsChecksum string `json:"rootfs_image_checksum"`
}

// TypeInfoV3 provides information about the type of update contained within the
// headerstructure.
type TypeInfoV3 struct {
	// Rootfs/Delta (Required).
	Type string `json:"type"`
	// Checksum of the image that needs to be installed on the device in order to
	// apply the current update.
	ArtifactDepends []TypeInfoDepends `json:"artifact_depends"`
	// Checksum of the image currently installed on the device.
	ArtifactProvides []TypeInfoProvides `json:"artifact_provides"`
}

// Validate checks that the required `Type` field is set.
func (ti *TypeInfoV3) Validate() error {
	if ti.Type == "" {
		return errors.Wrap(ErrValidatingData, "TypeInfoV3: ")
	}
	return nil
}

// Write writes the underlying struct into a json data structure (bytestream).
func (ti *TypeInfoV3) Write(b []byte) (n int, err error) {
	if err := decode(b, ti); err != nil {
		return 0, err
	}
	return len(b), nil
}

// TypeInfoV3 provides information about the type of update contained within the
// header-augment structure.
type AugmentedTypeInfoV3 struct {
	// Rootfs/Delta (Required).
	Type string `json:"type"`
	// Checksum of the image that needs to be installed on the device in order to
	// apply the current update.
	ArtifactDepends []TypeInfoDepends `json:"artifact_depends"`
}

// Validate checks that the required `Type` field is set.
func (ti *AugmentedTypeInfoV3) Validate() error {
	if ti.Type == "" {
		return errors.Wrap(ErrValidatingData, "TypeInfoV3: ")
	}
	return nil
}

// Write writes the underlying struct into a json data structure (bytestream).
func (ti *AugmentedTypeInfoV3) Write(b []byte) (n int, err error) {
	if err := decode(b, ti); err != nil {
		return 0, err
	}
	return len(b), nil
}

// Metadata contains artifacts metadata information. The exact metadata fields
// are user-defined and are not specified. The only requirement is that those
// must be stored in a for of JSON.
// The fields which must exist are update-type dependent. In case of
// `rootfs-update` image type, there are no additional fields required.
type Metadata map[string]interface{}

// Validate check corecness of artifacts metadata. Since the exact format is
// not specified validation always succeeds.
func (m Metadata) Validate() error {
	return nil
}

func (m *Metadata) Write(p []byte) (n int, err error) {
	if err := decode(p, m); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (m *Metadata) Map() map[string]interface{} {
	return map[string]interface{}(*m)
}

// Files represents the list of file names that make up the payload for given
// update.
type Files struct {
	FileList []string `json:"files"`
}

// Validate checks format of Files.
func (f Files) Validate() error {
	if len(f.FileList) == 0 {
		return ErrValidatingData
	}
	for _, f := range f.FileList {
		if len(f) == 0 {
			return ErrValidatingData
		}
	}
	return nil
}

func (f *Files) Write(p []byte) (n int, err error) {
	if err := decode(p, f); err != nil {
		return 0, err
	}
	return len(p), nil
}
