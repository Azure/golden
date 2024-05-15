package golden

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"path/filepath"
)

type varFileParser interface {
	ParseFile(content []byte, fileName string) (*hcl.File, error)
}

var _ varFileParser = jsonFileParser{}

type jsonFileParser struct {
	dslAbbreviation string
}

func (j jsonFileParser) ParseFile(content []byte, fileName string) (*hcl.File, error) {
	if filepath.Ext(fileName) != ".json" {
		return nil, nil
	}
	parser := hclparse.NewParser()
	file, diag := parser.ParseJSON(content, fileName)
	if diag.HasErrors() {
		return nil, diag
	}
	return file, nil
}

var _ varFileParser = hclFileParser{}

type hclFileParser struct {
	dslAbbreviation string
}

func (h hclFileParser) ParseFile(content []byte, fileName string) (*hcl.File, error) {
	if filepath.Ext(fileName) == ".json" {
		return nil, nil
	}
	parser := hclparse.NewParser()
	file, diag := parser.ParseHCL(content, fileName)
	if diag.HasErrors() {
		return nil, diag
	}
	return file, nil
}

var _ varFileParser = varFileParserImpl{}

type varFileParserImpl struct {
	dslAbbreviation string
}

func (h varFileParserImpl) ParseFile(content []byte, fileName string) (*hcl.File, error) {
	hclParser := hclFileParser{dslAbbreviation: h.dslAbbreviation}
	file, err := hclParser.ParseFile(content, fileName)
	if file != nil || err != nil {
		return file, err
	}
	jsonParser := jsonFileParser{dslAbbreviation: h.dslAbbreviation}
	file, err = jsonParser.ParseFile(content, fileName)
	if file != nil || err != nil {
		return file, err
	}
	return nil, fmt.Errorf("incorrect file %s: %+v", fileName, err)
}
