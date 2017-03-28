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

func TestUpdateUtils(t *testing.T) {
	assert.Equal(t, "data/0001", UpdatePath(1))
	assert.Equal(t, "headers/0002", UpdateHeaderPath(2))
	assert.Equal(t, "data/0003.tar.gz", UpdateDataPath(3))
}
