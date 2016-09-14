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

package parser

import (
	"archive/tar"
	"errors"
	"io"

	"github.com/mendersoftware/artifacts/metadata"
)

type ContentReader interface {
	ReadUpdateType() (*metadata.UpdateType, error)
	ReadUpdateFiles() error
	ReadDeviceType() (string, error)
	ReadMetadata() (*metadata.Metadata, error)
}

type Reader interface {
	ParseHeader(tr *tar.Reader, hPath string) error
	ParseData(r io.Reader) error

	ContentReader
}

type Writer interface {
	ArchiveHeader(tw *tar.Writer, srcDir, dstDir string) error
	ArchiveData(tw *tar.Writer, srcDir, dst string) error
}

type Parser interface {
	Reader
	Writer
}

type Workers map[string]Parser

type ParseManager struct {
	gParser  Parser
	pFactory map[string]Parser
	pWorker  map[string]Parser
}

func NewParseManager() *ParseManager {
	return &ParseManager{
		nil,
		make(map[string]Parser, 0),
		make(map[string]Parser, 0),
	}
}

func (p *ParseManager) GetWorkers() Workers {
	return p.pWorker
}

func (p *ParseManager) PushWorker(parser Parser, update string) error {
	if _, ok := p.pWorker[update]; ok {
		return errors.New("parser: already registered")
	}
	p.pWorker[update] = parser
	return nil
}

func (p *ParseManager) GetWorker(update string) (Parser, error) {
	if p, ok := p.pWorker[update]; ok {
		return p, nil
	}
	return nil, errors.New("parser: can not find worker for update " + update)
}

func (p *ParseManager) Register(parser Parser, parsingType string) error {
	if _, ok := p.pFactory[parsingType]; ok {
		return errors.New("parser: already registered")
	}
	p.pFactory[parsingType] = parser
	return nil
}

func (p *ParseManager) GetRegistered(parsingType string) (Parser, error) {
	parser, ok := p.pFactory[parsingType]
	if !ok {
		return nil, errors.New("parser: does not exist")
	}
	return parser, nil
}

func (p *ParseManager) GetGeneric() Parser {
	return p.gParser
}
