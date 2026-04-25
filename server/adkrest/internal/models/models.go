// Package models defines the data structures for the REST API.
package models

import "reflect"

// IsZeroValue checks if the val is the zero-ed value.
func IsZeroValue(val any) bool {
	return val == nil || reflect.DeepEqual(val, reflect.Zero(reflect.TypeOf(val)).Interface())
}
