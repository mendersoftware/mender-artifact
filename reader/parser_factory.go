// Copyright 2016 Mender Software AS
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

package reader

import (
	"archive/tar"
	"errors"
	"io"
)

type ArtifactParser interface {
	ParseHeader(tr *tar.Reader, hPath string) error
	ParseData(r io.Reader) error
}

type ParserFactory map[string]ArtifactParser

type Parsers struct {
	par ParserFactory
}

func (p *Parsers) Register(updater ArtifactParser, name string) error {
	if _, ok := p.par[name]; ok {
		return errors.New("artifact updater: already registered")
	}
	p.par[name] = updater
	return nil
}

// GetParser returns the copy of the parser
func (p Parsers) GetParser(name string) (ArtifactParser, error) {
	updater, ok := p.par[name]
	if !ok {
		return nil, errors.New("artifact updater: trying to get non existing updater")
	}
	return updater, nil
}
