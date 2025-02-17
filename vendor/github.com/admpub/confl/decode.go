package confl

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/admpub/map2struct"
)

var e = fmt.Errorf

// Primitive is a value that hasn't been decoded into a Go value.
// When using the various `Decode*` functions, the type `Primitive` may
// be given to any value, and its decoding will be delayed.
//
// A `Primitive` value can be decoded using the `PrimitiveDecode` function.
//
// The underlying representation of a `Primitive` value is subject to change.
// Do not rely on it.
//
// N.B. Primitive values are still parsed, so using them will only avoid
// the overhead of reflection. They can be useful when you don't know the
// exact type of data until run time.
type Primitive struct {
	undecoded interface{}
	context   Key
}

// PrimitiveDecode is just like the other `Decode*` functions, except it
// decodes a confl value that has already been parsed. Valid primitive values
// can *only* be obtained from values filled by the decoder functions,
// including this method. (i.e., `v` may contain more `Primitive`
// values.)
//
// Meta data for primitive values is included in the meta data returned by
// the `Decode*` functions with one exception: keys returned by the Undecoded
// method will only reflect keys that were decoded. Namely, any keys hidden
// behind a Primitive will be considered undecoded. Executing this method will
// update the undecoded keys in the meta data. (See the example.)
func (md *MetaData) PrimitiveDecode(primValue Primitive, v interface{}) error {
	md.context = primValue.context
	defer func() { md.context = nil }()
	return md.unify(primValue.undecoded, rvalue(v))
}

type Decoder struct {
	reader io.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r}
}

func (dec *Decoder) Decode(v interface{}) error {
	bs, err := ioutil.ReadAll(dec.reader)
	if err != nil {
		return err
	}
	_, err = Decode(string(bs), v)
	return err
}

func Unmarshal(bs []byte, v interface{}) error {
	_, err := Decode(string(bs), v)
	return err
}

// Decode will decode the contents of `data` in confl format into a pointer
// `v`.
//
// confl hashes correspond to Go structs or maps. (Dealer's choice. They can be
// used interchangeably.)
//
// confl arrays of tables correspond to either a slice of structs or a slice
// of maps.
//
// confl datetimes correspond to Go `time.Time` values.
//
// All other confl types (float, string, int, bool and array) correspond
// to the obvious Go types.
//
// An exception to the above rules is if a type implements the
// encoding.TextUnmarshaler interface. In this case, any primitive confl value
// (floats, strings, integers, booleans and datetimes) will be converted to
// a byte string and given to the value's UnmarshalText method. See the
// Unmarshaler example for a demonstration with time duration strings.
//
// Key mapping
//
// confl keys can map to either keys in a Go map or field names in a Go
// struct. The special `confl` struct tag may be used to map confl keys to
// struct fields that don't match the key name exactly. (See the example.)
// A case insensitive match to struct names will be tried if an exact match
// can't be found.
//
// The mapping between confl values and Go values is loose. That is, there
// may exist confl values that cannot be placed into your representation, and
// there may be parts of your representation that do not correspond to
// confl values. This loose mapping can be made stricter by using the IsDefined
// and/or Undecoded methods on the MetaData returned.
//
// This decoder will not handle cyclic types. If a cyclic type is passed,
// `Decode` will not terminate.
func Decode(data string, v interface{}) (MetaData, error) {
	p, err := parse(data)
	if err != nil {
		return MetaData{}, err
	}
	md := MetaData{
		p.mapping, p.types, p.ordered,
		make(map[string]bool, len(p.ordered)), nil,
	}
	return md, md.unify(p.mapping, rvalue(v))
}

// DecodeFile is just like Decode, except it will automatically read the
// contents of the file at `fpath` and decode it for you.
func DecodeFile(fpath string, v interface{}) (MetaData, error) {
	bs, err := ioutil.ReadFile(fpath)
	if err != nil {
		return MetaData{}, err
	}
	return Decode(string(bs), v)
}

// DecodeReader is just like Decode, except it will consume all bytes
// from the reader and decode it for you.
func DecodeReader(r io.Reader, v interface{}) (MetaData, error) {
	bs, err := ioutil.ReadAll(r)
	if err != nil {
		return MetaData{}, err
	}
	return Decode(string(bs), v)
}

// unify performs a sort of type unification based on the structure of `rv`,
// which is the client representation.
//
// Any type mismatch produces an error. Finding a type that we don't know
// how to handle produces an unsupported type error.
func (md *MetaData) unify(data interface{}, rv reflect.Value) error {
	// Special case. Look for a `Primitive` value.
	if rv.Type() == reflect.TypeOf((*Primitive)(nil)).Elem() {
		// Save the undecoded data and the key context into the primitive
		// value.
		context := make(Key, len(md.context))
		copy(context, md.context)
		rv.Set(reflect.ValueOf(Primitive{
			undecoded: data,
			context:   context,
		}))
		return nil
	}

	// Special case. Handle time.Time values specifically.
	// TODO: Remove this code when we decide to drop support for Go 1.1.
	// This isn't necessary in Go 1.2 because time.Time satisfies the encoding
	// interfaces.
	if rv.Type().AssignableTo(rvalue(time.Time{}).Type()) {
		return md.unifyDatetime(data, rv)
	}

	// Special case. Look for a value satisfying the TextUnmarshaler interface.
	if v, ok := rv.Interface().(TextUnmarshaler); ok {
		return md.unifyText(data, v)
	}
	// BUG(burntsushi)
	// The behavior here is incorrect whenever a Go type satisfies the
	// encoding.TextUnmarshaler interface but also corresponds to a conf
	// hash or array. In particular, the unmarshaler should only be applied
	// to primitive conf values. But at this point, it will be applied to
	// all kinds of values and produce an incorrect error whenever those values
	// are hashes or arrays (including arrays of tables).

	k := rv.Kind()

	// laziness
	if k >= reflect.Int && k <= reflect.Uint64 {
		return md.unifyInt(data, rv)
	}
	switch k {
	case reflect.Ptr:
		elem := reflect.New(rv.Type().Elem())
		err := md.unify(data, reflect.Indirect(elem))
		if err != nil {
			return err
		}
		rv.Set(elem)
		return nil
	case reflect.Struct:
		return md.unifyStruct(data, rv)
	case reflect.Map:
		return md.unifyMap(data, rv)
	case reflect.Array:
		return md.unifyArray(data, rv)
	case reflect.Slice:
		return md.unifySlice(data, rv)
	case reflect.String:
		return md.unifyString(data, rv)
	case reflect.Bool:
		return md.unifyBool(data, rv)
	case reflect.Interface:
		// we only support empty interfaces.
		if rv.NumMethod() > 0 {
			return e("Unsupported type '%s'.", rv.Kind())
		}
		return md.unifyAnything(data, rv)
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		return md.unifyFloat64(data, rv)
	}
	return e("Unsupported type '%s'.", rv.Kind())
}

func (md *MetaData) unifyStruct(mapping interface{}, rv reflect.Value) error {
	tmap, ok := mapping.(map[string]interface{})
	if !ok {
		return mismatch(rv, "map", mapping)
	}

	for key, datum := range tmap {
		var f *field
		fields := cachedTypeFields(rv.Type())
		for i := range fields {
			ff := &fields[i]
			if ff.name == key {
				f = ff
				break
			}
			if f == nil && strings.EqualFold(ff.name, key) {
				f = ff
			}
		}
		if f != nil {
			subv := rv
			for _, i := range f.index {
				subv = indirect(subv.Field(i))
			}
			if isUnifiable(subv) {
				md.decoded[md.context.add(key).String()] = true
				md.context = append(md.context, key)
				if err := md.unify(datum, subv); err != nil {
					return e("Type mismatch for '%s.%s': %s",
						rv.Type().String(), f.name, err)
				}
				md.context = md.context[0 : len(md.context)-1]
			} else if f.name != "" {
				// Bad user! No soup for you!
				return e("Field '%s.%s' is unexported, and therefore cannot "+
					"be loaded with reflection.", rv.Type().String(), f.name)
			}
		}
	}
	return nil
}

func (md *MetaData) unifyMap(mapping interface{}, rv reflect.Value) error {
	tmap, ok := mapping.(map[string]interface{})
	if !ok {
		return badtype("map", mapping)
	}
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(rv.Type()))
	}
	for k, v := range tmap {
		md.decoded[md.context.add(k).String()] = true
		md.context = append(md.context, k)

		rvkey := indirect(reflect.New(rv.Type().Key()))
		rvval := reflect.Indirect(reflect.New(rv.Type().Elem()))
		if err := md.unify(v, rvval); err != nil {
			return err
		}
		md.context = md.context[0 : len(md.context)-1]
		keyType := rvkey.Kind()
		if keyType >= reflect.Int && keyType <= reflect.Uint64 {
			i, err := strconv.ParseInt(k, 10, 64)
			if err != nil {
				return err
			}
			if err := md.unifyInt(i, rvkey); err != nil {
				return err
			}
		} else {
			switch keyType {
			case reflect.String:
				rvkey.SetString(k)
			default:
				return e("This key type of map is not supported. (key type: %v, map: %v)",
					keyType, mapping)
			}
		}
		if mv, ok := rvval.Interface().(map[string]interface{}); ok {
			index := reflect.ValueOf(k)
			originalValue := rv.MapIndex(index)
			if originalValue.IsValid() && originalValue.Kind() == reflect.Interface {
				err := map2struct.Scan(originalValue.Interface(), mv, `confl`, `json`)
				if err == nil {
					rv.SetMapIndex(rvkey, originalValue)
					continue
				}
			}
		}
		rv.SetMapIndex(rvkey, rvval)
	}
	return nil
}

func (md *MetaData) unifyArray(data interface{}, rv reflect.Value) error {
	datav := reflect.ValueOf(data)
	if datav.Kind() != reflect.Slice {
		return badtype("slice", data)
	}
	sliceLen := datav.Len()
	if sliceLen != rv.Len() {
		return e("expected array length %d; got array of length %d",
			rv.Len(), sliceLen)
	}
	return md.unifySliceArray(datav, rv)
}

func (md *MetaData) unifySlice(data interface{}, rv reflect.Value) error {
	datav := reflect.ValueOf(data)
	if datav.Kind() != reflect.Slice {
		return badtype("slice", data)
	}
	sliceLen := datav.Len()
	if rv.IsNil() {
		rv.Set(reflect.MakeSlice(rv.Type(), sliceLen, sliceLen))
	}
	return md.unifySliceArray(datav, rv)
}

func (md *MetaData) unifySliceArray(data, rv reflect.Value) error {
	sliceLen := data.Len()
	for i := 0; i < sliceLen; i++ {
		v := data.Index(i).Interface()
		sliceval := indirect(rv.Index(i))
		if err := md.unify(v, sliceval); err != nil {
			return err
		}
	}
	return nil
}

func (md *MetaData) unifyDatetime(data interface{}, rv reflect.Value) error {
	if _, ok := data.(time.Time); ok {
		rv.Set(reflect.ValueOf(data))
		return nil
	}
	return badtype("time.Time", data)
}

func (md *MetaData) unifyString(data interface{}, rv reflect.Value) error {
	switch v := data.(type) {
	case string:
		rv.SetString(v)
		return nil
	case int:
		rv.SetString(strconv.Itoa(v))
		return nil
	case int64:
		rv.SetString(strconv.FormatInt(v, 10))
		return nil
	case float32:
		rv.SetString(strconv.FormatFloat(float64(v), 'f', -1, 32))
		return nil
	case float64:
		rv.SetString(strconv.FormatFloat(v, 'f', -1, 64))
		return nil
	case bool:
		rv.SetString(strconv.FormatBool(v))
		return nil
	}
	return badtype("string", data)
}

func (md *MetaData) unifyFloat64(data interface{}, rv reflect.Value) error {
	num, ok := data.(float64)
	if !ok {
		//自动将整数转换为浮点数
		var numInt int64
		numInt, ok = data.(int64)
		if ok {
			num = float64(numInt)
		}
	}
	if ok {
		switch rv.Kind() {
		case reflect.Float32:
			fallthrough
		case reflect.Float64:
			rv.SetFloat(num)
		default:
			panic("bug")
		}
		return nil
	}
	return badtype("float", data)
}

func (md *MetaData) unifyInt(data interface{}, rv reflect.Value) error {
	if num, ok := data.(int64); ok {
		if rv.Kind() >= reflect.Int && rv.Kind() <= reflect.Int64 {
			switch rv.Kind() {
			case reflect.Int, reflect.Int64:
				// No bounds checking necessary.
			case reflect.Int8:
				if num < math.MinInt8 || num > math.MaxInt8 {
					return e("Value '%d' is out of range for int8.", num)
				}
			case reflect.Int16:
				if num < math.MinInt16 || num > math.MaxInt16 {
					return e("Value '%d' is out of range for int16.", num)
				}
			case reflect.Int32:
				if num < math.MinInt32 || num > math.MaxInt32 {
					return e("Value '%d' is out of range for int32.", num)
				}
			}
			rv.SetInt(num)
		} else if rv.Kind() >= reflect.Uint && rv.Kind() <= reflect.Uint64 {
			unum := uint64(num)
			switch rv.Kind() {
			case reflect.Uint, reflect.Uint64:
				// No bounds checking necessary.
			case reflect.Uint8:
				if num < 0 || unum > math.MaxUint8 {
					return e("Value '%d' is out of range for uint8.", num)
				}
			case reflect.Uint16:
				if num < 0 || unum > math.MaxUint16 {
					return e("Value '%d' is out of range for uint16.", num)
				}
			case reflect.Uint32:
				if num < 0 || unum > math.MaxUint32 {
					return e("Value '%d' is out of range for uint32.", num)
				}
			}
			rv.SetUint(unum)
		} else {
			panic("unreachable")
		}
		return nil
	}
	return badtype("integer", data)
}

func (md *MetaData) unifyBool(data interface{}, rv reflect.Value) error {
	if b, ok := data.(bool); ok {
		rv.SetBool(b)
		return nil
	}
	return badtype("boolean", data)
}

func (md *MetaData) unifyAnything(data interface{}, rv reflect.Value) error {
	rv.Set(reflect.ValueOf(data))
	return nil
}

func (md *MetaData) unifyText(data interface{}, v TextUnmarshaler) error {
	var s string
	switch sdata := data.(type) {
	case TextMarshaler:
		text, err := sdata.MarshalText()
		if err != nil {
			return err
		}
		s = string(text)
	case fmt.Stringer:
		s = sdata.String()
	case string:
		s = sdata
	case bool:
		s = strconv.FormatBool(sdata)
	case int64:
		s = strconv.FormatInt(sdata, 10)
	case float64:
		s = strconv.FormatFloat(sdata, 'f', -1, 64)
	default:
		return badtype("primitive (string-like)", data)
	}
	if err := v.UnmarshalText([]byte(s)); err != nil {
		return err
	}
	return nil
}

// rvalue returns a reflect.Value of `v`. All pointers are resolved.
func rvalue(v interface{}) reflect.Value {
	return indirect(reflect.ValueOf(v))
}

// indirect returns the value pointed to by a pointer.
// Pointers are followed until the value is not a pointer.
// New values are allocated for each nil pointer.
//
// An exception to this rule is if the value satisfies an interface of
// interest to us (like encoding.TextUnmarshaler).
func indirect(v reflect.Value) reflect.Value {
	if v.Kind() != reflect.Ptr {
		if v.CanAddr() {
			pv := v.Addr()
			if _, ok := pv.Interface().(TextUnmarshaler); ok {
				return pv
			}
		}
		return v
	}
	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return indirect(reflect.Indirect(v))
}

func isUnifiable(rv reflect.Value) bool {
	if rv.CanSet() {
		return true
	}
	if _, ok := rv.Interface().(TextUnmarshaler); ok {
		return true
	}
	return false
}

func badtype(expected string, data interface{}) error {
	return e("Expected %s but found '%T'.", expected, data)
}

func mismatch(user reflect.Value, expected string, data interface{}) error {
	return e("Type mismatch for %s. Expected %s but found '%T'.",
		user.Type().String(), expected, data)
}
