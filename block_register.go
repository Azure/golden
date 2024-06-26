package golden

import (
	"github.com/emirpasic/gods/sets"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/lonegunmanb/go-defaults"
	"reflect"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type blockConstructor = func(Config, *HclBlock) Block
type blockRegistry map[string]blockConstructor

var validBlockTypes sets.Set = hashset.New()
var refIters = map[string]refIterator{}

var baseFactory = map[string]func() any{}

func RegisterBaseBlock(factory func() BlockType) {
	bb := factory()
	baseFactory[bb.BlockType()] = func() any {
		return factory()
	}
}

func RegisterBlock(t Block) {
	bt := t.BlockType()
	refKeyWord := bt
	if s, ok := t.(BlockCustomizedRefType); ok {
		refKeyWord = s.CustomizedRefType()
	}
	registry, ok := factories[bt]
	if !ok {
		registry = make(blockRegistry)
		factories[bt] = registry
	}
	_, ok = refIters[refKeyWord]
	if !ok {
		refIters[refKeyWord] = iterator(refKeyWord, t.AddressLength())
	}
	validBlockTypes.Add(bt)
	registry[t.Type()] = func(c Config, hb *HclBlock) Block {
		newBlock := reflect.New(reflect.TypeOf(t).Elem()).Elem()
		newBaseBlock := NewBaseBlock(c, hb)
		newBaseBlock.setForEach(hb.ForEach)
		newBaseBlock.setMetaNestedBlock()
		newBlock.FieldByName("BaseBlock").Set(reflect.ValueOf(newBaseBlock))
		b := newBlock.Addr().Interface().(Block)
		if f, ok := baseFactory[bt]; ok {
			blockName := cases.Title(language.English).String(bt)
			newBlock.FieldByName("Base" + blockName).Set(reflect.ValueOf(f()))
		}
		defaults.SetDefaults(b)
		return b
	}
}

func IsBlockTypeWanted(bt string) bool {
	return validBlockTypes.Contains(bt)
}

func registerCommonBlock() {
	RegisterBlock(new(LocalBlock))
	RegisterBlock(new(VariableBlock))
}

var factories = map[string]blockRegistry{}
