// Copyright 2021 Northern.tech AS
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

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"slices"

	"github.com/pkg/errors"
)

var (
	ExternalBinaryPaths = []string{"/usr/sbin", "/sbin", "/usr/local/sbin"}
)

var (
	BrewSpecificPaths = []string{"/usr/local/opt"}
)

var unsupportedBinariesDarwin = []string{
	"parted",
	"fsck.ext4",
	"fsck.vfat",
}

const errorUnsupportedDarwin = "Operations that use %q are unfortunately not available on Mac OS."

func GetBinaryPath(command string) (string, error) {
	// first check if command exists in PATH
	p, err := exec.LookPath(command)
	if err == nil {
		return p, nil
	}

	// maybe sbin isn't included in PATH, check there explicitly.
	for _, p = range ExternalBinaryPaths {
		p, err = exec.LookPath(path.Join(p, command))
		if err == nil {
			return p, nil
		}
	}

	if runtime.GOOS == "darwin" {
		// look for the binaries in brew symlink directories
		// example: /usr/local/opt/e2fsprogs/bin, /usr/local/opt/mtools/bin etc.
		for _, p = range BrewSpecificPaths {
			items, err := os.ReadDir(p)
			if err != nil {
				break
			}
			for _, d := range items {
				// normal files and symbolic links will be processed too
				// and just result in error when looking for binary
				k, err := exec.LookPath(path.Join(p, d.Name(), "bin", command))
				if err == nil {
					return k, nil
				}
				k, err = exec.LookPath(path.Join(p, d.Name(), "sbin", command))
				if err == nil {
					return k, nil
				}
			}
		}

		// not found, but oh well...
		base := path.Base(command)
		if slices.Contains(unsupportedBinariesDarwin, base) {
			return command, errors.Wrap(err, fmt.Sprintf(errorUnsupportedDarwin, base))
		}
	}

	return command, err
}
