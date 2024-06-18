package golden

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/lonegunmanb/hclfuncs"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"
)

var configFs = afero.NewOsFs()

type NewBaseConfigArgs struct {
	Basedir                  string
	VarConfigDir             *string
	Ctx                      context.Context
	DslAbbreviation          string
	DslFullName              string
	IgnoreUnknownVariables   bool
	CliFlagAssignedVariables []CliFlagAssignedVariables
}

type BaseConfig struct {
	ctx                      context.Context
	basedir                  string
	varConfigDir             *string
	d                        *Dag
	rawBlockAddresses        map[string]struct{}
	dslFullName              string
	dslAbbreviation          string
	cliFlagAssignedVariables []CliFlagAssignedVariables
	inputVariables           map[string]VariableValueRead
	inputVariableReadsLoader *sync.Once
	ignoreUnknownVariables   bool
}

func (c *BaseConfig) Context() context.Context {
	return c.ctx
}

func (c *BaseConfig) DslFullName() string     { return c.dslFullName }
func (c *BaseConfig) DslAbbreviation() string { return c.dslAbbreviation }

func (c *BaseConfig) EmptyEvalContext() *hcl.EvalContext {
	return &hcl.EvalContext{
		Functions: hclfuncs.Functions(c.basedir),
		Variables: make(map[string]cty.Value),
	}
}

func (c *BaseConfig) EvalContext() *hcl.EvalContext {
	ctx := c.EmptyEvalContext()
	for bt, bs := range c.blocksByTypes() {
		sample := bs[0]
		if s, ok := sample.(BlockCustomizedRefType); ok {
			bt = s.CustomizedRefType()
		}
		if _, ok := sample.(SingleValueBlock); ok {
			ctx.Variables[bt] = SingleValues(castBlock[SingleValueBlock](bs))
			continue
		}
		ctx.Variables[bt] = Values(bs)
	}
	return ctx
}

func NewBasicConfigFromArgs(a NewBaseConfigArgs) *BaseConfig {
	c := NewBasicConfig(a.Basedir, a.DslFullName, a.DslAbbreviation, a.VarConfigDir, a.CliFlagAssignedVariables, a.Ctx)
	c.ignoreUnknownVariables = a.IgnoreUnknownVariables
	return c
}

func NewBasicConfig(basedir, dslFullName, dslAbbreviation string, varConfigDir *string, cliFlagAssignedVariables []CliFlagAssignedVariables, ctx context.Context) *BaseConfig {
	if ctx == nil {
		ctx = context.Background()
	}
	c := &BaseConfig{
		basedir:                  basedir,
		varConfigDir:             varConfigDir,
		ctx:                      ctx,
		dslAbbreviation:          dslAbbreviation,
		dslFullName:              dslFullName,
		cliFlagAssignedVariables: cliFlagAssignedVariables,
		d:                        newDag(),
		inputVariableReadsLoader: &sync.Once{},
		rawBlockAddresses:        make(map[string]struct{}),
	}
	return c
}

func (c *BaseConfig) RunPrePlan() error {
	return c.runDag(prePlan)
}

func (c *BaseConfig) RunPlan() error {
	return c.runDag(dagPlan)
}

func (c *BaseConfig) GetVertices() map[string]interface{} {
	if c.d == nil {
		return nil
	}
	return c.d.GetVertices()
}

func (c *BaseConfig) GetAncestors(id string) (map[string]interface{}, error) {
	return c.d.GetAncestors(id)
}

func (c *BaseConfig) GetChildren(id string) (map[string]interface{}, error) {
	return c.d.GetChildren(id)
}

func (c *BaseConfig) ValidBlockAddress(address string) bool {
	if v, err := c.d.GetVertex(address); v != nil && err == nil {
		return true
	}
	if _, ok := c.rawBlockAddresses[address]; ok {
		return true
	}
	return false
}

func (c *BaseConfig) readInputVariables() (map[string]VariableValueRead, error) {
	if c.inputVariables != nil {
		return c.inputVariables, nil
	}
	var readErr error
	c.inputVariableReadsLoader.Do(func() {
		envVars := c.readVariablesFromEnv()
		defaultFileVars, err := c.readVariablesFromDefaultVarFiles()
		if err != nil {
			readErr = err
			return
		}
		autoFileVars, err := c.readVariablesFromAutoVarFiles()
		if err != nil {
			readErr = err
			return
		}
		cliAssignedVariables, err := c.readCliAssignedVariables()
		if err != nil {
			readErr = err
			return
		}
		c.inputVariables = merge(envVars, defaultFileVars, autoFileVars, cliAssignedVariables)
	})
	return c.inputVariables, readErr
}

func (c *BaseConfig) readVariablesFromEnv() map[string]VariableValueRead {
	valuesFromEnv := make(map[string]VariableValueRead)
	variables := Blocks[*VariableBlock](c)
	for _, vb := range variables {
		valuesFromEnv[vb.Name()] = vb.readValueFromEnv()
	}
	return valuesFromEnv
}

func (c *BaseConfig) readCliAssignedVariables() (map[string]VariableValueRead, error) {
	r := make(map[string]VariableValueRead)
	for _, assignedVariables := range c.cliFlagAssignedVariables {
		reads, err := assignedVariables.Variables(c)
		if err != nil {
			return nil, err
		}
		r = merge(r, reads)
	}
	return r, nil
}

func (c *BaseConfig) readVariablesFromAutoVarFiles() (map[string]VariableValueRead, error) {
	autoHclVarFilePattern := fmt.Sprintf("*.auto.%svars", c.dslAbbreviation)
	autoJsonVarFilePattern := autoHclVarFilePattern + ".json"

	hclMatches, err := afero.Glob(configFs, filepath.Join(c.variableConfigFilesDir(), autoHclVarFilePattern))
	if err != nil {
		return nil, fmt.Errorf("cannot list auto var files at %s: %+v", c.variableConfigFilesDir(), err)
	}
	jsonMatches, err := afero.Glob(configFs, filepath.Join(c.variableConfigFilesDir(), autoJsonVarFilePattern))
	if err != nil {
		return nil, fmt.Errorf("cannot list auto var files at %s: %+v", c.variableConfigFilesDir(), err)
	}
	matches := append(hclMatches, jsonMatches...)
	sort.Strings(matches)
	return c.readVariablesFromVarFiles(matches)
}

func (c *BaseConfig) readVariablesFromDefaultVarFiles() (map[string]VariableValueRead, error) {
	defaultHclVarFilePath := filepath.Join(c.variableConfigFilesDir(), fmt.Sprintf("%s.%svars", c.dslFullName, c.dslAbbreviation))
	defaultJsonVarFilePath := defaultHclVarFilePath + ".json"
	return c.readVariablesFromVarFiles([]string{defaultHclVarFilePath, defaultJsonVarFilePath})
}

func (c *BaseConfig) readVariablesFromVarFiles(paths []string) (map[string]VariableValueRead, error) {
	r := make(map[string]VariableValueRead)
	for _, path := range paths {
		vars, err := c.readVariablesFromVarFile(path)
		if err != nil {
			return nil, err
		}
		r = merge(r, vars)
	}
	return r, nil
}

func (c *BaseConfig) readVariablesFromVarFile(fileName string) (map[string]VariableValueRead, error) {
	var m map[string]VariableValueRead
	exist, err := afero.Exists(configFs, fileName)
	if err != nil {
		return nil, fmt.Errorf("cannot check existance of %s: %+v", fileName, err)
	}
	if !exist {
		return nil, nil
	}
	content, err := afero.ReadFile(configFs, fileName)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %+v", fileName, err)
	}

	m, err = c.ReadVariablesFromSingleVarFile(content, fileName)
	if err != nil {
		return nil, fmt.Errorf("cannot parse %s: %+v", fileName, err)
	}
	return m, nil
}

func (c *BaseConfig) ReadVariablesFromSingleVarFile(fileContent []byte, fileName string) (map[string]VariableValueRead, error) {
	parser := &varFileParserImpl{dslAbbreviation: c.dslAbbreviation}
	file, err := parser.ParseFile(fileContent, filepath.Base(fileName))
	if err != nil {
		return nil, err
	}
	attributes, diag := file.Body.JustAttributes()
	if diag.HasErrors() {
		return nil, diag
	}
	reads := make(map[string]VariableValueRead)
	for _, attr := range attributes {
		value, diag := attr.Expr.Value(nil)
		var err error
		if diag.HasErrors() {
			err = diag
		}
		reads[attr.Name] = NewVariableValueRead(attr.Name, &value, err)
	}

	return reads, nil
}

func (c *BaseConfig) variableConfigFilesDir() string {
	if c.varConfigDir != nil {
		return *c.varConfigDir
	}
	return c.basedir
}

func (c *BaseConfig) blocksByTypes() map[string][]Block {
	r := make(map[string][]Block)
	for _, b := range blocks(c) {
		bt := b.BlockType()
		r[bt] = append(r[bt], b)
	}
	return r
}

func (c *BaseConfig) buildDag(blocks []Block) error {
	for _, b := range blocks {
		c.rawBlockAddresses[b.Address()] = struct{}{}
	}
	return c.d.buildDag(blocks)
}

func (c *BaseConfig) runDag(onReady func(Block) error) error {
	return c.d.runDag(c, onReady)
}

func (c *BaseConfig) expandBlock(b Block) ([]Block, error) {
	var expandedBlocks []Block
	hclBlock := b.HclBlock()
	attr, ok := hclBlock.Body.Attributes["for_each"]
	if !ok || b.getForEach() != nil {
		return nil, nil
	}
	forEachValue, diag := attr.Expr.Value(c.EvalContext())
	if diag.HasErrors() {
		return nil, diag
	}
	if !forEachValue.CanIterateElements() {
		return nil, fmt.Errorf("invalid `for_each`, except set or map: %s", attr.Range().String())
	}
	address := b.Address()
	upstreams, err := c.d.GetAncestors(address)
	if err != nil {
		return nil, err
	}
	downstreams, err := c.d.GetChildren(address)
	if err != nil {
		return nil, err
	}
	iterator := forEachValue.ElementIterator()
	for iterator.Next() {
		key, value := iterator.Element()
		newBlock := NewHclBlock(hclBlock.Block, hclBlock.wb, NewForEach(key, value))
		nb, err := wrapBlock(b.Config(), newBlock)
		if err != nil {
			return nil, err
		}
		nb.markExpanded()
		expandedAddress := blockAddress(newBlock)
		expandedBlocks = append(expandedBlocks, nb)
		err = c.d.AddVertexByID(expandedAddress, nb)
		if err != nil {
			return nil, err
		}
		for upstreamAddress := range upstreams {
			err := c.d.addEdge(upstreamAddress, expandedAddress)
			if err != nil {
				return nil, err
			}
		}
		for downstreamAddress := range downstreams {
			err := c.d.addEdge(expandedAddress, downstreamAddress)
			if err != nil {
				return nil, err
			}
		}
	}
	b.markExpanded()
	return expandedBlocks, c.d.DeleteVertex(address)
}

func merge[TK, TV comparable](maps ...map[TK]TV) map[TK]TV {
	r := make(map[TK]TV)
	for _, m := range maps {
		for k, v := range m {
			r[k] = v
		}
	}
	return r
}
