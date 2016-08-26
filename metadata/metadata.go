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

package metadata

import (
	"errors"
	"strings"
)

var ErrInvalidInfo = errors.New("invalid artifacts info")

type MetadataInfo struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
}

type MetadataInfoJSON string

func (m MetadataInfo) Validate() error {
	if len(m.Format) == 0 || m.Version == 0 {
		return ErrInvalidInfo
	}
	return nil
}

var ErrInvalidHeaderInfo = errors.New("invalid artifacts info")

type MetadataUpdateType struct {
	Type string `json:"type"`
}

type MetadataHeaderInfo struct {
	Updates []MetadataUpdateType `json:"updates"`
}

func (m MetadataHeaderInfo) Validate() error {
	if len(m.Updates) == 0 {
		return ErrInvalidHeaderInfo
	}
	for _, update := range m.Updates {
		if update == (MetadataUpdateType{}) {
			return ErrInvalidHeaderInfo
		}
	}
	return nil
}

var ErrInvalidTypeInfo = errors.New("invalid type info")

type MetadataTypeInfo struct {
	Rootfs string `json:"rootfs"`
}

func (m MetadataTypeInfo) Validate() error {
	if len(m.Rootfs) == 0 {
		return ErrInvalidTypeInfo
	}
	return nil
}

var ErrInvalidMetadata = errors.New("invalid metadata")

type Metadata struct {
	// we don't know exactly what tyoe of data we will have here
	data map[string]interface{}
}

func (m Metadata) Validate() error {
	if m.data == nil {
		return ErrInvalidMetadata
	}
	// mandatory fields
	var deviceType interface{}
	var imageID interface{}

	for k, v := range m.data {
		if v == nil {
			return ErrInvalidMetadata
		}
		if strings.Compare(k, "DeviceType") == 0 {
			deviceType = v
		}
		if strings.Compare(k, "ImageID") == 0 {
			imageID = v
		}
	}
	if deviceType == nil || imageID == nil {
		return ErrInvalidMetadata
	}
	return nil
}

var ErrInvalidFilesInfo = errors.New("invalid files info")

type MetadataFile struct {
	File string `json:"type"`
}

type MetadataFiles struct {
	Files []MetadataFile `json:"files"`
}

func (m MetadataFiles) Validate() error {
	if len(m.Files) == 0 {
		return ErrInvalidFilesInfo
	}
	for _, file := range m.Files {
		if file == (MetadataFile{}) {
			return ErrInvalidFilesInfo
		}
	}
	return nil
}
