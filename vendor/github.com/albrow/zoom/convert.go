// Copyright 2015 Alex Browne.  All rights reserved.
// Use of this source code is governed by the MIT
// license, which can be found in the LICENSE file.

// File scan.go contains code that converts go data structures
// to and from a format that redis can understand

package zoom

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/garyburd/redigo/redis"
)

// scanModel iterates through fieldValues, converts each value to the correct type, and
// scans the value into the fields of mr.model. It expects fieldValues to be the output
// from an HMGET command from redis, without the field names included. The order of the
// values in fieldValues must match the order of the corresponding field names. The id
// field is special and should have the field name "-", which will be set with the SetModelID
// method. fieldNames should be the actual field names as they appear in the struct definition,
// not the redis names which may be custom.
func scanModel(fieldNames []string, fieldValues []interface{}, mr *modelRef) error {
	ms := mr.spec
	if fieldValues == nil || len(fieldValues) == 0 {
		return newModelNotFoundError(mr)
	}
	for i, reply := range fieldValues {
		if reply == nil {
			continue
		}
		fieldName := fieldNames[i]
		replyBytes, err := redis.Bytes(reply, nil)
		if err != nil {
			return err
		}
		if fieldName == "-" {
			// The ID is signified by the field name "-" since that cannot
			// possibly collide with other field names.
			mr.model.SetModelID(string(replyBytes))
			continue
		}
		fs, found := ms.fieldsByName[fieldName]
		if !found {
			return fmt.Errorf("zoom: Error in scanModel: Could not find field %s in %T", fieldName, mr.model)
		}
		fieldVal := mr.fieldValue(fieldName)
		switch fs.kind {
		case primativeField:
			if err := scanPrimitiveVal(replyBytes, fieldVal); err != nil {
				return err
			}
		case pointerField:
			if err := scanPointerVal(replyBytes, fieldVal); err != nil {
				return err
			}
		default:
			if err := scanInconvertibleVal(mr.spec.fallback, replyBytes, fieldVal); err != nil {
				return err
			}
		}
	}
	return nil
}

// scanPrimitiveVal converts a slice of bytes response from redis into the type of dest
// and then sets dest to that value
func scanPrimitiveVal(src []byte, dest reflect.Value) error {
	if len(src) == 0 {
		return nil // skip blanks
	}
	switch dest.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		srcInt, err := strconv.ParseInt(string(src), 10, 0)
		if err != nil {
			return fmt.Errorf("zoom: could not convert %s to int", string(src))
		}
		dest.SetInt(srcInt)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		srcUint, err := strconv.ParseUint(string(src), 10, 0)
		if err != nil {
			return fmt.Errorf("zoom: could not convert %s to uint", string(src))
		}
		dest.SetUint(srcUint)

	case reflect.Float32, reflect.Float64:
		srcFloat, err := strconv.ParseFloat(string(src), 64)
		if err != nil {
			return fmt.Errorf("zoom: could not convert %s to float", string(src))
		}
		dest.SetFloat(srcFloat)
	case reflect.Bool:
		srcBool, err := strconv.ParseBool(string(src))
		if err != nil {
			return fmt.Errorf("zoom: could not convert %s to bool", string(src))
		}
		dest.SetBool(srcBool)
	case reflect.String:
		dest.SetString(string(src))
	case reflect.Slice, reflect.Array:
		// Slice or array of bytes
		dest.SetBytes(src)
	default:
		return fmt.Errorf("zoom: don't know how to scan primitive type: %T", src)
	}
	return nil
}

// scanPointerVal works like scanVal but expects dest to be a pointer to some
// primitive type
func scanPointerVal(src []byte, dest reflect.Value) error {
	// Skip empty or nil fields
	if string(src) == "NULL" {
		return nil
	}
	dest.Set(reflect.New(dest.Type().Elem()))
	return scanPrimitiveVal(src, dest.Elem())
}

// scanIncovertibleVal unmarshals src into dest using the given
// MarshalerUnmarshaler
func scanInconvertibleVal(marshalerUnmarshaler MarshalerUnmarshaler, src []byte, dest reflect.Value) error {
	// Skip empty or nil fields
	if len(src) == 0 || string(src) == "NULL" {
		return nil
	}
	// TODO: account for json, msgpack or other custom fallbacks
	if err := marshalerUnmarshaler.Unmarshal(src, dest.Addr().Interface()); err != nil {
		return err
	}
	return nil
}
