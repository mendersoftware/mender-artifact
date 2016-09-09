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
)

type ArtifactParser interface {
	ParseHeader(tr *tar.Reader, hPath string) error
	ParseData(r io.Reader) error
	NeedsDataFile() bool
	ArchiveHeader(tw *tar.Writer, srcDir, dstDir string) error
	ArchiveData(tw *tar.Writer, srcDir, dst string) error
}

type Parsers struct {
	pFactory ParserFactory
	ParserIterator
}

func NewParserFactory() *Parsers {
	return &Parsers{map[string]ArtifactParser{}, ParserIterator{}}
}

type pListEntry struct {
	p ArtifactParser
	u string
}
type ParserIterator struct {
	pList []pListEntry
	cnt   int
}

func (pi *ParserIterator) PushParser(parser ArtifactParser, update string) {
	pi.pList = append(pi.pList, pListEntry{parser, update})
}

func (pi *ParserIterator) Next() (ArtifactParser, string, error) {
	if len(pi.pList) <= pi.cnt {
		return nil, "", io.EOF
	}
	defer func() { pi.cnt++ }()
	return pi.pList[pi.cnt].p, pi.pList[pi.cnt].u, nil
}

func (pi *ParserIterator) Reset() {
	pi.cnt = 0
}

type ParserFactory map[string]ArtifactParser

func (p *Parsers) Register(parser ArtifactParser, parsingType string) error {
	if _, ok := p.pFactory[parsingType]; ok {
		return errors.New("parser: already registered")
	}
	p.pFactory[parsingType] = parser
	return nil
}

// GetParser returns copy of the parser
func (p Parsers) GetParser(parsingType string) (ArtifactParser, error) {
	parser, ok := p.pFactory[parsingType]
	if !ok {
		return nil, errors.New("parser: does not exist")
	}
	return parser, nil
}
