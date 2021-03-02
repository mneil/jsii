package kernel

import (
	"reflect"

	"github.com/aws/jsii-runtime-go/api"
)

// CastAndSetToPtr accepts a pointer to any type and attempts to cast the value
// argument to be the same type. Then it sets the value of the pointer element
// to be the newly cast data. This is used to cast payloads from JSII to
// expected return types for Get and Invoke functions.
func (c *client) CastAndSetToPtr(ptr interface{}, data interface{}) {
	ptrVal := reflect.ValueOf(ptr).Elem()
	dataVal := reflect.ValueOf(data)

	c.castAndSetToPtr(ptrVal, dataVal)
}

// castAndSetToPtr is the same as CastAndSetToPtr except it operates on the
// reflect.Value representation of the pointer and data.
func (c *client) castAndSetToPtr(ptr reflect.Value, data reflect.Value) {
	if !data.IsValid() {
		// data will not be valid if was made from a nil value, as there would
		// not have been enough type information available to build a valid
		// reflect.Value. In such cases, we must craft the correctly-typed zero
		// value ourselves.
		data = reflect.Zero(ptr.Type())
	} else if data.Kind() == reflect.Interface && !data.IsNil() {
		// If data is a non-nil interface, unwrap it to get it's dynamic value
		// type sorted out, so that further calls in this method don't have to
		// worry about this edge-case when reasoning on kinds.
		data = reflect.ValueOf(data.Interface())
	}

	if ref, isRef := castValToRef(data); isRef {
		// If return data is a jsii struct passed by reference, de-reference it all.
		if fields, isStruct := c.Types().StructFields(ptr.Type()); isStruct {
			for _, field := range fields {
				got, err := c.Get(GetProps{
					Property: field.Tag.Get("json"),
					ObjRef:   ref,
				})
				if err != nil {
					panic(err)
				}
				fieldVal := ptr.FieldByIndex(field.Index)
				c.castAndSetToPtr(fieldVal, reflect.ValueOf(got.Value))
			}
			return
		}

		// If return data is jsii object references, add to objects table.
		if err := c.Types().InitJsiiProxy(ptr); err == nil {
			if err = c.RegisterInstance(ptr, ref.InstanceID); err != nil {
				panic(err)
			}
		} else {
			panic(err)
		}
		return
	}

	if enumref, isEnum := castValToEnumRef(data); isEnum {
		member, err := c.Types().EnumMemberForEnumRef(enumref)
		if err != nil {
			panic(err)
		}

		ptr.Set(reflect.ValueOf(member))
		return
	}

	// maps
	if m, isMap := c.castValToMap(data, ptr.Type()); isMap {
		ptr.Set(m)
		return
	}

	// arrays
	if ptr.Kind() == reflect.Slice && data.Kind() == reflect.Slice {
		len := data.Len()
		ptr.Set(reflect.MakeSlice(ptr.Type(), len, len))

		// If return type is a slice, recursively cast elements
		for i := 0; i < len; i++ {
			c.castAndSetToPtr(ptr.Index(i), data.Index(i))
		}

		return
	}

	ptr.Set(data)
}

func castValToRef(data reflect.Value) (api.ObjectRef, bool) {
	ref := api.ObjectRef{}
	ok := false

	if data.Kind() == reflect.Map {
		for _, k := range data.MapKeys() {
			// Finding values type requires extracting from reflect.Value
			// otherwise .Kind() returns `interface{}`
			v := reflect.ValueOf(data.MapIndex(k).Interface())

			if k.Kind() == reflect.String && k.String() == "$jsii.byref" && v.Kind() == reflect.String {
				ref.InstanceID = v.String()
				ok = true
			}

		}
	}

	return ref, ok
}

func castValToEnumRef(data reflect.Value) (enum api.EnumRef, ok bool) {
	ok = false

	if data.Kind() == reflect.Map {
		for _, k := range data.MapKeys() {
			// Finding values type requires extracting from reflect.Value
			// otherwise .Kind() returns `interface{}`
			v := reflect.ValueOf(data.MapIndex(k).Interface())

			if k.Kind() == reflect.String && k.String() == "$jsii.enum" && v.Kind() == reflect.String {
				enum.MemberFQN = v.String()
				ok = true
				return
			}
		}
	}

	return
}

// castValToMap attempts converting the provided jsii wire value to a
// go map. This recognizes the "$jsii.map" object and does the necessary
// recursive value conversion.
func (c *client) castValToMap(data reflect.Value, mapType reflect.Type) (m reflect.Value, ok bool) {
	ok = false

	if data.Kind() != reflect.Map || data.Type().Key().Kind() != reflect.String {
		return
	}

	if mapType.Kind() == reflect.Map && mapType.Key().Kind() != reflect.String {
		return
	}
	anyType := reflect.TypeOf((*interface{})(nil)).Elem()
	if mapType == anyType {
		mapType = reflect.TypeOf((map[string]interface{})(nil))
	}

	dataIter := data.MapRange()
	for dataIter.Next() {
		key := dataIter.Key().String()
		if key != "$jsii.map" {
			continue
		}

		// Finding value type requries extracting from reflect.Value
		// otherwise .Kind() returns `interface{}`
		val := reflect.ValueOf(dataIter.Value().Interface())
		if val.Kind() != reflect.Map {
			return
		}

		ok = true

		m = reflect.MakeMap(mapType)

		iter := val.MapRange()
		for iter.Next() {
			val := iter.Value()
			// Note: reflect.New(t) returns a pointer to a newly allocated t
			convertedVal := reflect.New(mapType.Elem()).Elem()
			c.castAndSetToPtr(convertedVal, val)

			m.SetMapIndex(iter.Key(), convertedVal)
		}
		return
	}
	return
}