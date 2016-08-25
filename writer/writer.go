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

package writer

import (
	"encoding/json"

	"github.com/mendersoftware/artifacts/metadata"
	"github.com/pkg/errors"
)

func getInfoJSON(metadata *metadata.MetadataInfo) ([]byte, error) {
	if metadata == nil {
		return nil, nil
	}
	return json.Marshal(metadata)
}

func WriteInfo(metadata *metadata.MetadataInfo) error {
	info, err := getInfoJSON(metadata)
	// below should handle passing empty metadata
	if err != nil || info == nil {
		return errors.New("unable to convert metadata to JSON")
	}
	return nil
}
