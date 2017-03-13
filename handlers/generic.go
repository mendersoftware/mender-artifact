// Copyright 2017 Mender Software AS
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
	"io"
	"os"

	"github.com/mendersoftware/mender-artifact/artifact"
)

type Generic struct {
}

func NewGeneric() *Generic {
	return new(Generic)
}

func (g *Generic) GetUpdateFiles() [](*artifact.File) {
	return nil
}

func (g *Generic) GetType() string {
	return ""
}

func (g *Generic) Copy() artifact.Installer {
	return nil
}

func (g *Generic) SetFromHeader(r io.Reader, name string) error {
	return nil
}

func (g *Generic) Install(r io.Reader, f os.FileInfo) error {
	return nil
}
