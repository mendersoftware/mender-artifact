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

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSetupTeardown(t *testing.T) (artifact string, sdimg string, f func()) {

	tmp, err := ioutil.TempDir("", "mender-modify")
	require.NoError(t, err)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	require.NoError(t, err)

	err = WriteArtifact(tmp, 2, filepath.Join(tmp, "mender_test.img"))
	require.NoError(t, err)
	artifact = filepath.Join(tmp, "artifact.mender")

	err = copyFile("mender_test.sdimg", filepath.Join(tmp, "mender_test.sdimg"))
	require.NoError(t, err)
	sdimg = filepath.Join(tmp, "mender_test.sdimg")

	return artifact, sdimg, func() {
		os.RemoveAll(tmp)
	}
}

func TestCopy(t *testing.T) {

	// build the mender-artifact binary
	require.Nil(t, exec.Command("go", "build").Run())
	defer os.Remove("mender-artifact")

	dir, err := os.Getwd()
	require.Nil(t, err)

	tests := []struct {
		initfunc       func()
		name           string
		argv           []string
		expected       string
		err            string
		verifyTestFunc func(imgpath string)
	}{
		{
			name: "error on no path given into the image file",
			argv: []string{"cp", "partitions.sdimg", "output.txt"},
			err:  "failed to parse image path",
		},
		{
			name:     "write artifact_info file to stdout",
			argv:     []string{"cat", ":/etc/mender/artifact_info"},
			expected: "artifact_name=release-1",
		},
		{
			initfunc: func() {
				require.Nil(t, ioutil.WriteFile("output.txt", []byte{}, 0755))
			},
			name: "write artifact_info file to output.txt",
			argv: []string{"cp", ":/etc/mender/artifact_info", "output.txt"},
			verifyTestFunc: func(imgpath string) {
				data, err := ioutil.ReadFile("output.txt")
				require.Nil(t, err)
				assert.Equal(t, strings.TrimSpace(string(data)), "artifact_name=release-1")
			},
		},

		{
			name: "read from output.txt and write to img",
			initfunc: func() {
				// create some new data in the output.txt file,
				// so that it does not shadow the previous test
				require.Nil(t, ioutil.WriteFile("output.txt", []byte("artifact_name=foobar"), 0644))
			},
			argv: []string{"cp", "output.txt", ":/etc/mender/artifact_info"},
			verifyTestFunc: func(imgpath string) {
				// as copy out of image is already verified to function, we
				// will now use this functionality to confirm that the write
				// was succesfull.
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/etc/mender/artifact_info")
				var out bytes.Buffer
				cmd.Stdout = &out
				err := cmd.Run()
				require.Nil(t, err, "got unexpected error: %v", err)
				assert.Equal(t, "artifact_name=foobar", out.String())
			},
		},
	}

	for _, test := range tests {
		// create a copy of the working images
		artifact, sdimg, closer := testSetupTeardown(t)
		defer closer()

		fmt.Printf("---------- Running test -----------\n%s\n-----------------------------------\n", test.name)
		fmt.Println()

		// Run once for the artifact, and once for the sdimg
		for _, imgpath := range []string{artifact, sdimg} {
			// buffer the argv vector, since it is used twice
			var testargv = make([]string, len(test.argv))
			copy(testargv, test.argv)

			testargv = argvAddImgPath(imgpath, testargv)

			if test.initfunc != nil {
				test.initfunc()
			}

			cmd := exec.Command(filepath.Join(dir, "mender-artifact"), testargv...)
			var actual bytes.Buffer
			cmd.Stdout = &actual
			var errout bytes.Buffer
			cmd.Stderr = &errout

			err = cmd.Run()

			if err != nil {
				if test.err != "" {
					assert.Equal(t, test.err, strings.TrimSpace(errout.String()), test.name)
				} else {
					t.Fatal(fmt.Sprintf("cmd: %s failed with err: %v", test.name, err))
				}
			} else {
				if test.verifyTestFunc != nil {
					test.verifyTestFunc(imgpath)
				} else {
					assert.Equal(t, test.expected, strings.TrimSpace(actual.String()), test.name)
				}
			}
		}
	}
}

func argvAddImgPath(imgpath string, sarr []string) []string {
	for i, str := range sarr {
		if strings.Contains(str, "artifact_info") {
			sarr[i] = imgpath + str
		}
	}
	return sarr
}
