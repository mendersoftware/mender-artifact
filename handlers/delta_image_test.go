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
	"crypto/sha256"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeltaHandler(t *testing.T) {
	tp, err := ioutil.TempFile("", "tpart")
	require.Nil(t, err)
	dfsi := NewDeltafsInstaller(tp.Name())
	h := sha256.New()
	if _, err = io.Copy(h, tp); err != nil {
		require.Nil(t, err)
	}
	finfo, err := tp.Stat()

	tcs := []struct {
		name string
		df   *DataFile
		errf func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool
	}{
		{"Correct sum", &DataFile{Checksum: h.Sum(nil)}, assert.NoError},
		{"Incorrect sum", &DataFile{Checksum: []byte("incorrectsum")}, assert.Error},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			dfsi.update = tc.df
			err = dfsi.Install(tp, &finfo)
			tc.errf(t, err)
		})
	}
}
