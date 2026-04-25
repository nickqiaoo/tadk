package llminternal

import (
	"fmt"
	"reflect"
)

// clone returns a deep copy of the src.
// NOTE: this does not work for types with unexported fields.
func clone[M any](src M) M {
	val := reflect.ValueOf(src)

	if val.Kind() == reflect.Ptr && val.IsNil() {
		var zero M
		return zero
	}

	srcIsPointer := val.Kind() == reflect.Ptr
	if srcIsPointer {
		val = val.Elem()
	}

	newVal := reflect.New(val.Type()).Elem()
	deepCopy(val, newVal)

	if srcIsPointer {
		return newVal.Addr().Interface().(M)
	}
	return newVal.Interface().(M)
}

func deepCopy(src, dst reflect.Value) {
	switch src.Kind() {
	case reflect.Struct:
		t := src.Type()
		for i := 0; i < src.NumField(); i++ {
			if !t.Field(i).IsExported() {
				panic(fmt.Sprintf("deepCopy: unexported field %q in type %q", t.Field(i).Name, t.Name()))
			}
			fieldCopy := reflect.New(src.Field(i).Type()).Elem()
			deepCopy(src.Field(i), fieldCopy)
			dst.Field(i).Set(fieldCopy)
		}
	case reflect.Slice:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeSlice(src.Type(), src.Len(), src.Cap()))
		for i := 0; i < src.Len(); i++ {
			elemCopy := reflect.New(src.Index(i).Type()).Elem()
			deepCopy(src.Index(i), elemCopy)
			dst.Index(i).Set(elemCopy)
		}
	case reflect.Map:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeMap(src.Type()))
		for _, key := range src.MapKeys() {
			keyCopy := reflect.New(key.Type()).Elem()
			deepCopy(key, keyCopy)
			valCopy := reflect.New(src.MapIndex(key).Type()).Elem()
			deepCopy(src.MapIndex(key), valCopy)
			dst.SetMapIndex(keyCopy, valCopy)
		}
	case reflect.Ptr:
		if src.IsNil() {
			return
		}
		newPtr := reflect.New(src.Elem().Type())
		deepCopy(src.Elem(), newPtr.Elem())
		dst.Set(newPtr)
	default:
		dst.Set(src)
	}
}
