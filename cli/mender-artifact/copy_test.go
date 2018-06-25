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
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSetupTeardown(t *testing.T) (artifact, sdimg, fatsdimg string, f func()) {

	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	err = WriteArtifact(tmp, 2, filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)
	artifact = filepath.Join(tmp, "artifact.mender")

	err = copyFile("mender_test.sdimg", filepath.Join(tmp, "mender_test.sdimg"))
	assert.NoError(t, err)
	sdimg = filepath.Join(tmp, "mender_test.sdimg")

	err = copyFile("mender_test_fat.sdimg", filepath.Join(tmp, "mender_test_fat.sdimg"))
	require.Nil(t, err)
	fatsdimg = filepath.Join(tmp, "mender_test_fat.sdimg")

	return artifact, sdimg, fatsdimg, func() {
		os.RemoveAll(tmp)
	}
}

func TestCopy(t *testing.T) {

	// build the mender-artifact binary
	assert.Nil(t, exec.Command("go", "build").Run())
	defer os.Remove("mender-artifact")

	dir, err := os.Getwd()
	assert.Nil(t, err)

	tests := []struct {
		initfunc       func()
		stdindata      string
		name           string
		argv           []string
		expected       string
		err            string
		verifyTestFunc func(imgpath string)
	}{
		{
			name: "error on no path given into the image file",
			argv: []string{"mender-artifact", "cp", "partitions.sdimg", "output.txt"},
			err:  "failed to parse image path",
		},
		{
			name: "no mender artifact or sdimg provided",
			argv: []string{"mender-artifact", "cp", "foobar", "output.txt"},
			err:  "no artifact or sdimage provided\n",
		},
		{
			name: "got 1 arguments, wants two",
			argv: []string{"mender-artifact", "cp", "foobar"},
			err:  "got 1 arguments, wants two",
		},
		{
			name: "too many arguments provided to cat",
			argv: []string{"mender-artifact", "cat", "foo", "bar"},
			err:  "Got 2 arguments, wants one",
		},
		{
			name: "error: please enter a path into the image",
			argv: []string{"mender-artifact", "cp", "foo", "menderimg.mender:"},
			err:  "please enter a path into the image",
		},
		{
			name:     "write artifact_info file to stdout",
			argv:     []string{"mender-artifact", "cat", ":/etc/mender/artifact_info"},
			expected: "artifact_name=release-1\n",
		},
		{
			initfunc: func() {
				assert.Nil(t, ioutil.WriteFile("output.txt", []byte{}, 0755))
			},
			name: "write artifact_info file to output.txt",
			argv: []string{"mender-artifact", "cp", ":/etc/mender/artifact_info", "output.txt"},
			verifyTestFunc: func(imgpath string) {
				data, err := ioutil.ReadFile("output.txt")
				assert.Nil(t, err)
				assert.Equal(t, strings.TrimSpace(string(data)), "artifact_name=release-1")
			},
		},

		{
			name: "read from output.txt and write to img",
			initfunc: func() {
				// create some new data in the output.txt file,
				// so that it does not shadow the previous test
				assert.Nil(t, ioutil.WriteFile("output.txt", []byte("artifact_name=foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "output.txt", ":/etc/mender/artifact_info"},
			verifyTestFunc: func(imgpath string) {
				// as copy out of image is already verified to function, we
				// will now use this functionality to confirm that the write
				// was succesfull.
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/etc/mender/artifact_info")
				var out bytes.Buffer
				cmd.Stdout = &out
				err := cmd.Run()
				assert.Nil(t, err, "got unexpected error: %v", err)
				assert.Equal(t, "artifact_name=foobar", out.String())
			},
		},
		{
			name: "data on stdin",
			argv: []string{"mender-artifact", "cp", ":/etc/mender/artifact_info", "-"},
		},
		{
			name: "write and read from the data partition",
			argv: []string{"mender-artifact", "cp", "output.txt", ":/data/test.txt"},
			verifyTestFunc: func(imgpath string) {
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/data/test.txt")
				var out bytes.Buffer
				cmd.Stdout = &out
				err := cmd.Run()
				require.Nil(t, err, "catting the copied file does not function")
				assert.Equal(t, "artifact_name=foobar", out.String())
			},
		},
		{
			name: "Install: Error on no file permissions given",
			argv: []string{"mender-artifact", "install", "testkey", ":/etc/mender/testkey.key"},
			err:  "File permissions needs to be set, if you are simply copying, the cp command should fit your needs",
		},
		{
			name: "Install: Error on parse error",
			argv: []string{"mender-artifact", "install", "foo.txt", "nosdimg"},
			err:  "No artifact or sdimg provided",
		},
		{
			name: "Install: Error on  wrong number of arguments given",
			argv: []string{"mender-artifact", "install", "foo.txt", "bar.txt", ":/some/path/file.txt"},
			err:  "got 3 arguments, wants two",
		},
		{
			name: "Install file, standard permissions (0600)",
			initfunc: func() {
				require.Nil(t, ioutil.WriteFile("testkey", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "install", "-m", "0600", "testkey", ":/etc/mender/testkey.key"},
			verifyTestFunc: func(imgpath string) {
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/etc/mender/testkey.key")
				var out bytes.Buffer
				cmd.Stdout = &out
				err := cmd.Run()
				assert.Nil(t, err, "got unexpected error: %v", err)
				assert.Equal(t, "foobar", out.String())
				// Cleanup the testkey
				assert.Nil(t, os.Remove("testkey"))
				// Check that the permission bits have been set correctly!
				pf, err := NewPartitionFile(imgpath+":/etc/mender/testkey.key", "")
				require.Nil(t, err)
				// Type switch on the artifact, or sdimg underlying
				switch pf.(type) {
				case *partitionFile:
					imgpath := pf.(*partitionFile).path
					cmd := exec.Command("debugfs", "-R", "stat /etc/mender/testkey.key", imgpath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					require.True(t, strings.Contains(string(out), "Mode:  0600"))
				case partitions:
					imgpath := pf.(partitions)[0].path
					cmd := exec.Command("debugfs", "-R", "stat /etc/mender/testkey.key", imgpath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					require.True(t, strings.Contains(string(out), "Mode:  0600"))
				}
			},
		},
		{
			name: "write and read from the boot partition",
			initfunc: func() {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", ":/boot/test.txt"},
			verifyTestFunc: func(imgpath string) {
				os.Args = []string{"mender-artifact", "cp",
					imgpath + ":/boot/test.txt",
					"test.res"}
				err := run()
				assert.NoError(t, err)
				data, err := ioutil.ReadFile("test.res")
				require.Nil(t, err)
				assert.Equal(t, "foobar", string(data))
			},
		},
		{
			name: "write and read from the boot/efi partition",
			initfunc: func() {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", ":/boot/efi/test.txt"},
			verifyTestFunc: func(imgpath string) {
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/boot/efi/test.txt")
				var out bytes.Buffer
				cmd.Stdout = &out
				err := cmd.Run()
				require.Nil(t, err, fmt.Sprintf("catting the copied file does not function: %v", err))
				assert.Equal(t, "foobar", out.String())
			},
		},
		{
			name: "write and read from the boot/grub partition",
			initfunc: func() {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", ":/boot/grub/test.txt"},
			verifyTestFunc: func(imgpath string) {
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/boot/grub/test.txt")
				var out bytes.Buffer
				cmd.Stdout = &out
				err := cmd.Run()
				require.Nil(t, err, fmt.Sprintf("catting the copied file does not function: %v", err))
				assert.Equal(t, "foobar", out.String())
			},
		},
	}

	for _, test := range tests {
		// create a copy of the working images
		artifact, sdimg, fatsdimg, closer := testSetupTeardown(t)
		defer closer()

		t.Logf("---------- Running test -----------\n%s\n-----------------------------------\n", test.name)

		// Run once for the artifact, and once for the sdimg
		for _, imgpath := range []string{artifact, sdimg, fatsdimg} {
			// buffer the argv vector, since it is used twice
			var testargv = make([]string, len(test.argv))
			copy(testargv, test.argv)

			testargv = argvAddImgPath(imgpath, testargv)

			os.Args = testargv

			if test.initfunc != nil {
				test.initfunc()
			}

			old := os.Stdout // keep backup of the real stdout
			r, w, err := os.Pipe()
			if err != nil {
				log.Fatal(err)
			}
			os.Stdout = w

			outC := make(chan string)
			// copy the output in a separate goroutine so printing can't block indefinitely
			go func() {
				var buf bytes.Buffer
				io.Copy(&buf, r)
				outC <- buf.String()
			}()

			err = run()

			// back to normal state
			w.Close()
			os.Stdout = old // restoring the real stdout
			out := <-outC

			if err != nil {
				if test.err != "" {
					assert.Equal(t, test.err, err.Error(), test.name)
				} else {
					t.Log(out)
					t.Fatal(fmt.Sprintf("cmd: %s failed with err: %v", test.name, err))
				}
			} else {
				if test.verifyTestFunc != nil {
					test.verifyTestFunc(imgpath)
				} else {
					assert.Equal(t, test.expected, out, test.name)
				}
			}
		}
	}
}

// Dirty hack to add the temp-path to the argument-vector.
// NOTE - The path is added according to the strings.Contains search,
// and thus argument files, and files in the path cannot match.
func argvAddImgPath(imgpath string, sarr []string) []string {
	for i, str := range sarr {
		if strings.Contains(str, "artifact_info") || strings.Contains(str, "testkey.key") || strings.Contains(str, "test.txt") {
			sarr[i] = imgpath + str
		}
	}
	return sarr
}
