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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeJsonStructures(t *testing.T) {
	type testType struct {
		orig       []byte
		override   []byte
		expected   []byte
		successful bool
	}
	testCases := []testType{
		testType{
			orig:       []byte("{\"test\":\"hidden\"}"),
			override:   []byte("{\"test\":\"overridden\"}"),
			expected:   []byte("{\"test\":\"overridden\"}"),
			successful: true,
		},
		testType{
			orig:       []byte("{\"test\":\"visible\"}"),
			override:   []byte("{\"test2\":\"also-visible\"}"),
			expected:   []byte("{\"test\":\"visible\",\"test2\":\"also-visible\"}"),
			successful: true,
		},
		testType{
			orig:       []byte("{\"test\":[\"visible\"]}"),
			override:   []byte("{}"),
			expected:   []byte("{\"test\":[\"visible\"]}"),
			successful: true,
		},
		testType{
			orig:       []byte("{\"test\":[\"visible\"]}"),
			override:   []byte("{\"test2\":[\"also-visible\"]}"),
			expected:   []byte("{\"test\":[\"visible\"],\"test2\":[\"also-visible\"]}"),
			successful: true,
		},
		testType{
			orig:       []byte("{\"test\":[\"hidden\"]}"),
			override:   []byte("{\"test\":[\"overridden\"]}"),
			expected:   []byte("{\"test\":[\"overridden\"]}"),
			successful: true,
		},
		testType{
			orig:       []byte("{\"test\":[\"hidden\"]}"),
			override:   []byte("{\"test\":\"overridden\"}"),
			successful: false,
		},
		testType{
			orig:       []byte("{\"test\":\"hidden\"}"),
			override:   []byte("{\"test\":[\"overridden\"]}"),
			successful: false,
		},
		testType{
			orig:       []byte("{}"),
			override:   []byte("{\"test\":[{\"inner\":\"overridden\"}]}"),
			successful: false,
		},
		testType{
			orig:       []byte("{\"test\":\"hidden\"}"),
			override:   []byte("{\"test\":[{\"inner\":\"overridden\"}]}"),
			successful: false,
		},
		testType{
			orig:       []byte("{\"test\":[{\"inner\":\"value\"}]}"),
			override:   []byte("{}"),
			successful: false,
		},
		testType{
			orig:       []byte("{\"test\":\"hidden\"}"),
			override:   []byte("{\"test\":{\"inner\":\"overridden\"}}"),
			successful: false,
		},
		testType{
			orig:       []byte("{\"test\":{\"inner\":\"hidden\"}}"),
			override:   []byte("{\"test\":\"overridden\"}"),
			successful: false,
		},
		testType{
			orig:       []byte("{\"test\":{\"inner\":\"hidden\"}}"),
			override:   []byte("{\"test\":{\"inner\":\"overridden\"}}"),
			expected:   []byte("{\"test\":{\"inner\":\"overridden\"}}"),
			successful: true,
		},
		testType{
			orig:       []byte("{\"test\":{\"inner\":\"one\"}}"),
			override:   []byte("{\"test2\":{\"inner\":\"another\"}}"),
			expected:   []byte("{\"test\":{\"inner\":\"one\"},\"test2\":{\"inner\":\"another\"}}"),
			successful: true,
		},
		testType{
			orig:       []byte("{\"test\":{\"inner\":\"one\"}}"),
			override:   []byte("{\"test\":{\"inner2\":\"another\"}}"),
			expected:   []byte("{\"test\":{\"inner\":\"one\",\"inner2\":\"another\"}}"),
			successful: true,
		},
		testType{
			orig:       []byte("{\"test\":{\"inner\":\"one\"}}"),
			override:   []byte("{\"test\":{\"inner\":[\"another\"]}}"),
			successful: false,
		},
		testType{
			orig:       []byte("{\"test\":[\"hidden\"]}"),
			override:   []byte("{\"test\":[{\"inner\":\"overridden\"}]}"),
			successful: false,
		},
	}

	for n, testCase := range testCases {
		t.Run(fmt.Sprintf("test case %d", n), func(t *testing.T) {
			var generic interface{}

			err := json.Unmarshal(testCase.orig, &generic)
			require.NoError(t, err)
			orig := generic.(map[string]interface{})

			err = json.Unmarshal(testCase.override, &generic)
			require.NoError(t, err)
			override := generic.(map[string]interface{})

			result, err := mergeJsonStructures(orig, override)
			if !testCase.successful {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			marshalled, err := json.Marshal(result)
			assert.NoError(t, err)

			assert.Equal(t, testCase.expected, marshalled)
		})
	}
}

func TestModuleImageGetProperties(t *testing.T) {
	orig := NewModuleImage("test-type")
	augm := NewAugmentedModuleImage(orig, "test-type")

	orig.typeInfoV3 = &artifact.TypeInfoV3{}
	orig.typeInfoV3.ArtifactDepends = &artifact.TypeInfoDepends{
		"testDependKey": "testDependValue",
	}
	orig.typeInfoV3.ArtifactProvides = &artifact.TypeInfoProvides{
		"testProvideKey": "testProvideValue",
	}
	orig.metaData = map[string]interface{}{
		"testMetaDataKey": "testMetaDataValue",
	}

	augm.typeInfoV3 = &artifact.TypeInfoV3{}
	augm.typeInfoV3.ArtifactDepends = &artifact.TypeInfoDepends{
		"testAugmentDependKey": "testAugmentDependValue",
	}
	augm.typeInfoV3.ArtifactProvides = &artifact.TypeInfoProvides{
		"testAugmentProvideKey": "testAugmentProvideValue",
	}
	augm.metaData = map[string]interface{}{
		"testAugmentMetaDataKey": "testAugmentMetaDataValue",
	}

	depends, err := orig.GetUpdateDepends()
	assert.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoDepends{
		"testDependKey": "testDependValue",
	}, *depends)

	provides, err := orig.GetUpdateProvides()
	assert.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoProvides{
		"testProvideKey": "testProvideValue",
	}, *provides)

	metaData, err := orig.GetUpdateMetaData()
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"testMetaDataKey": "testMetaDataValue",
	}, metaData)

	depends, err = augm.GetUpdateDepends()
	assert.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoDepends{
		"testDependKey":        "testDependValue",
		"testAugmentDependKey": "testAugmentDependValue",
	}, *depends)

	provides, err = augm.GetUpdateProvides()
	assert.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoProvides{
		"testProvideKey":        "testProvideValue",
		"testAugmentProvideKey": "testAugmentProvideValue",
	}, *provides)

	metaData, err = augm.GetUpdateMetaData()
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"testMetaDataKey":        "testMetaDataValue",
		"testAugmentMetaDataKey": "testAugmentMetaDataValue",
	}, metaData)

	(*augm.typeInfoV3.ArtifactDepends)["testDependKey"] = "alternateValue"
	(*augm.typeInfoV3.ArtifactProvides)["testProvideKey"] = "alternateValue"
	augm.metaData["testMetaDataKey"] = "alternateValue"

	depends, err = augm.GetUpdateDepends()
	assert.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoDepends{
		"testDependKey":        "alternateValue",
		"testAugmentDependKey": "testAugmentDependValue",
	}, *depends)

	provides, err = augm.GetUpdateProvides()
	assert.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoProvides{
		"testProvideKey":        "alternateValue",
		"testAugmentProvideKey": "testAugmentProvideValue",
	}, *provides)

	metaData, err = augm.GetUpdateMetaData()
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"testMetaDataKey":        "alternateValue",
		"testAugmentMetaDataKey": "testAugmentMetaDataValue",
	}, metaData)

	augm.metaData["testMetaDataKey"] = []interface{}{"wrongType"}

	_, err = augm.GetUpdateMetaData()
	assert.Error(t, err)
}
