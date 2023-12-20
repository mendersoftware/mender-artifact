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

//go:build windows
// +build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

func CopyOwner(tFile *os.File, artFile string) error {
	art, err := os.Open(artFile)
	if err != nil {
		return err
	}
	defer art.Close()
	artHandle := windows.Handle(art.Fd())
	sd, err := windows.GetSecurityInfo(artHandle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	tHandle := windows.Handle(tFile.Fd())
	err = windows.SetSecurityInfo(tHandle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, owner, nil, nil, nil)
	return err
}
