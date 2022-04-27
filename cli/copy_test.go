// Copyright 2022 Northern.tech AS
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

package cli

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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/utils"
)

func testSetupTeardown(t *testing.T) (artifact, sdimg, fatsdimg, sparsesdimg string, f func()) {

	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	err = WriteArtifact(tmp, LatestFormatVersion, filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)
	artifact = filepath.Join(tmp, "artifact.mender")

	err = copyFile("mender_test.sdimg", filepath.Join(tmp, "mender_test.sdimg"))
	assert.NoError(t, err)
	sdimg = filepath.Join(tmp, "mender_test.sdimg")

	err = copyFile("mender_test_fat.sdimg", filepath.Join(tmp, "mender_test_fat.sdimg"))
	require.Nil(t, err)
	fatsdimg = filepath.Join(tmp, "mender_test_fat.sdimg")

	err = copyFile("mender_test.sparse.sdimg", filepath.Join(tmp, "mender_test.sparse.sdimg"))
	require.Nil(t, err)
	sparsesdimg = filepath.Join(tmp, "mender_test.sparse.sdimg")

	return artifact, sdimg, fatsdimg, sparsesdimg, func() {
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

func TestCLIErrors(t *testing.T) {

	tests := []struct {
		initfunc       func(imgpath string)
		name           string
		argv           []string
		expected       string
		err            string
		verifyTestFunc func(imgpath string)
	}{
		{
			name: "error on no path given into the image file",
			argv: []string{"mender-artifact", "cp", "artifact.mender", "output.txt"},
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
			name: "input image has invalid extension",
			argv: []string{"mender-artifact", "cat", "someimg.fooimb:bar"},
			err:  "The input image does not seem to be a valid image",
		},
		{
			name: "error: please enter a path into the image",
			argv: []string{"mender-artifact", "cat", "menderimg.mender:"},
			err:  "please enter a path into the image",
		},
		{
			name: "Install: Error on parse error",
			argv: []string{"mender-artifact", "install", "foo.txt", "nosdimg"},
			err:  "No artifact or sdimg provided",
		},
		{
			name: "Install: Error on no file permissions given",
			argv: []string{"mender-artifact", "install", "testkey", "artifact.mender:/etc/mender/testkey.key"},
			err:  "File permissions needs to be set, if you are simply copying, the cp command should fit your needs",
		},
		{
			name: "Install: Error on  wrong number of arguments given",
			argv: []string{"mender-artifact", "install", "foo.txt", "bar.txt", ":/some/path/file.txt"},
			err:  "Wrong number of arguments, got 3",
		},
		{
			name: "Install: Error on wrong number of arguments given to directory install",
			argv: []string{"mender-artifact", "install", "-d", "foo.txt", ":/some/path/file.txt"},
			err:  "Wrong number of arguments, got 2",
		},
		{
			name: "Install: Error on parse error for directory install",
			argv: []string{"mender-artifact", "install", "-d", "foo.txt"},
			err:  "No artifact or sdimg provided",
		},
	}

	for _, test := range tests {
		err := Run(test.argv)
		assert.Contains(t, err.Error(), test.err)
	}

}

func TestCopyRootfsImage(t *testing.T) {
	// build the mender-artifact binary
	assert.Nil(t, exec.Command("go", "build", "..").Run())
	defer os.Remove("mender-artifact")

	dir, err := os.Getwd()
	require.Nil(t, err)
	dir = filepath.Join(dir, "")

	tests := []struct {
		initfunc       func(imgpath string)
		name           string
		argv           []string
		expected       string
		err            string
		verifyTestFunc func(imgpath string)
	}{
		{
			name:     "write artifact_info file to stdout",
			argv:     []string{"mender-artifact", "cat", "<artifact|sdimg|fat-sdimg|sparse-sdimg>:/etc/mender/artifact_info"},
			expected: "artifact_name=release-1\n",
		},
		{
			name: "write artifact_info file to stdout from signed artifact",
			argv: []string{"mender-artifact", "cat", "<artifact>:/etc/mender/artifact_info"},
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
			argv: []string{"mender-artifact", "cp", "<artifact|sdimg|fat-sdimg|sparse-sdimg>:/etc/mender/artifact_info", "output.txt"},
			verifyTestFunc: func(imgpath string) {
				data, err := ioutil.ReadFile("output.txt")
				assert.Nil(t, err)
				assert.Equal(t, strings.TrimSpace(string(data)), "artifact_name=release-1")
				os.Remove("output.txt")
			},
		},
		{
			name: "read from output.txt and write to img",
			initfunc: func(imgpath string) {
				// create some new data in the output.txt file,
				// so that it does not shadow the previous test
				assert.Nil(t, ioutil.WriteFile("output.txt", []byte("artifact_name=foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "output.txt", "<artifact|sdimg|sparse-sdimg|fat-sdimg>:test.txt"},
			verifyTestFunc: func(imgpath string) {
				// as copy out of image is already verified to function, we
				// will now use this functionality to confirm that the write
				// was succesfull.
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":test.txt")
				var out bytes.Buffer
				cmd.Stdout = &out
				err := cmd.Run()
				assert.Nil(t, err, "got unexpected error: %s", err)
				assert.Equal(t, "artifact_name=foobar", out.String())
				os.Remove("output.txt")
			},
		},
		{
			initfunc: func(imgpath string) {
				assert.Nil(t, ioutil.WriteFile("input.txt", []byte("dummy-text"), 0755))
			},
			name: "copy file into a non-existing directory",
			argv: []string{"mender-artifact", "cp", "input.txt", "<artifact|sdimg|sparse-sdimg|fat-sdimg>:/non/existing/path"},
			err:  "The directory: /non/existing does not exist in the image",
			verifyTestFunc: func(string) {
				os.Remove("input.txt")
			},
		},
		{
			initfunc: func(imgpath string) {
				assert.Nil(t, ioutil.WriteFile("input.mender.txt", []byte("dummy-text"), 0755))
			},
			name: "copy file with containing '.mender' in its filename",
			argv: []string{"mender-artifact", "cp", "input.mender.txt", "<artifact|sdimg|sparse-sdimg|fat-sdimg>:/etc/mender/"},
			verifyTestFunc: func(string) {
				os.Remove("input.txt")
			},
		},
		{
			name: "write and read from the data partition",
			initfunc: func(imgpath string) {
				assert.Nil(t, ioutil.WriteFile("artifact_info", []byte("artifact_name=foobar"), 0755))
				_, err := os.Open("artifact_info")
				require.Nil(t, err)
			},
			argv: []string{"mender-artifact", "cp", "artifact_info", "<sdimg|sparse-sdimg|fat-sdimg>:/data/test.txt"},
			verifyTestFunc: func(imgpath string) {
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/data/test.txt")
				var out bytes.Buffer
				cmd.Stdout = &out
				err := cmd.Run()
				require.Nil(t, err, "catting the copied file does not function")
				assert.Equal(t, "artifact_name=foobar", out.String())
				os.Remove("artifact_info")
			},
		},
		{
			name: "error: artifact does not contain a data partition",
			initfunc: func(string) {
				assert.Nil(t, ioutil.WriteFile("foo.txt", nil, 0755))

			},
			argv: []string{"mender-artifact", "cp", "foo.txt", "<artifact>:/data/test.txt"},
			err:  "newArtifactExtFile: A mender artifact does not contain a data partition, only a rootfs",
			verifyTestFunc: func(string) {
				os.Remove("foo.txt")
			},
		},
		{
			name: "error: artifact does not contain a boot partition",
			initfunc: func(string) {
				assert.Nil(t, ioutil.WriteFile("foo.txt", nil, 0755))

			},
			argv: []string{"mender-artifact", "cp", "foo.txt", "<artifact>:/uboot/test.txt"},
			err:  "newArtifactExtFile: A mender artifact does not contain a boot partition, only a rootfs",
			verifyTestFunc: func(string) {
				os.Remove("foo.txt")
			},
		},
		{
			name: "Install file, standard permissions (0600)",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("testkey", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "install", "-m", "0600", "testkey", "<artifact|sdimg|sparse-sdimg|fat-sdimg>:/etc/mender/testkey.key"},
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
				pf, err := virtualImage.OpenFile(nil, imgpath+":/etc/mender/testkey.key")
				defer pf.Close()
				require.Nil(t, err)
				// Type switch on the artifact, or sdimg underlying
				switch innerImg := pf.(*vImageAndFile).file.(type) {
				case *extFile:
					bin, err := utils.GetBinaryPath("debugfs")
					require.Nil(t, err)
					cmd := exec.Command(bin, "-R", "stat /etc/mender/testkey.key", innerImg.imagePath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					assert.True(t, strings.Contains(string(out), "Mode:  0600"))
				case sdimgFile:
					bin, err := utils.GetBinaryPath("debugfs")
					require.Nil(t, err)
					cmd := exec.Command(bin, "-R", "stat /etc/mender/testkey.key", innerImg[0].(*extFile).imagePath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					assert.True(t, strings.Contains(string(out), "Mode:  0600"))
				default:
					t.Fatal("Unexpected file type")
				}
			},
		},
		{
			name: "Install file with permissions (0777)",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("testkey", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "install", "-m", "0777", "testkey", "<artifact|sdimg|sparse-sdimg|fat-sdimg>:/etc/mender/testkey.key"},
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
				pf, err := virtualImage.OpenFile(nil, imgpath+":/etc/mender/testkey.key")
				defer pf.Close()
				require.Nil(t, err)
				// Type switch on the artifact, or sdimg underlying
				switch innerImg := pf.(*vImageAndFile).file.(type) {
				case *extFile:
					bin, err := utils.GetBinaryPath("debugfs")
					require.Nil(t, err)
					cmd := exec.Command(bin, "-R", "stat /etc/mender/testkey.key", innerImg.imagePath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					assert.True(t, strings.Contains(string(out), "Mode:  0777"))
				case sdimgFile:
					bin, err := utils.GetBinaryPath("debugfs")
					require.Nil(t, err)
					cmd := exec.Command(bin, "-R", "stat /etc/mender/testkey.key", innerImg[0].(*extFile).imagePath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					assert.True(t, strings.Contains(string(out), "Mode:  0777"))
				default:
					t.Fatal("Unexpected file type")
				}
			},
		},
		{
			name: "Install file with permissions (0700)",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("testkey", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "install", "-m", "0700", "testkey", "<artifact|sdimg|sparse-sdimg|fat-sdimg>:/etc/mender/testkey.key"},
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
				pf, err := virtualImage.OpenFile(nil, imgpath+":/etc/mender/testkey.key")
				defer pf.Close()
				require.Nil(t, err)
				// Type switch on the artifact, or sdimg underlying
				switch innerImg := pf.(*vImageAndFile).file.(type) {
				case *extFile:
					bin, err := utils.GetBinaryPath("debugfs")
					require.Nil(t, err)
					cmd := exec.Command(bin, "-R", "stat /etc/mender/testkey.key", innerImg.imagePath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					assert.True(t, strings.Contains(string(out), "Mode:  0700"))
				case sdimgFile:
					bin, err := utils.GetBinaryPath("debugfs")
					require.Nil(t, err)
					cmd := exec.Command(bin, "-R", "stat /etc/mender/testkey.key", innerImg[0].(*extFile).imagePath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					assert.True(t, strings.Contains(string(out), "Mode:  0700"))
				default:
					t.Fatal("Unexpected file type")
				}
			},
		},
		{
			name: "write and read from the boot partition",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", "<sdimg|fat-sdimg|sparse-sdimg>:/uboot/test.txt"},
			verifyTestFunc: func(imgpath string) {
				err := Run([]string{
					"mender-artifact", "cp",
					imgpath + ":/uboot/test.txt",
					"test.res"})
				assert.NoError(t, err)
				data, err := ioutil.ReadFile("test.res")
				assert.Nil(t, err, imgpath)
				assert.Equal(t, "foobar", string(data), imgpath)
				os.Remove("foo.txt")
				os.Remove("test.res")
			},
		},
		{
			name: "write and read from the boot/efi partition",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", "<sdimg|fat-sdimg|sparse-sdimg>:/boot/efi/test.txt"},
			verifyTestFunc: func(imgpath string) {
				err := Run([]string{
					"mender-artifact", "cp",
					imgpath + ":/boot/efi/test.txt",
					"test.res"})
				assert.NoError(t, err)
				data, err := ioutil.ReadFile("test.res")
				require.Nil(t, err)
				assert.Equal(t, "foobar", string(data))
				os.Remove("foo.txt")
				os.Remove("test.res")
			},
		},
		{
			name: "cat from the boot partition",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
				err := Run([]string{
					"mender-artifact", "cp",
					"foo.txt",
					imgpath + ":/boot/efi/test.txt",
				})
				assert.NoError(t, err)
			},
			argv:     []string{"mender-artifact", "cat", "<sdimg|fat-sdimg|sparse-sdimg>:/boot/efi/test.txt"},
			expected: "foobar",
			verifyTestFunc: func(string) {
				os.Remove("foo.txt")
			},
		},
		{
			name: "write and read from the boot/grub partition",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", "<sdimg|fat-sdimg|sparse-sdimg>:/boot/grub/test.txt"},
			verifyTestFunc: func(imgpath string) {
				err := Run([]string{
					"mender-artifact", "cp",
					imgpath + ":/boot/grub/test.txt",
					"test.res"})
				assert.NoError(t, err)
				data, err := ioutil.ReadFile("test.res")
				require.Nil(t, err)
				assert.Equal(t, "foobar", string(data))
				os.Remove("foo.txt")
				os.Remove("test.res")
			},
		},
		{
			name: "error: read non-existing file from sdimg boot partition (ext)",
			argv: []string{"mender-artifact", "cp", "<sdimg|sparse-sdimg>:/boot/grub/test.txt", "foo.txt"},
			err:  "The file: test.txt does not exist in the image",
		},
		{
			name: "error: read non-existing file from sdimg boot partition (fat)",
			argv: []string{"mender-artifact", "cp", "<fat-sdimg>:/boot/grub/test.txt", "foo.txt"},
			err:  "fatPartitionFile: Read: MTools mcopy failed: exit status 1",
		},
		{
			name: "error: mender artifact does not contain a boot partition",
			argv: []string{"mender-artifact", "cp", "<artifact>:/boot/grub/test.txt", "foo.txt"},
			err:  "newArtifactExtFile: A mender artifact does not contain a boot partition, only a rootfs",
		},
		{
			name: "check if artifact does not change name",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", "<artifact>:/test.txt"},
			verifyTestFunc: func(imgpath string) {
				defer os.Remove("foo.txt")
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
				ar.ScriptsReadCallback = readScripts
				ar.VerifySignatureCallback = ver
				err = ar.ReadArtifact()
				require.Nil(t, err)
				// Verify that the artifact-name has not changed.
				assert.Equal(t, "test-artifact", ar.GetArtifactName())
				// Cleanup
			},
		},
		{
			name: "Make sure that the update in a mender artifact does not change name",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", "<artifact>:/etc/test.txt"},
			verifyTestFunc: func(imgpath string) {
				defer os.Remove("foo.txt")
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
				ar.ScriptsReadCallback = readScripts
				ar.VerifySignatureCallback = ver
				err = ar.ReadArtifact()
				require.Nil(t, err)
				inst := ar.GetHandlers()
				// Verify that the update name has not changed.
				assert.Equal(t, "mender_test.img", inst[0].GetUpdateFiles()[0].Name)
			},
		},
		{
			name: "Delete a file from an image or Artifact",
			argv: []string{"mender-artifact", "rm", "<artifact|sdimg|sparse-sdimg|fat-sdimg>:/etc/mender/artifact_info"},
			verifyTestFunc: func(imgpath string) {
				err := Run([]string{
					"mender-artifact", "cat",
					imgpath + ":/etc/mender/artifact_info"})
				assert.Error(t, err)
			},
		},
		{
			name: "Error when deleting a non-empty directory from an image or Artifact",
			argv: []string{"mender-artifact", "rm", "<artifact|sdimg|fat-sdimg>:/etc/mender/"},
			err:  "debugfsRemoveDir: debugfs: error running command:",
		},
		{
			name: "Delete a directory from an image or Artifact recursively",
			argv: []string{"mender-artifact", "rm", "-r", "<artifact|sdimg|fat-sdimg|sparse-sdimg>:/etc/mender/"},
			verifyTestFunc: func(imgpath string) {
				err := Run([]string{
					"mender-artifact", "cat",
					imgpath + ":/etc/mender/artifact_info"})
				assert.Error(t, err)
			},
		},
		{
			name: "Delete a file from a fat partition",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("test.txt", []byte("foobar"), 0644))
				require.NoError(t, Run([]string{
					"mender-artifact", "cp",
					"test.txt",
					imgpath + ":/boot/grub/test.txt"}))
			},
			argv: []string{"mender-artifact", "rm", "<fat-sdimg>:/boot/grub/test.txt"},
			verifyTestFunc: func(imgpath string) {
				defer os.Remove("test.txt")
				err := Run([]string{
					"mender-artifact", "cat",
					imgpath + ":/boot/test.txt"})
				assert.Error(t, err)
			},
		},
		{
			name: "Delete a directory from a fat partition",
			argv: []string{"mender-artifact", "rm", "-r", "<fat-sdimg>:/uboot/testdir/"},
		},
		{
			name: "Make sure that mender-artifact cp keeps the file permissions intact",
			initfunc: func(imgpath string) {
				f, err := os.OpenFile("foobar.txt", os.O_RDWR|os.O_CREATE, 0666)
				require.Nil(t, err)
				defer f.Close()
				err = f.Chmod(0666)
			},
			argv: []string{"mender-artifact", "cp", "foobar.txt", "<artifact|sdimg|fat-sdimg|sparse-sdimg>:/etc/mender/foo.txt"},
			verifyTestFunc: func(imgpath string) {
				defer os.Remove("foobar.txt")
				pf, err := virtualImage.OpenFile(nil, imgpath+":/etc/mender/foo.txt")
				defer pf.Close()
				require.Nil(t, err)
				// Type switch on the artifact, or sdimg underlying
				switch innerImg := pf.(*vImageAndFile).file.(type) {
				case *extFile:
					bin, err := utils.GetBinaryPath("debugfs")
					require.Nil(t, err)
					cmd := exec.Command(bin, "-R", "stat etc/mender/foo.txt", innerImg.imagePath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					require.True(t, strings.Contains(string(out), "Mode:  0666"))
				case sdimgFile:
					bin, err := utils.GetBinaryPath("debugfs")
					require.Nil(t, err)
					cmd := exec.Command(bin, "-R", "stat etc/mender/foo.txt", innerImg[0].(*extFile).imagePath)
					out, err := cmd.CombinedOutput()
					require.Nil(t, err)
					require.True(t, strings.Contains(string(out), "Mode:  0666"))
				}
			},
		},
		{
			name: "Make sure that rootfs-image.checksum is updated when repacking Artifact",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("foo.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "cp", "foo.txt", "<artifact>:/etc/test.txt"},
			verifyTestFunc: func(imgpath string) {
				defer os.Remove("foo.txt")
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
				ar.ScriptsReadCallback = readScripts
				ar.VerifySignatureCallback = ver
				err = ar.ReadArtifact()
				require.Nil(t, err)
				inst := ar.GetHandlers()

				provides, err := inst[0].GetUpdateProvides()
				require.NoError(t, err)
				assert.Equal(t, string(inst[0].GetUpdateFiles()[0].Checksum), provides["rootfs-image.checksum"])
			},
		},
		{
			name: "Create a directory",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("test.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "install", "-d", "<artifact|sdimg|fat-sdimg|sparse-sdimg>:/foo"},
			verifyTestFunc: func(imgpath string) {
				err := Run([]string{
					"mender-artifact",
					"install",
					"-m", "0644",
					"test.txt",
					imgpath + ":/foo/test.txt"})
				assert.NoError(t, err)
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/foo/test.txt")
				var out bytes.Buffer
				cmd.Stdout = &out
				err = cmd.Run()
				assert.Nil(t, err, "got unexpected error: %v", err)
				assert.Equal(t, "foobar", out.String())
				// Cleanup the testkey
				assert.Nil(t, os.Remove("test.txt"))
			},
		},
		{
			name: "Create nested directories",
			initfunc: func(imgpath string) {
				require.Nil(t, ioutil.WriteFile("test.txt", []byte("foobar"), 0644))
			},
			argv: []string{"mender-artifact", "install", "-d", "<artifact|sdimg|fat-sdimg|sparse-sdimg>:/foo/bar"},
			verifyTestFunc: func(imgpath string) {
				err := Run([]string{
					"mender-artifact",
					"install",
					"-m", "0644",
					"test.txt",
					imgpath + ":/foo/bar/test.txt"})
				assert.NoError(t, err)
				cmd := exec.Command(filepath.Join(dir, "mender-artifact"), "cat", imgpath+":/foo/bar/test.txt")
				var out bytes.Buffer
				cmd.Stdout = &out
				err = cmd.Run()
				assert.Nil(t, err, "got unexpected error: %v", err)
				assert.Equal(t, "foobar", out.String())
				// Cleanup the testkey
				assert.Nil(t, os.Remove("test.txt"))
			},
		},
		{
			name: "Create a directory that already exists",
			argv: []string{"mender-artifact", "install", "-d", "<artifact|sdimg|fat-sdimg|sparse-sdimg>:/"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			// create a copy of the working images
			artifact, sdimg, fatsdimg, sparsesdimg, closer := testSetupTeardown(t)
			defer closer()
			type testArgs struct {
				validImg    string
				testArgVect []string
			}
			targs := []testArgs{}
			// Add the images the test is valid for.
			targv := make([][]string, 4)
			targv[0] = make([]string, len(test.argv))
			targv[1] = make([]string, len(test.argv))
			targv[2] = make([]string, len(test.argv))
			targv[3] = make([]string, len(test.argv))
			copy(targv[0], test.argv)
			copy(targv[1], test.argv)
			copy(targv[2], test.argv)
			copy(targv[3], test.argv)
			for i, arg := range test.argv {
				sarr := strings.Split(arg, ":")
				if len(sarr) == 2 {
					testimgs := strings.Split(strings.Trim(sarr[0], "<>"), "|")
					for _, str := range testimgs {
						switch str {
						case "artifact":
							targv[0][i] = artifact + ":" + sarr[1]
							targs = append(targs, testArgs{
								validImg:    artifact,
								testArgVect: targv[0],
							})
						case "sdimg":
							targv[1][i] = sdimg + ":" + sarr[1]
							targs = append(targs, testArgs{
								validImg:    sdimg,
								testArgVect: targv[1],
							})
						case "fat-sdimg":
							targv[2][i] = fatsdimg + ":" + sarr[1]
							targs = append(targs, testArgs{
								validImg:    fatsdimg,
								testArgVect: targv[2],
							})
						case "sparse-sdimg":
							targv[3][i] = sparsesdimg + ":" + sarr[1]
							targs = append(targs, testArgs{
								validImg:    sparsesdimg,
								testArgVect: targv[3],
							})

						default:
							t.Fatalf("Unrecognized image type: %s\n", str)
						}
					}
					break
				}
			}

			if len(targs) == 0 {
				t.Fatalf("Test: %s - enabled no test cases\n", test.name)
			}

			for _, targ := range targs {
				t.Run(filepath.Base(targ.validImg), func(t *testing.T) {
					if strings.HasSuffix(targ.validImg, ".sdimg") {
						skipPartedTestsOnMac(t)
					}

					if test.initfunc != nil {
						test.initfunc(targ.validImg)
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

					err = Run(targ.testArgVect)

					// back to normal state
					w.Close()
					os.Stdout = old // restoring the real stdout
					out := <-outC

					if test.err != "" {
						assert.Contains(t, err.Error(), test.err)
					} else if err != nil {
						t.Log(out)
						t.Logf("Test argument vector: %v", targ)
						t.Fatal(fmt.Sprintf("cmd: %s failed with err: %v", test.name, err))
					}

					if test.verifyTestFunc != nil {
						test.verifyTestFunc(targ.validImg)
					} else if test.err == "" {
						assert.Equal(t, test.expected, out, test.name)
					}
				})
			}
		})
	}
}

func TestCopyFromStdin(t *testing.T) {

	type testArgs struct {
		name      string
		argv      []string
		stdinPipe *os.File
		testImg   string
	}

	type testExpects struct {
		name    string
		testImg string
		err     error
	}

	var testArgv []string

	tests := []struct {
		name           string
		argv           []string
		setupTestFunc  func(testArgs)
		verifyTestFunc func(testExpects)
	}{
		{
			name: "Test boot partition file copy from stdin (ext4, fat)",
			setupTestFunc: func(ta testArgs) {
				ta.stdinPipe.Write([]byte("foobar"))
				testArgv = []string{"mender-artifact", "cp",
					"-", ta.testImg + ":/uboot/foo.txt"}
			},
			verifyTestFunc: func(te testExpects) {
				assert.Nil(t, te.err)

				// Copy back out and verify
				err := Run([]string{
					"mender-artifact", "cp",
					te.testImg + ":/uboot/foo.txt",
					"output.txt"})
				defer os.Remove("output.txt")
				assert.Nil(t, err)
				of, err := ioutil.ReadFile("output.txt")
				assert.Nil(t, err)
				assert.Equal(t, "foobar", string(of), te.name, te.testImg)
			},
		},
		{
			name: "Test non existing dir from stdin (ext, fat)",
			setupTestFunc: func(ta testArgs) {
				ta.stdinPipe.Write([]byte("foobar"))
				testArgv = []string{"mender-artifact", "cp", "-",
					ta.testImg + ":/uboot/nonexisting/foo.txt"}
			},
			verifyTestFunc: func(te testExpects) {
				assert.Error(t, te.err, te.name, te.testImg)
			},
		},
	}

	for _, test := range tests {
		_, sdimg, fatsdimg, sparsesdimg, closer := testSetupTeardown(t)
		defer closer()
		for _, testimg := range []string{sdimg, fatsdimg, sparsesdimg} {
			if strings.HasSuffix(testimg, ".sdimg") {
				skipPartedTestsOnMac(t)
			}

			// Copy in from stdin
			r, w, err := os.Pipe()
			defer r.Close()
			if err != nil {
				t.Fatal(t, err)
			}
			orgStdin := os.Stdin
			os.Stdin = r
			test.setupTestFunc(testArgs{
				name:      test.name,
				testImg:   testimg,
				stdinPipe: w,
			})
			go func(w *os.File) {
				time.Sleep(1 * time.Second)
				err := w.Close() // EOF
				if err != nil {
					t.Fatalf("Failed to close the pipe")
				}
			}(w)
			err = Run(testArgv)
			// Reset the original stdin
			os.Stdin = orgStdin
			test.verifyTestFunc(testExpects{
				name:    test.name,
				testImg: testimg,
				err:     err,
			})

		}

	}
}

func TestCopyModuleImage(t *testing.T) {

	tmpdir, err := ioutil.TempDir("", "mendertest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	artfile := filepath.Join(tmpdir, "artifact.mender")
	df, err := ioutil.TempFile(tmpdir, "CopyModuleImage")
	require.Nil(t, err)
	defer df.Close()

	fd, err := os.OpenFile(filepath.Join(tmpdir, "updateFile"), os.O_WRONLY|os.O_CREATE, 0644)
	require.NoError(t, err)
	fd.Write([]byte("updateContent"))
	fd.Close()

	err = Run([]string{
		"mender-artifact", "write", "module-image",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-T", "testType",
		"-f", filepath.Join(tmpdir, "updateFile"),
	})
	assert.NoError(t, err)

	err = Run([]string{
		"mender-artifact", "cp",
		df.Name(),
		artfile + ":/dummy/path",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFsTypeUnsupported.Error())

	err = Run([]string{
		"mender-artifact", "cat",
		artfile + ":/dummy/path",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFsTypeUnsupported.Error())

	err = Run([]string{
		"mender-artifact", "install",
		"-m", "777",
		df.Name(),
		artfile + ":/dummy/path",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFsTypeUnsupported.Error())

	err = Run([]string{
		"mender-artifact", "rm",
		artfile + ":/dummy/path",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFsTypeUnsupported.Error())

}
