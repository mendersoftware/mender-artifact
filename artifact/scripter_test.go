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

package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdding(t *testing.T) {
	s := new(Scripts)
	err := s.Add(`10_ArtifactDownload.Enter.ask-user`)
	assert.NoError(t, err)

	err = s.Add(`10_ArtifactDownload.Leave`)
	assert.NoError(t, err)

	err = s.Add(`/some_directory/10_ArtifactDownload.Leave`)
	assert.NoError(t, err)

	// non existing state
	err = s.Add(`10_InvalidState.Enter`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported script state")

	// bad formatting
	err = s.Add(`10_ArtifactDownload.Bad`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid script")

	err = s.Add(`ArtifactDownload.Enter`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid script")

}
