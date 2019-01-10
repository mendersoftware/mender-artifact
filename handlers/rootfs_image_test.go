// Copyright 2019 Northern.tech AS
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
	"archive/tar"
	"bytes"
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestHandlerRootfs(t *testing.T) {
	// test if update type is reported correctly
	r := NewRootfsV1("")
	require.Equal(t, "rootfs-image", r.GetUpdateType())

	// test get update files
	r.update = &DataFile{Name: "update.ext4"}
	require.Equal(t, "update.ext4", r.GetUpdateFiles()[0].Name)
	require.Equal(t, 1, r.version)

	r = NewRootfsV2("")
	require.Equal(t, "rootfs-image", r.GetUpdateType())

	// test get update files
	r.update = &DataFile{Name: "update_next.ext4"}
	require.Equal(t, "update_next.ext4", r.GetUpdateFiles()[0].Name)
	require.Equal(t, 2, r.version)

	// test cppy
	n := r.NewInstance()
	require.IsType(t, &Rootfs{}, n)
}

type TestErrWriter bytes.Buffer

func (t *TestErrWriter) Write(b []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func TestRootfsReadHeader(t *testing.T) {
	var r Installer
	r = NewRootfsV1("custom")

	tc := []struct {
		version            int
		rootfs             Installer
		data               string
		name               string
		shouldErr          bool
		errMsg             string
		shouldErrAugmented bool
		errMsgAugmented    string
	}{
		{rootfs: r, data: "invalid", name: "headers/0000/files", shouldErr: true,
			errMsg: "invalid character 'i'", version: 2},
		{rootfs: r, data: `{"files":["update.ext4"]}`,
			name: "headers/0000/files", shouldErr: false, version: 2},
		{rootfs: r, data: `{"files":["update.ext4", "next_update.ext4"]}`,
			name: "headers/0000/files", shouldErr: true, version: 2,
			errMsg: "Rootfs image does not contain exactly one file"},
		{rootfs: r, data: `1212121212121212121212121212`,
			name: "headers/0000/checksums/update.ext4.sum", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/non-existing", shouldErr: true,
			errMsg: "unsupported file", version: 2},
		{rootfs: r, data: "data", name: "headers/0000/type-info", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/meta-data", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/scripts/pre/my_script", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/scripts/post/my_script", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/scripts/check/my_script", shouldErr: false, version: 2},
		{rootfs: r, data: "", name: "headers/0000/signatures/update.sig", shouldErr: false, version: 2},
		/////////////////////////
		// Version 3 specifics //
		/////////////////////////
		{rootfs: NewRootfsV3("custom"), data: `{"files":["update.ext4", "next_update.ext4"]}`,
			name: "headers/0000/files", shouldErr: true, version: 3,
			errMsg:             "\"files\" entry found in version 3 artifact",
			shouldErrAugmented: true,
			errMsgAugmented:    "\"files\" entry found in version 3 artifact"},
		{rootfs: NewRootfsV3("custom"), data: "",
			name: "headers/0000/unexpected-file", shouldErr: true, version: 3,
			errMsg:             "unsupported file",
			shouldErrAugmented: true,
			errMsgAugmented:    "unsupported file"},
		{rootfs: NewRootfsV3("custom"), data: "",
			name: "headers/0000/type-info", shouldErr: false, version: 3},
	}

	for _, test := range tc {

		buf := bytes.NewBuffer(nil)

		tw := tar.NewWriter(buf)
		err := tw.WriteHeader(&tar.Header{
			Name: "not-needed",
			Size: int64(len(test.data)),
		})
		require.NoError(t, err)
		_, err = tw.Write([]byte(test.data))
		require.NoError(t, err)
		err = tw.Close()
		require.NoError(t, err)

		tr := tar.NewReader(buf)
		_, err = tr.Next()
		require.NoError(t, err)

		err = r.ReadHeader(buf, test.name, test.version, false)
		if test.shouldErr {
			require.Error(t, err)
			if test.errMsg != "" {
				require.Contains(t, errors.Cause(err).Error(), test.errMsg)
			}
		} else {
			require.NoError(t, err)
		}

		if test.version < 3 {
			// Done for version 1 and 2
			continue
		}

		err = r.ReadHeader(bytes.NewReader([]byte(test.data)), test.name, test.version, true)
		if test.shouldErrAugmented {
			require.Error(t, err)
			if test.errMsgAugmented != "" {
				require.Contains(t, errors.Cause(err).Error(), test.errMsgAugmented)
			}
		} else {
			require.NoError(t, err)
		}
	}
}
