package view

import (
	"reflect"
	"strings"
	"sync"
)

// TODO: field name converter
// TODO: add force flag (split caching)

const tagName = "view"

// An UnsupportedTypeError is returned by Render when attempting
// to process an unsupported value type.
type UnsupportedTypeError struct {
	Type reflect.Type
}

func (e *UnsupportedTypeError) Error() string {
	return "struct-view: unsupported type: " + e.Type.String()
}

func Render(src interface{}, viewName string) (interface{}, error) {
	m := &viewMatcher{viewName}
	opt := &options{false, viewName, m.match}
	return mapInterface(src, opt)
}

type options struct {
	noCache      bool
	cacheTag     string
	fieldMatcher func(f field) bool
}

type viewMatcher struct {
	viewName string
}

func (m *viewMatcher) match(f field) bool {
	return f.isMatchView(m.viewName)
}

func mapInterface(src interface{}, opt *options) (i interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*UnsupportedTypeError); ok {
				err = e
				return
			}
			panic(r)
		}
	}()
	i = mapValue(reflect.ValueOf(src), opt)
	return
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

func identityMapper(v reflect.Value) interface{} {
	return v.Interface()
}

type mapperCacheKey struct {
	t   reflect.Type
	tag string
}

var mapperCache struct {
	sync.RWMutex
	m map[mapperCacheKey]mapperFunc
}

// getTypeMapper is like newTypeMapper but uses a cache to avoid repeated work if it possible.
func getTypeMapper(t reflect.Type, opt *options) mapperFunc {
	if opt.noCache {
		// TODO: check processing of recursive structs
		return newTypeMapper(t, opt)
	}

	key := mapperCacheKey{t, opt.cacheTag}
	mapperCache.RLock()
	f := mapperCache.m[key]
	mapperCache.RUnlock()
	if f != nil {
		return f
	}

	// To deal with recursive types, populate the map with an
	// indirect func before we build it. This type waits on the
	// real func (f) to be ready and then calls it.  This indirect
	// func is only used for recursive types.
	mapperCache.Lock()
	if mapperCache.m == nil {
		mapperCache.m = make(map[mapperCacheKey]mapperFunc)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	mapperCache.m[key] = func(v reflect.Value) interface{} {
		wg.Wait()
		return f(v)
	}
	mapperCache.Unlock()

	// Compute fields without lock.
	// Might duplicate effort but won't hold other computations back.
	f = newTypeMapper(t, opt)
	wg.Done()

	mapperCache.Lock()
	mapperCache.m[key] = f
	mapperCache.Unlock()
	return f
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
	panic(&UnsupportedTypeError{v.Type()})
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
	return mapValue(v.Elem(), im.opt)
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
	voidInterfaceValuePtr = new(interface{})
	voidInterfaceType     = reflect.TypeOf(voidInterfaceValuePtr).Elem()
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
	fn := getTypeMapper(t.Elem(), opt)
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
	fields := getTypeFields(t)
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

		if opt.fieldMatcher(f) {
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

var fieldCache struct {
	sync.RWMutex
	m map[reflect.Type][]field
}

// getTypeFields is like buildTypeFields but uses a cache to avoid repeated work.
func getTypeFields(t reflect.Type) []field {
	fieldCache.RLock()
	f, exists := fieldCache.m[t]
	fieldCache.RUnlock()
	if exists {
		return f
	}

	// Compute fields without lock.
	// Might duplicate effort but won't hold other computations back.
	f = buildTypeFields(t)

	fieldCache.Lock()
	if fieldCache.m == nil {
		fieldCache.m = make(map[reflect.Type][]field)
	}
	fieldCache.m[t] = f
	fieldCache.Unlock()
	return f
}

func buildTypeFields(t reflect.Type) (fields []field) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		fields = append(fields, newField(sf))
	}
	return
}

func newField(sf reflect.StructField) field {
	tag := sf.Tag.Get(tagName)
	views := strings.Split(tag, ",")
	return field{sf.Name, sf.Index[0], tag, views}
}

type field struct {
	name  string
	index int
	tag   string
	views []string
}

func (f *field) isMatchView(viewName string) bool {
	for _, v := range f.views {
		if viewName == v {
			return true
		}
	}
	return false
}
