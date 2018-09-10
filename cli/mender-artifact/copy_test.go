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

	"github.com/mendersoftware/mender-artifact/areader"
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

const testPrivateKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIMU7txA5VwTnkNIXQx+XjJE4HLSjvkVsckyFYhpjWXIioAoGCCqGSM49
AwEHoUQDQgAEzzRKBsM1lJ+z/ljS+9kCAJCiTB6+HbyD2TE2hLKGj9xnFkzOHnEj
7KybiE2PAx6skWvCPqBP5+H0d68jN9mOAw==
-----END EC PRIVATE KEY-----
`

const testPublicKey = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEzzRKBsM1lJ+z/ljS+9kCAJCiTB6+
HbyD2TE2hLKGj9xnFkzOHnEj7KybiE2PAx6skWvCPqBP5+H0d68jN9mOAw==
-----END PUBLIC KEY-----
`

func TestCopy(t *testing.T) {
	// build the mender-artifact binary
	assert.Nil(t, exec.Command("go", "build").Run())
	defer os.Remove("mender-artifact")

	dir, err := os.Getwd()
	assert.Nil(t, err)

	tests := []struct {
		initfunc       func(imgpath string)
		stdindata      string
		validimages    []int // artifact, sdimg, sdimg-fat - default all
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
			name:        "write artifact_info file to stdout from signed artifact",
			argv:        []string{"mender-artifact", "cat", ":/etc/mender/artifact_info"},
			validimages: []int{0}, // only artifacts can be signed
			initfunc: func(imgpath string) {
				// Write a private key to the same directory as the test artifact.
				privateKeyFileName := filepath.Join(filepath.Dir(imgpath), "private.key")
				err := ioutil.WriteFile(privateKeyFileName, []byte(testPrivateKey), 0600)
				assert.Nil(t, err, "unexpected error writing private key file: %v", err)

				// Write a public key to the same directory as the test artifact.
				publicKeyFileName := filepath.Join(filepath.Dir(imgpath), "public.key")
				err = ioutil.WriteFile(publicKeyFileName, []byte(testPublicKey), 0600)
				assert.Nil(t, err, "unexpected error writing public key file: %v", err)

				// Use the private key to sign the artifact in place.
				executable := filepath.Join(dir, "mender-artifact")
				cmd := exec.Command(executable, "sign", imgpath, "-k", privateKeyFileName)
				err = cmd.Run()
				assert.Nil(t, err, "unexpected error signing artifact: %v", err)

				// Use the public key to verify the signature.
				cmd = exec.Command(executable, "validate", imgpath, "-k", publicKeyFileName)
				err = cmd.Run()
				assert.Nil(t, err, "unexpected error verifying artifact signature: %v", err)
			},
			expected: "artifact_name=release-1\n",
		},
		{
			initfunc: func(imgpath string) {
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
			initfunc: func(imgpath string) {
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
			name:        "write and read from the data partition",
			argv:        []string{"mender-artifact", "cp", "output.txt", ":/data/test.txt"},
			validimages: []int{1, 2}, // Not valid for an artifact.
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
			name:        "error: artifact does not contain a data partition",
			validimages: []int{0},
			argv:        []string{"mender-artifact", "cp", "output.txt", ":/data/test.txt"},
			err:         "newArtifactExtFile: A mender artifact does not contain a data partition, only a rootfs",
		},
		{
			name:        "error: artifact does not contain a boot partition",
			validimages: []int{0},
			argv:        []string{"mender-artifact", "cp", "output.txt", ":/uboot/test.txt"},
			err:         "newArtifactExtFile: A mender artifact does not contain a boot partition, only a rootfs",
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
			initfunc: func(imgpath string) {
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
				defer pf.Close()
				require.Nil(t, err)
				// Type switch on the artifact, or sdimg underlying
				switch pf.(type) {
				case *artifactExtFile:
					imgpath := pf.(*artifactExtFile).path
					cmd := exec.Command("debugfs", "-R", "stat /etc/mender/testkey.key", imgpath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					require.True(t, strings.Contains(string(out), "Mode:  0600"))
				case sdimgFile:
					imgpath := pf.(sdimgFile)[0].(*extFile).path
					cmd := exec.Command("debugfs", "-R", "stat /etc/mender/testkey.key", imgpath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					require.True(t, strings.Contains(string(out), "Mode:  0600"))
				}
			},
		},
		{
			name: "write and read from the boot partition",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			validimages: []int{1, 2}, // Not valid for artifacts.
			argv:        []string{"mender-artifact", "cp", "foo.txt", ":/uboot/test.txt"},
			verifyTestFunc: func(imgpath string) {
				os.Args = []string{"mender-artifact", "cp",
					imgpath + ":/uboot/test.txt",
					"test.res"}
				err := run()
				assert.NoError(t, err)
				data, err := ioutil.ReadFile("test.res")
				require.Nil(t, err)
				assert.Equal(t, "foobar", string(data))
				os.Remove("foo.txt")
				os.Remove("test.res")
			},
		},
		{
			name: "write and read from the boot/efi partition",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv:        []string{"mender-artifact", "cp", "foo.txt", ":/boot/efi/test.txt"},
			validimages: []int{1, 2}, // Not valid for an artifact.
			verifyTestFunc: func(imgpath string) {
				os.Args = []string{"mender-artifact", "cp",
					imgpath + ":/boot/efi/test.txt",
					"test.res"}
				err := run()
				assert.NoError(t, err)
				data, err := ioutil.ReadFile("test.res")
				require.Nil(t, err)
				assert.Equal(t, "foobar", string(data))
				os.Remove("foo.txt")
				os.Remove("test.res")
			},
		},
		{
			name: "write and read from the boot/grub partition",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			validimages: []int{1, 2}, // Not valid for artifacts.
			argv:        []string{"mender-artifact", "cp", "foo.txt", ":/boot/grub/test.txt"},
			verifyTestFunc: func(imgpath string) {
				os.Args = []string{"mender-artifact", "cp",
					imgpath + ":/boot/grub/test.txt",
					"test.res"}
				err := run()
				assert.NoError(t, err)
				data, err := ioutil.ReadFile("test.res")
				require.Nil(t, err)
				assert.Equal(t, "foobar", string(data))
				os.Remove("foo.txt")
				os.Remove("test.res")
			},
		},
		{
			name:        "error: read non-existing file from sdimg boot partition (ext)",
			validimages: []int{1}, // only valid for ext boot partition
			argv:        []string{"mender-artifact", "cp", ":/boot/grub/test.txt", "foo.txt"},
			err:         "extFile: ReadError: debugfsCopyFile failed: file /test.txt not found in image",
		},
		{
			name:        "error: read non-existing file from sdimg boot partition (fat)",
			validimages: []int{2}, // only valid for fat boot partition
			argv:        []string{"mender-artifact", "cp", ":/boot/grub/test.txt", "foo.txt"},
			err:         "fatPartitionFile: Read: MTools mtype dump failed: exit status 1",
		},
		{
			name:        "error: mender artifact does not containt a boot partition",
			validimages: []int{0}, // only valid for fat boot partition
			argv:        []string{"mender-artifact", "cp", ":/boot/grub/test.txt", "foo.txt"},
			err:         "newArtifactExtFile: A mender artifact does not contain a boot partition, only a rootfs",
		},
		{
			name: "check if artifact does not change name",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			validimages: []int{0},
			argv:        []string{"mender-artifact", "cp", "foo.txt", ":/test.txt"},
			verifyTestFunc: func(imgpath string) {
				// Read the artifact after cp.
				readScripts := func(r io.Reader, info os.FileInfo) error {
					return nil
				}
				f, err := os.Open(imgpath)
				require.Nil(t, err)
				defer f.Close()
				ver := func(message, sig []byte) error {
					return nil
				}
				ar := areader.NewReader(f)
				_, err = read(ar, ver, readScripts)
				require.Nil(t, err)
				// Verify that the artifact-name has not changed.
				assert.Equal(t, "test-artifact", ar.GetArtifactName())
				// Cleanup
				os.Remove("foo.txt")
			},
		},
		{
			name:        "Make sure that the update in a mender artifact does not change name",
			validimages: []int{0}, // Only test for artifacts.
			argv:        []string{"mender-artifact", "cp", "foo.txt", ":/etc/test.txt"},
			verifyTestFunc: func(imgpath string) {
				// Read the artifact after cp.
				readScripts := func(r io.Reader, info os.FileInfo) error {
					return nil
				}
				f, err := os.Open(imgpath)
				require.Nil(t, err)
				defer f.Close()
				ver := func(message, sig []byte) error {
					return nil
				}
				ar := areader.NewReader(f)
				r, err := read(ar, ver, readScripts)
				require.Nil(t, err)
				inst := r.GetHandlers()
				// Verify that the update name has not changed.
				assert.Equal(t, "mender_test.img", inst[0].GetUpdateFiles()[0].Name)
			},
		},
	}

	for _, test := range tests {
		// create a copy of the working images
		artifact, sdimg, fatsdimg, closer := testSetupTeardown(t)
		defer closer()
		validimages := []string{}
		// Add the images the test is valid for.
		if len(test.validimages) == 0 {
			test.validimages = []int{0, 1, 2} // Default case is all images.
		}
		for _, validindex := range test.validimages {
			switch validindex {
			case 0:
				validimages = append(validimages, artifact)
			case 1:
				validimages = append(validimages, sdimg)
			case 2:
				validimages = append(validimages, fatsdimg)
			default:
				t.FailNow()
			}
		}
		// Run once for the artifact, and once for the sdimg
		for _, imgpath := range validimages {
			// buffer the argv vector, since it is used twice
			var testargv = make([]string, len(test.argv))
			copy(testargv, test.argv)

			testargv = argvAddImgPath(imgpath, testargv)

			os.Args = testargv

			if test.initfunc != nil {
				test.initfunc(imgpath)
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
