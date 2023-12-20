// Copyright 2024 Northern.tech AS
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

//go:build !windows
// +build !windows

package cli

import (
	"os"
	"syscall"
)

func CopyOwner(tFile *os.File, artFile string) error {
	artFileStat, err := os.Stat(artFile)
	if err != nil {
		return err
	}
	err = os.Chown(tFile.Name(), int(artFileStat.Sys().(*syscall.Stat_t).Uid),
		int(artFileStat.Sys().(*syscall.Stat_t).Gid))
	return err
}
