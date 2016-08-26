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

import "errors"

var ErrInvalidInfo = errors.New("invalid artifacts info")

type MetadataInfo struct {
	Format  string `json:"format"`
	Version string `json:"version"`
}

type MetadataInfoJSON string

func (m MetadataInfo) Validate() error {
	if len(m.Format) == 0 || len(m.Version) == 0 {
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
	return nil
}
