// Copyright 2017 Northern.tech AS
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
	s := Scripts{}
	err := s.Add(`ArtifactCommit_Enter_10_ask-user`)
	assert.NoError(t, err)
	assert.Len(t, s.names, 1)

	list := s.Get()
	assert.Len(t, list, 1)
	assert.Equal(t, "ArtifactCommit_Enter_10_ask-user", list[0])

	err = s.Add(`ArtifactCommit_Leave_10`)
	assert.NoError(t, err)
	assert.Len(t, s.names, 2)

	err = s.Add(`/some_directory/ArtifactCommit_Enter_11`)
	assert.NoError(t, err)
	assert.Len(t, s.names, 3)

	// script already exists
	err = s.Add(`ArtifactCommit_Enter_11`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "script already exists")
	assert.Len(t, s.names, 3)

	// non existing state
	err = s.Add(`InvalidState_Enter_10`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported script state")
	assert.Len(t, s.names, 3)

	// bad formatting
	err = s.Add(`ArtifactCommit_Bad_10`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid script")
	assert.Len(t, s.names, 3)

	err = s.Add(`ArtifactCommit_Enter`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid script")
	assert.Len(t, s.names, 3)
}
