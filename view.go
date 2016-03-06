package view

import (
	"reflect"
	"strings"
)

// TODO: make encoders, cache and precompile: https://golang.org/src/encoding/json/encode.go
// TODO: make errors handling (invalidTypeEncoder)
// TODO: use json tags (and so on)
// TODO: use right tags (skip struct slashing if no one tag is setted)
// TODO: able to pass field name converter

const tagName = "view"

func Render(src interface{}, view string) interface{} {
	return mapValue(reflect.ValueOf(src), &options{func(_, fieldTag string) bool {
		return strings.Index(fieldTag, view) >= 0
	}})
}

type options struct {
	// how to cache this case?
	fieldMatcher func(fieldName, fieldTag string) bool
}

func mapValue(v reflect.Value, opt *options) interface{} {
	return getValueMapper(v, opt)(v)
}

type mapperFunc func(v reflect.Value) interface{}

func getValueMapper(v reflect.Value, opt *options) mapperFunc {
	if !v.IsValid() {
		return invalidValueMapper
	}
	tm := getTypeMapper(v.Type(), opt)
	if tm == nil {
		tm = identityMapper
	}
	return tm
}

func invalidValueMapper(v reflect.Value) interface{} {
	return nil
}

func getTypeMapper(t reflect.Type, opt *options) mapperFunc {
	// TODO: cache mappers
	return newTypeMapper(t, opt)
}

func identityMapper(v reflect.Value) interface{} {
	return v.Interface()
}

func newTypeMapper(t reflect.Type, opt *options) mapperFunc {
	switch t.Kind() {
	case reflect.Invalid, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return unsupportedTypeMapper
	case reflect.Ptr:
		return newPtrMapper(t, opt)
	case reflect.Interface:
		return newInterfaceMapper(t, opt)
	case reflect.Map:
		return newMapMapper(t, opt)
	case reflect.Array:
		return newArrayMapper(t, opt)
	case reflect.Slice:
		return newSliceMapper(t, opt)
	case reflect.Struct:
		return newStructMapper(t, opt)
	default:
		return nil
	}
}

func unsupportedTypeMapper(v reflect.Value) interface{} {
	panic("Not implemented")
}

func newPtrMapper(t reflect.Type, opt *options) mapperFunc {
	fn := getTypeMapper(t.Elem(), opt)
	if fn == nil {
		return nil
	}
	pm := &ptrMapper{fn}
	return pm.mapValue
}

type ptrMapper struct {
	elemMapper mapperFunc
}

func (pm *ptrMapper) mapValue(v reflect.Value) interface{} {
	if v.IsNil() {
		return nil
	}
	return pm.elemMapper(v.Elem())
}

func newInterfaceMapper(_ reflect.Type, opt *options) mapperFunc {
	im := &interfaceMapper{opt}
	return im.mapValue
}

type interfaceMapper struct {
	opt *options
}

func (im *interfaceMapper) mapValue(v reflect.Value) interface{} {
	if v.IsNil() {
		return nil
	}
	return mapValue(v, im.opt)
}

func newSliceMapper(t reflect.Type, opt *options) mapperFunc {
	fn := newArrayMapper(t, opt)
	if fn == nil {
		return nil
	}
	sm := &sliceMapper{fn}
	return sm.mapValue
}

type sliceMapper struct {
	arrayMapper mapperFunc
}

func (sm *sliceMapper) mapValue(v reflect.Value) interface{} {
	if v.IsNil() {
		return nil
	}
	return sm.arrayMapper(v)
}

func newArrayMapper(t reflect.Type, opt *options) mapperFunc {
	fn := getTypeMapper(t.Elem(), opt)
	if fn == nil {
		return nil
	}
	am := &arrayMapper{fn}
	return am.mapValue
}

type arrayMapper struct {
	elemMapper mapperFunc
}

var (
	voidInterfaceValue interface{}
	voidInterfaceType  = reflect.TypeOf(voidInterfaceValue)
)

func (am *arrayMapper) mapValue(v reflect.Value) interface{} {
	l := v.Len()
	result := make([]interface{}, l)
	for i := 0; i < l; i++ {
		result[i] = am.elemMapper(v.Index(i))
	}
	return result
}

func newMapMapper(t reflect.Type, opt *options) mapperFunc {
	fn := getTypeMapper(t, opt)
	if fn == nil {
		return nil
	}
	sm := &mapMapper{reflect.MapOf(t.Key(), voidInterfaceType), fn}
	return sm.mapValue
}

type mapMapper struct {
	mapType    reflect.Type
	elemMapper mapperFunc
}

func (mm *mapMapper) mapValue(v reflect.Value) interface{} {
	result := reflect.MakeMap(mm.mapType)
	keys := v.MapKeys()
	for _, key := range keys {
		result.SetMapIndex(key, reflect.ValueOf(mm.elemMapper(v.MapIndex(key))))
	}
	return result.Interface()
}

// структура не преобразуется если не совпало ни одно поле (или совпали все поля) и
// нет необходимости преобразовывать какое-либо поле
func newStructMapper(t reflect.Type, opt *options) mapperFunc {
	fields := buildTypeFields(t)
	fieldMappers := make([]mapperFunc, len(fields))
	canBeIdent := true

	var (
		matchedFields  []field
		matchedMappers []mapperFunc
	)

	// make mappers
	for i, f := range fields {
		// TODO: fieldTypeByIndex(reflect.Type, []int) reflect.Type
		ft := t.Field(f.index).Type
		fn := getTypeMapper(ft, opt)
		// TODO: comment this
		if fn == nil {
			fn = identityMapper
		} else {
			canBeIdent = false
		}
		fieldMappers[i] = fn

		if opt.fieldMatcher(f.name, f.tag) {
			matchedFields = append(matchedFields, f)
			matchedMappers = append(matchedMappers, fn)
		}
	}
	if len(matchedFields) != 0 && len(matchedFields) != len(fields) {
		canBeIdent = false
	}
	if canBeIdent {
		return nil
	}
	if len(matchedFields) == 0 {
		matchedFields = fields
		matchedMappers = fieldMappers
	}

	sm := &structMapper{
		fields:       matchedFields,
		fieldMappers: matchedMappers,
	}
	return sm.mapValue
}

type structMapper struct {
	fields       []field
	fieldMappers []mapperFunc
}

func (sm *structMapper) mapValue(v reflect.Value) interface{} {
	result := make(map[string]interface{})
	for i := range sm.fields {
		f := &sm.fields[i]
		// TODO: fieldValueByIndex(reflect.Value, []int) reflect.Value
		fv := v.Field(f.index)
		val := sm.fieldMappers[i](fv)
		result[f.name] = val
	}
	return result
}

// checks first condition of struct identity: no one field matched
func buildTypeFields(t reflect.Type) (fields []field) {
	// TODO: cache
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		fields = append(fields, field{sf.Name, sf.Index[0], sf.Tag.Get(tagName)})
	}
	return
}

type field struct {
	name  string
	index int
	tag   string
}
