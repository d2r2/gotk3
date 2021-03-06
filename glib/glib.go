// Copyright (c) 2013-2014 Conformal Systems <info@conformal.com>
//
// This file originated from: http://opensource.conformal.com/
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

// Package glib provides Go bindings for GLib 2.  Supports version 2.36
// and later.
package glib

// #cgo pkg-config: glib-2.0 gobject-2.0 gio-2.0
// #include <gio/gio.h>
// #include <glib.h>
// #include <glib-object.h>
// #include "glib.go.h"
import "C"

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

/*
 * Type conversions
 */

func gbool(b bool) C.gboolean {
	if b {
		return C.gboolean(1)
	}
	return C.gboolean(0)
}

func gobool(b C.gboolean) bool {
	if b != 0 {
		return true
	}
	return false
}

func goString(cstr *C.gchar) string {
	return C.GoString((*C.char)(cstr))
}

func goStringArray(c **C.gchar) []string {
	var strs []string

	for *c != nil {
		strs = append(strs, goString(*c))
		c = C.next_gcharptr(c)
	}

	return strs
}

/*
 * Unexported vars
 */

type closureContext struct {
	rf reflect.Value
	// allow arbitrary number of user extra args
	userData []reflect.Value
}

var (
	errNilPtr = errors.New("cgo returned unexpected nil pointer")

	closures = struct {
		sync.RWMutex
		m map[*C.GClosure]closureContext
	}{
		m: make(map[*C.GClosure]closureContext),
	}

	signals = make(map[SignalHandle]*C.GClosure)
)

/*
 * Constants
 */

// Type is a representation of GLib's GType.
type Type uint

const (
	TYPE_INVALID   Type = C.G_TYPE_INVALID
	TYPE_NONE      Type = C.G_TYPE_NONE
	TYPE_INTERFACE Type = C.G_TYPE_INTERFACE
	TYPE_CHAR      Type = C.G_TYPE_CHAR
	TYPE_UCHAR     Type = C.G_TYPE_UCHAR
	TYPE_BOOLEAN   Type = C.G_TYPE_BOOLEAN
	TYPE_INT       Type = C.G_TYPE_INT
	TYPE_UINT      Type = C.G_TYPE_UINT
	TYPE_LONG      Type = C.G_TYPE_LONG
	TYPE_ULONG     Type = C.G_TYPE_ULONG
	TYPE_INT64     Type = C.G_TYPE_INT64
	TYPE_UINT64    Type = C.G_TYPE_UINT64
	TYPE_ENUM      Type = C.G_TYPE_ENUM
	TYPE_FLAGS     Type = C.G_TYPE_FLAGS
	TYPE_FLOAT     Type = C.G_TYPE_FLOAT
	TYPE_DOUBLE    Type = C.G_TYPE_DOUBLE
	TYPE_STRING    Type = C.G_TYPE_STRING
	TYPE_POINTER   Type = C.G_TYPE_POINTER
	TYPE_BOXED     Type = C.G_TYPE_BOXED
	TYPE_PARAM     Type = C.G_TYPE_PARAM
	TYPE_OBJECT    Type = C.G_TYPE_OBJECT
	TYPE_VARIANT   Type = C.G_TYPE_VARIANT
)

// Name is a wrapper around g_type_name().
func (t Type) Name() string {
	c := C.g_type_name(C.GType(t))
	return goString(c)
}

// Depth is a wrapper around g_type_depth().
func (t Type) Depth() uint {
	return uint(C.g_type_depth(C.GType(t)))
}

// Parent is a wrapper around g_type_parent().
func (t Type) Parent() Type {
	return Type(C.g_type_parent(C.GType(t)))
}

// UserDirectory is a representation of GLib's GUserDirectory.
type UserDirectory int

const (
	USER_DIRECTORY_DESKTOP      UserDirectory = C.G_USER_DIRECTORY_DESKTOP
	USER_DIRECTORY_DOCUMENTS    UserDirectory = C.G_USER_DIRECTORY_DOCUMENTS
	USER_DIRECTORY_DOWNLOAD     UserDirectory = C.G_USER_DIRECTORY_DOWNLOAD
	USER_DIRECTORY_MUSIC        UserDirectory = C.G_USER_DIRECTORY_MUSIC
	USER_DIRECTORY_PICTURES     UserDirectory = C.G_USER_DIRECTORY_PICTURES
	USER_DIRECTORY_PUBLIC_SHARE UserDirectory = C.G_USER_DIRECTORY_PUBLIC_SHARE
	USER_DIRECTORY_TEMPLATES    UserDirectory = C.G_USER_DIRECTORY_TEMPLATES
	USER_DIRECTORY_VIDEOS       UserDirectory = C.G_USER_DIRECTORY_VIDEOS
)

const USER_N_DIRECTORIES int = C.G_USER_N_DIRECTORIES

/*
 * GApplicationFlags
 */

type ApplicationFlags int

const (
	APPLICATION_FLAGS_NONE           ApplicationFlags = C.G_APPLICATION_FLAGS_NONE
	APPLICATION_IS_SERVICE           ApplicationFlags = C.G_APPLICATION_IS_SERVICE
	APPLICATION_HANDLES_OPEN         ApplicationFlags = C.G_APPLICATION_HANDLES_OPEN
	APPLICATION_HANDLES_COMMAND_LINE ApplicationFlags = C.G_APPLICATION_HANDLES_COMMAND_LINE
	APPLICATION_SEND_ENVIRONMENT     ApplicationFlags = C.G_APPLICATION_SEND_ENVIRONMENT
	APPLICATION_NON_UNIQUE           ApplicationFlags = C.G_APPLICATION_NON_UNIQUE
)

// goMarshal is called by the GLib runtime when a closure needs to be invoked.
// The closure will be invoked with as many arguments as it can take, from 0 to
// the full amount provided by the call. If the closure asks for more parameters
// than there are to give, a warning is printed to stderr and the closure is
// not run.
//
//export goMarshal
func goMarshal(closure *C.GClosure, retValue *C.GValue,
	nParams C.guint, params *C.GValue,
	invocationHint C.gpointer, marshalData *C.GValue) {

	// Get the context associated with this callback closure.
	closures.RLock()
	cc := closures.m[closure]
	closures.RUnlock()

	// Get number of parameters passed in.  If user data was saved with the
	// closure context, increment the total number of parameters.
	nGLibParams := int(nParams)
	nTotalParams := nGLibParams
	nTotalParams += len(cc.userData)

	// Get number of parameters from the callback closure.  If this exceeds
	// the total number of marshaled parameters, a warning will be printed
	// to stderr, and the callback will not be run.
	nCbParams := cc.rf.Type().NumIn()
	if nCbParams > nTotalParams {
		fmt.Fprintf(os.Stderr,
			"too many closure args: have %d, max allowed %d\n",
			nCbParams, nTotalParams)
		return
	}

	// Create a slice of reflect.Values as arguments to call the function.
	gValues := gValueSlice(params, nCbParams)
	args := make([]reflect.Value, 0, nCbParams)

	// Fill beginning of args, up to the minimum of the total number of callback
	// parameters and parameters from the glib runtime.
	for i := 0; i < nCbParams && i < nGLibParams; i++ {
		v := &Value{&gValues[i]}
		val, err := v.GoValue()
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"no suitable Go value for arg %d: %v\n", i, err)
			return
		}
		rv := reflect.ValueOf(val)
		args = append(args, rv.Convert(cc.rf.Type().In(i)))
	}

	// If non-nil user data was passed in and not all args have been set,
	// get and set the reflect.Value directly from the GValue.
	//if cc.userData.IsValid() && len(args) < cap(args) {
	if len(cc.userData) > 0 && len(args) < cap(args) {
		for i, v := range cc.userData {
			args = append(args, v.Convert(cc.rf.Type().In(nCbParams-len(cc.userData)+i)))
		}
	}

	// Call closure with args. If the callback returns one or more
	// values, save the GValue equivalent of the first.
	rv := cc.rf.Call(args)
	if retValue != nil && len(rv) > 0 {
		if g, err := GValue(rv[0].Interface()); err != nil {
			fmt.Fprintf(os.Stderr,
				"cannot save callback return value: %v", err)
		} else {
			*retValue = *g.native()
		}
	}
}

// gValueSlice converts a C array of GValues to a Go slice.
func gValueSlice(values *C.GValue, nValues int) (slice []C.GValue) {
	header := (*reflect.SliceHeader)((unsafe.Pointer(&slice)))
	header.Cap = nValues
	header.Len = nValues
	header.Data = uintptr(unsafe.Pointer(values))
	return
}

/*
 * Main event loop
 */

type SourceHandle uint

// IdleAdd adds an idle source to the default main event loop
// context.  After running once, the source func will be removed
// from the main event loop, unless f returns a single bool true.
//
// This function will cause a panic when f eventually runs if the
// types of args do not match those of f.
func IdleAdd(f interface{}, args ...interface{}) (SourceHandle, error) {
	// f must be a func with no parameters.
	rf := reflect.ValueOf(f)
	if rf.Type().Kind() != reflect.Func {
		return 0, errors.New("f is not a function")
	}

	// Create an idle source func to be added to the main loop context.
	idleSrc := C.g_idle_source_new()
	if idleSrc == nil {
		return 0, errNilPtr
	}
	return sourceAttach(idleSrc, rf, args...)
}

// TimeoutAdd adds an timeout source to the default main event loop
// context.  After running once, the source func will be removed
// from the main event loop, unless f returns a single bool true.
//
// This function will cause a panic when f eventually runs if the
// types of args do not match those of f.
// timeout is in milliseconds
func TimeoutAdd(timeout uint, f interface{}, args ...interface{}) (SourceHandle, error) {
	// f must be a func with no parameters.
	rf := reflect.ValueOf(f)
	if rf.Type().Kind() != reflect.Func {
		return 0, errors.New("f is not a function")
	}

	// Create a timeout source func to be added to the main loop context.
	timeoutSrc := C.g_timeout_source_new(C.guint(timeout))
	if timeoutSrc == nil {
		return 0, errNilPtr
	}

	return sourceAttach(timeoutSrc, rf, args...)
}

// sourceAttach attaches a source to the default main loop context.
func sourceAttach(src *C.GSource, rf reflect.Value, args ...interface{}) (SourceHandle, error) {
	if src == nil {
		return 0, errNilPtr
	}

	// rf must be a func with no parameters.
	if rf.Type().Kind() != reflect.Func {
		C.g_source_destroy(src)
		return 0, errors.New("rf is not a function")
	}

	// Create a new GClosure from f that invalidates itself when
	// f returns false.  The error is ignored here, as this will
	// always be a function.
	var closure *C.GClosure
	closure, _ = ClosureNew(func() {
		// Create a slice of reflect.Values arguments to call the func.
		rargs := make([]reflect.Value, len(args))
		for i := range args {
			rargs[i] = reflect.ValueOf(args[i])
		}

		// Call func with args. The callback will be removed, unless
		// it returns exactly one return value of true.
		rv := rf.Call(rargs)
		if len(rv) == 1 {
			if rv[0].Kind() == reflect.Bool {
				if rv[0].Bool() {
					return
				}
			}
		}
		C.g_closure_invalidate(closure)
		C.g_source_destroy(src)
	})

	// Remove closure context when closure is finalized.
	C._g_closure_add_finalize_notifier(closure)

	// Set closure to run as a callback when the idle source runs.
	C.g_source_set_closure(src, closure)

	// Attach the idle source func to the default main event loop
	// context.
	cid := C.g_source_attach(src, nil)
	return SourceHandle(cid), nil
}

/*
 * Miscellaneous Utility Functions
 */

// GetHomeDir is a wrapper around g_get_home_dir().
func GetHomeDir() string {
	c := C.g_get_home_dir()
	return goString(c)
}

// GetUserCacheDir is a wrapper around g_get_user_cache_dir().
func GetUserCacheDir() string {
	c := C.g_get_user_cache_dir()
	return goString(c)
}

// GetUserDataDir is a wrapper around g_get_user_data_dir().
func GetUserDataDir() string {
	c := C.g_get_user_data_dir()
	return goString(c)
}

// GetUserConfigDir is a wrapper around g_get_user_config_dir().
func GetUserConfigDir() string {
	c := C.g_get_user_config_dir()
	return goString(c)
}

// GetUserRuntimeDir is a wrapper around g_get_user_runtime_dir().
func GetUserRuntimeDir() string {
	c := C.g_get_user_runtime_dir()
	return goString(c)
}

// GetUserSpecialDir is a wrapper around g_get_user_special_dir().  A
// non-nil error is returned in the case that g_get_user_special_dir()
// returns NULL to differentiate between NULL and an empty string.
func GetUserSpecialDir(directory UserDirectory) (string, error) {
	c := C.g_get_user_special_dir(C.GUserDirectory(directory))
	if c == nil {
		return "", errNilPtr
	}
	return goString(c), nil
}

/*
 * GInitiallyUnowned
 */

// InitiallyUnowned is a representation of GLib's GInitiallyUnowned.
type InitiallyUnowned struct {
	// This must be a pointer so copies of the ref-sinked object
	// do not outlive the original object, causing an unref
	// finalizer to prematurely run.
	*Object
}

/*
 * GValue
 */

// Value is a representation of GLib's GValue.
//
// Don't allocate Values on the stack or heap manually as they may not
// be properly unset when going out of scope. Instead, use ValueAlloc(),
// which will set the runtime finalizer to unset the Value after it has
// left scope.
type Value struct {
	gvalue *C.GValue
}

// native returns a pointer to the underlying GValue.
func (v *Value) native() *C.GValue {
	return v.gvalue
}

// Native returns a pointer to the underlying GValue.
func (v *Value) Native() unsafe.Pointer {
	return unsafe.Pointer(v.native())
}

// ValueAlloc allocates a Value and sets a runtime finalizer to call
// g_value_unset() on the underlying GValue after leaving scope.
// ValueAlloc() returns a non-nil error if the allocation failed.
func ValueAlloc() (*Value, error) {
	c := C._g_value_alloc()
	if c == nil {
		return nil, errNilPtr
	}

	v := &Value{c}

	//An allocated GValue is not guaranteed to hold a value that can be unset
	//We need to double check before unsetting, to prevent:
	//`g_value_unset: assertion 'G_IS_VALUE (value)' failed`
	runtime.SetFinalizer(v, func(f *Value) {
		if t, _, err := f.Type(); err != nil || t == TYPE_INVALID || t == TYPE_NONE {
			C.g_free(C.gpointer(f.native()))
		}

		f.unset()
	})

	return v, nil
}

// ValueInit is a wrapper around g_value_init() and allocates and
// initializes a new Value with the Type t.  A runtime finalizer is set
// to call g_value_unset() on the underlying GValue after leaving scope.
// ValueInit() returns a non-nil error if the allocation failed.
func ValueInit(t Type) (*Value, error) {
	c := C._g_value_init(C.GType(t))
	if c == nil {
		return nil, errNilPtr
	}

	v := &Value{c}

	runtime.SetFinalizer(v, (*Value).unset)
	return v, nil
}

// ValueFromNative returns a type-asserted pointer to the Value.
func ValueFromNative(l unsafe.Pointer) *Value {
	//TODO why it does not add finalizer to the value?
	return &Value{(*C.GValue)(l)}
}

func (v *Value) unset() {
	C.g_value_unset(v.native())
}

// Type is a wrapper around the G_VALUE_HOLDS_GTYPE() macro and
// the g_value_get_gtype() function.  GetType() returns TYPE_INVALID if v
// does not hold a Type, or otherwise returns the Type of v.
func (v *Value) Type() (actual Type, fundamental Type, err error) {
	if !gobool(C._g_is_value(v.native())) {
		return actual, fundamental, errors.New("invalid GValue")
	}
	cActual := C._g_value_type(v.native())
	cFundamental := C._g_value_fundamental(cActual)
	return Type(cActual), Type(cFundamental), nil
}

// GValue converts a Go type to a comparable GValue.  GValue()
// returns a non-nil error if the conversion was unsuccessful.
func GValue(v interface{}) (gvalue *Value, err error) {
	if v == nil {
		val, err := ValueInit(TYPE_POINTER)
		if err != nil {
			return nil, err
		}
		val.SetPointer(uintptr(unsafe.Pointer(nil)))
		return val, nil
	}

	switch e := v.(type) {
	case bool:
		val, err := ValueInit(TYPE_BOOLEAN)
		if err != nil {
			return nil, err
		}
		val.SetBool(e)
		return val, nil

	case int8:
		val, err := ValueInit(TYPE_CHAR)
		if err != nil {
			return nil, err
		}
		val.SetSChar(e)
		return val, nil

	case int64:
		val, err := ValueInit(TYPE_INT64)
		if err != nil {
			return nil, err
		}
		val.SetInt64(e)
		return val, nil

	case int:
		val, err := ValueInit(TYPE_INT)
		if err != nil {
			return nil, err
		}
		val.SetInt(e)
		return val, nil

	case uint8:
		val, err := ValueInit(TYPE_UCHAR)
		if err != nil {
			return nil, err
		}
		val.SetUChar(e)
		return val, nil

	case uint64:
		val, err := ValueInit(TYPE_UINT64)
		if err != nil {
			return nil, err
		}
		val.SetUInt64(e)
		return val, nil

	case uint:
		val, err := ValueInit(TYPE_UINT)
		if err != nil {
			return nil, err
		}
		val.SetUInt(e)
		return val, nil

	case float32:
		val, err := ValueInit(TYPE_FLOAT)
		if err != nil {
			return nil, err
		}
		val.SetFloat(e)
		return val, nil

	case float64:
		val, err := ValueInit(TYPE_DOUBLE)
		if err != nil {
			return nil, err
		}
		val.SetDouble(e)
		return val, nil

	case string:
		val, err := ValueInit(TYPE_STRING)
		if err != nil {
			return nil, err
		}
		val.SetString(e)
		return val, nil

	case *Object:
		val, err := ValueInit(TYPE_OBJECT)
		if err != nil {
			return nil, err
		}
		val.SetInstance(e.Native())
		return val, nil

	default:
		/* Try this since above doesn't catch constants under other types */
		rval := reflect.ValueOf(v)
		switch rval.Kind() {
		case reflect.Int8:
			val, err := ValueInit(TYPE_CHAR)
			if err != nil {
				return nil, err
			}
			val.SetSChar(int8(rval.Int()))
			return val, nil

		case reflect.Int16:
			return nil, errors.New("Type not implemented")

		case reflect.Int32:
			return nil, errors.New("Type not implemented")

		case reflect.Int64:
			val, err := ValueInit(TYPE_INT64)
			if err != nil {
				return nil, err
			}
			val.SetInt64(rval.Int())
			return val, nil

		case reflect.Int:
			val, err := ValueInit(TYPE_INT)
			if err != nil {
				return nil, err
			}
			val.SetInt(int(rval.Int()))
			return val, nil

		case reflect.Uintptr, reflect.Ptr:
			val, err := ValueInit(TYPE_POINTER)
			if err != nil {
				return nil, err
			}
			val.SetPointer(rval.Pointer())
			return val, nil
		}
	}

	return nil, errors.New("Type not implemented")
}

// GValueMarshaler is a marshal function to convert a GValue into an
// appropriate Go type.  The uintptr parameter is a *C.GValue.
type GValueMarshaler func(uintptr) (interface{}, error)

// TypeMarshaler represents an actual type and it's associated marshaler.
type TypeMarshaler struct {
	T Type
	F GValueMarshaler
}

// RegisterGValueMarshalers adds marshalers for several types to the
// internal marshalers map.  Once registered, calling GoValue on any
// Value witha registered type will return the data returned by the
// marshaler.
func RegisterGValueMarshalers(tm []TypeMarshaler) {
	gValueMarshalers.register(tm)
}

type marshalMap map[Type]GValueMarshaler

// gValueMarshalers is a map of Glib types to functions to marshal a
// GValue to a native Go type.
var gValueMarshalers = marshalMap{
	TYPE_INVALID:   marshalInvalid,
	TYPE_NONE:      marshalNone,
	TYPE_INTERFACE: marshalInterface,
	TYPE_CHAR:      marshalChar,
	TYPE_UCHAR:     marshalUchar,
	TYPE_BOOLEAN:   marshalBoolean,
	TYPE_INT:       marshalInt,
	TYPE_LONG:      marshalLong,
	TYPE_ENUM:      marshalEnum,
	TYPE_INT64:     marshalInt64,
	TYPE_UINT:      marshalUint,
	TYPE_ULONG:     marshalUlong,
	TYPE_FLAGS:     marshalFlags,
	TYPE_UINT64:    marshalUint64,
	TYPE_FLOAT:     marshalFloat,
	TYPE_DOUBLE:    marshalDouble,
	TYPE_STRING:    marshalString,
	TYPE_POINTER:   marshalPointer,
	TYPE_BOXED:     marshalBoxed,
	TYPE_OBJECT:    marshalObject,
	TYPE_VARIANT:   marshalVariant,
}

func (m marshalMap) register(tm []TypeMarshaler) {
	for i := range tm {
		m[tm[i].T] = tm[i].F
	}
}

func (m marshalMap) lookup(v *Value) (GValueMarshaler, error) {
	actual, fundamental, err := v.Type()
	if err != nil {
		return nil, err
	}

	if f, ok := m[actual]; ok {
		return f, nil
	}
	if f, ok := m[fundamental]; ok {
		return f, nil
	}
	return nil, errors.New("missing marshaler for type")
}

func marshalInvalid(uintptr) (interface{}, error) {
	return nil, errors.New("invalid type")
}

func marshalNone(uintptr) (interface{}, error) {
	return nil, nil
}

func marshalInterface(uintptr) (interface{}, error) {
	return nil, errors.New("interface conversion not yet implemented")
}

func marshalChar(p uintptr) (interface{}, error) {
	c := C.g_value_get_schar((*C.GValue)(unsafe.Pointer(p)))
	return int8(c), nil
}

func marshalUchar(p uintptr) (interface{}, error) {
	c := C.g_value_get_uchar((*C.GValue)(unsafe.Pointer(p)))
	return uint8(c), nil
}

func marshalBoolean(p uintptr) (interface{}, error) {
	c := C.g_value_get_boolean((*C.GValue)(unsafe.Pointer(p)))
	return gobool(c), nil
}

func marshalInt(p uintptr) (interface{}, error) {
	c := C.g_value_get_int((*C.GValue)(unsafe.Pointer(p)))
	return int(c), nil
}

func marshalLong(p uintptr) (interface{}, error) {
	c := C.g_value_get_long((*C.GValue)(unsafe.Pointer(p)))
	return int(c), nil
}

func marshalEnum(p uintptr) (interface{}, error) {
	c := C.g_value_get_enum((*C.GValue)(unsafe.Pointer(p)))
	return int(c), nil
}

func marshalInt64(p uintptr) (interface{}, error) {
	c := C.g_value_get_int64((*C.GValue)(unsafe.Pointer(p)))
	return int64(c), nil
}

func marshalUint(p uintptr) (interface{}, error) {
	c := C.g_value_get_uint((*C.GValue)(unsafe.Pointer(p)))
	return uint(c), nil
}

func marshalUlong(p uintptr) (interface{}, error) {
	c := C.g_value_get_ulong((*C.GValue)(unsafe.Pointer(p)))
	return uint(c), nil
}

func marshalFlags(p uintptr) (interface{}, error) {
	c := C.g_value_get_flags((*C.GValue)(unsafe.Pointer(p)))
	return uint(c), nil
}

func marshalUint64(p uintptr) (interface{}, error) {
	c := C.g_value_get_uint64((*C.GValue)(unsafe.Pointer(p)))
	return uint64(c), nil
}

func marshalFloat(p uintptr) (interface{}, error) {
	c := C.g_value_get_float((*C.GValue)(unsafe.Pointer(p)))
	return float32(c), nil
}

func marshalDouble(p uintptr) (interface{}, error) {
	c := C.g_value_get_double((*C.GValue)(unsafe.Pointer(p)))
	return float64(c), nil
}

func marshalString(p uintptr) (interface{}, error) {
	c := C.g_value_get_string((*C.GValue)(unsafe.Pointer(p)))
	return goString(c), nil
}

func marshalBoxed(p uintptr) (interface{}, error) {
	c := C.g_value_get_boxed((*C.GValue)(unsafe.Pointer(p)))
	return uintptr(unsafe.Pointer(c)), nil
}

func marshalPointer(p uintptr) (interface{}, error) {
	c := C.g_value_get_pointer((*C.GValue)(unsafe.Pointer(p)))
	return unsafe.Pointer(c), nil
}

func marshalObject(p uintptr) (interface{}, error) {
	c := C.g_value_get_object((*C.GValue)(unsafe.Pointer(p)))
	return newObject((*C.GObject)(c)), nil
}

func marshalVariant(p uintptr) (interface{}, error) {
	c := C.g_value_get_variant((*C.GValue)(unsafe.Pointer(p)))
	return newVariant((*C.GVariant)(c)), nil
	//	return nil, errors.New("variant conversion not yet implemented")
}

// GoValue converts a Value to comparable Go type.  GoValue()
// returns a non-nil error if the conversion was unsuccessful.  The
// returned interface{} must be type asserted as the actual Go
// representation of the Value.
//
// This function is a wrapper around the many g_value_get_*()
// functions, depending on the type of the Value.
func (v *Value) GoValue() (interface{}, error) {
	f, err := gValueMarshalers.lookup(v)
	if err != nil {
		return nil, err
	}

	//No need to add finalizer because it is already done by ValueAlloc and ValueInit
	rv, err := f(uintptr(unsafe.Pointer(v.native())))
	return rv, err
}

// SetBool is a wrapper around g_value_set_boolean().
func (v *Value) SetBool(val bool) {
	C.g_value_set_boolean(v.native(), gbool(val))
}

// SetSChar is a wrapper around g_value_set_schar().
func (v *Value) SetSChar(val int8) {
	C.g_value_set_schar(v.native(), C.gint8(val))
}

// SetInt64 is a wrapper around g_value_set_int64().
func (v *Value) SetInt64(val int64) {
	C.g_value_set_int64(v.native(), C.gint64(val))
}

// SetInt is a wrapper around g_value_set_int().
func (v *Value) SetInt(val int) {
	C.g_value_set_int(v.native(), C.gint(val))
}

// SetUChar is a wrapper around g_value_set_uchar().
func (v *Value) SetUChar(val uint8) {
	C.g_value_set_uchar(v.native(), C.guchar(val))
}

// SetUInt64 is a wrapper around g_value_set_uint64().
func (v *Value) SetUInt64(val uint64) {
	C.g_value_set_uint64(v.native(), C.guint64(val))
}

// SetUInt is a wrapper around g_value_set_uint().
func (v *Value) SetUInt(val uint) {
	C.g_value_set_uint(v.native(), C.guint(val))
}

// SetFloat is a wrapper around g_value_set_float().
func (v *Value) SetFloat(val float32) {
	C.g_value_set_float(v.native(), C.gfloat(val))
}

// SetDouble is a wrapper around g_value_set_double().
func (v *Value) SetDouble(val float64) {
	C.g_value_set_double(v.native(), C.gdouble(val))
}

// SetString is a wrapper around g_value_set_string().
func (v *Value) SetString(val string) {
	cstr := C.CString(val)
	defer C.free(unsafe.Pointer(cstr))
	C.g_value_set_string(v.native(), (*C.gchar)(cstr))
}

// SetInstance is a wrapper around g_value_set_instance().
func (v *Value) SetInstance(instance uintptr) {
	C.g_value_set_instance(v.native(), C.gpointer(instance))
}

// SetPointer is a wrapper around g_value_set_pointer().
func (v *Value) SetPointer(p uintptr) {
	C.g_value_set_pointer(v.native(), C.gpointer(p))
}

// GetPointer is a wrapper around g_value_get_pointer().
func (v *Value) GetPointer() unsafe.Pointer {
	return unsafe.Pointer(C.g_value_get_pointer(v.native()))
}

// GetString is a wrapper around g_value_get_string().  GetString()
// returns a non-nil error if g_value_get_string() returned a NULL
// pointer to distinguish between returning a NULL pointer and returning
// an empty string.
func (v *Value) GetString() (string, error) {
	c := C.g_value_get_string(v.native())
	if c == nil {
		return "", errNilPtr
	}
	return goString(c), nil
}

type Signal struct {
	name     string
	signalId C.guint
}

func SignalNew(s string) (*Signal, error) {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))

	signalId := C._g_signal_new((*C.gchar)(cstr))

	if signalId == 0 {
		return nil, fmt.Errorf("invalid signal name: %s", s)
	}

	return &Signal{
		name:     s,
		signalId: signalId,
	}, nil
}

func (s *Signal) String() string {
	return s.name
}

type Quark uint32

// GetApplicationName is a wrapper around g_get_application_name().
func GetApplicationName() string {
	c := C.g_get_application_name()

	return goString(c)
}

// SetApplicationName is a wrapper around g_set_application_name().
func SetApplicationName(name string) {
	cstr := C.CString(name)
	defer C.free(unsafe.Pointer(cstr))

	C.g_set_application_name((*C.gchar)(cstr))
}

// InitI18n initializes the i18n subsystem.
func InitI18n(domain string, dir string) {
	domainStr := C.CString(domain)
	defer C.free(unsafe.Pointer(domainStr))

	dirStr := C.CString(dir)
	defer C.free(unsafe.Pointer(dirStr))

	C.init_i18n(domainStr, dirStr)
}

// Local localizes a string using gettext
func Local(input string) string {
	cstr := C.CString(input)
	defer C.free(unsafe.Pointer(cstr))

	c := C.localize(cstr)
	return goString((*C.gchar)(c))
}
