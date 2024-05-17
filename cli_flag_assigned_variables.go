package golden

import (
	"fmt"
	"github.com/spf13/afero"
	"path/filepath"
	"strings"
)

type CliFlagAssignedVariables interface {
	Variables(c *BaseConfig) (map[string]VariableValueRead, error)
}

var _ CliFlagAssignedVariables = CliFlagAssignedVariable{}

type CliFlagAssignedVariable struct {
	varName  string
	rawValue string
}

func NewCliFlagAssignedVariable(varName, rawValue string) CliFlagAssignedVariable {
	return CliFlagAssignedVariable{
		varName:  varName,
		rawValue: rawValue,
	}
}

func (v CliFlagAssignedVariable) Variables(c *BaseConfig) (map[string]VariableValueRead, error) {
	variableBlocks := Blocks[*VariableBlock](c)
	variables := make(map[string]*VariableBlock)
	for _, vb := range variableBlocks {
		variables[vb.Name()] = vb
	}
	vb, ok := variables[v.varName]
	if !ok {
		return nil, fmt.Errorf(`a variable named "%s" was assigned on the command line, but cannot find a variable of that name. To use this value, add a "variable" block to the configuraion`, v.varName)
	}
	read := vb.parseVariableValueFromString(v.rawValue, false)
	return map[string]VariableValueRead{
		read.Name: read,
	}, nil
}

var _ CliFlagAssignedVariables = CliFlagAssignedVariableFile{}

type CliFlagAssignedVariableFile struct {
	varFileName string
}

func NewCliFlagAssignedVariableFile(varFileName string) CliFlagAssignedVariableFile {
	return CliFlagAssignedVariableFile{
		varFileName: varFileName,
	}
}

func (v CliFlagAssignedVariableFile) Variables(c *BaseConfig) (map[string]VariableValueRead, error) {
	exist, err := afero.Exists(configFs, v.varFileName)
	if err != nil {
		return nil, fmt.Errorf("cannot check existance of %s: %+v", v.varFileName, err)
	}
	if !exist && !strings.HasPrefix(v.varFileName, c.basedir) {
		return CliFlagAssignedVariableFile{varFileName: filepath.Join(c.basedir, v.varFileName)}.Variables(c)
	}
	return c.readVariablesFromVarFile(v.varFileName)
}
