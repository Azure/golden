package golden

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

var customTypeMapping = make(map[reflect.Type]cty.Type)

func AddCustomTypeMapping[T any](customType cty.Type) {
	var t T
	customTypeMapping[reflect.TypeOf(t)] = customType
}

// ToCtyValue is a function that converts a primary/collection type to cty.Value
func ToCtyValue(input any) cty.Value {
	if v, isCtyValue := input.(cty.Value); isCtyValue {
		return v
	}
	val := reflect.ValueOf(input)

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cty.NumberIntVal(val.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cty.NumberUIntVal(val.Uint())
	case reflect.Float32, reflect.Float64:
		return cty.NumberFloatVal(val.Float())
	case reflect.String:
		return cty.StringVal(val.String())
	case reflect.Bool:
		return cty.BoolVal(val.Bool())
	case reflect.Slice:
		if val.Len() == 0 {
			sliceType := reflect.TypeOf(input)
			return cty.ListValEmpty(GoTypeToCtyType(sliceType.Elem()))
		}
		var vals []cty.Value
		for i := 0; i < val.Len(); i++ {
			vals = append(vals, ToCtyValue(val.Index(i).Interface()))
		}
		return cty.ListVal(vals)
	case reflect.Map:
		if val.Len() == 0 {
			mapType := reflect.TypeOf(input)
			elementType := mapType.Elem()
			return cty.MapValEmpty(GoTypeToCtyType(elementType))
		}
		vals := make(map[string]cty.Value)
		iter := val.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			vals[key] = ToCtyValue(iter.Value().Interface())
		}
		return cty.MapVal(vals)
	case reflect.Struct:
		vals := make(map[string]cty.Value)
		for i := 0; i < val.NumField(); i++ {
			fn, _ := fieldName(val.Type().Field(i))
			fv := val.Field(i)
			vals[fn] = ToCtyValue(fv.Interface())
		}
		return cty.ObjectVal(vals)
	case reflect.Ptr:
		if val.IsNil() {
			return cty.NilVal
		}
		return ToCtyValue(val.Elem().Interface())
	default:
		return cty.NilVal
	}
}

func GoTypeToCtyType(goType reflect.Type) cty.Type {
	if goType == nil {
		return cty.NilType
	}
	if customType, ok := customTypeMapping[goType]; ok {
		return customType
	}
	switch goType.Kind() {
	case reflect.Bool:
		return cty.Bool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cty.Number
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cty.Number
	case reflect.Float32, reflect.Float64:
		return cty.Number
	case reflect.String:
		return cty.String
	case reflect.Slice:
		elemType := GoTypeToCtyType(goType.Elem())
		return cty.List(elemType)
	case reflect.Map:
		valueType := GoTypeToCtyType(goType.Elem())
		return cty.Map(valueType)
	case reflect.Struct:
		return StructToCtyType(goType)
	default:
		return cty.NilType
	}
}

func StructToCtyType(typ reflect.Type) cty.Type {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return cty.NilType
	}
	attrs := make(map[string]cty.Type)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldName, ok := fieldName(field)
		if !ok {
			continue
		}
		fieldType := field.Type
		ctyType := GoTypeToCtyType(fieldType)
		attrs[fieldName] = ctyType
	}
	return cty.Object(attrs)
}

func Int(i int) *int {
	return &i
}

func CtyValueToString(val cty.Value) string {
	if val.IsNull() && val != cty.NilVal {
		return "null"
	}
	switch val.Type() {
	case cty.String:
		return val.AsString()
	case cty.Number:
		bf := val.AsBigFloat()
		return bf.Text('f', -1)
	case cty.Bool:
		return fmt.Sprintf("%t", val.True())
	case cty.NilType:
		return "nil"
	default:
		if val.Type().IsListType() || val.Type().IsSetType() || val.Type().IsTupleType() {
			strs := make([]string, 0, val.LengthInt())
			it := val.ElementIterator()
			for it.Next() {
				_, v := it.Element()
				strs = append(strs, CtyValueToString(v))
			}
			return "[" + strings.Join(strs, ", ") + "]"
		}
		if val.Type().IsMapType() || val.Type().IsObjectType() {
			strs := make([]string, 0, val.LengthInt())
			it := val.ElementIterator()
			for it.Next() {
				k, v := it.Element()
				strs = append(strs, fmt.Sprintf("%s: %s", k.AsString(), CtyValueToString(v)))
			}
			return "{" + strings.Join(strs, ", ") + "}"
		}
		// For other types, use the GoString method, which will give a
		// string representation of the internal structure of the value.
		return val.GoString()
	}
}
