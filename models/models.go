// Package models contains typed Redfish request and response contracts.
// Unknown enum values and vendor properties are retained for forward compatibility.
package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// StringOrNumber accepts vendor resource identifiers represented as strings or numbers.
type StringOrNumber string

func (v *StringOrNumber) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*v = StringOrNumber(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*v = StringOrNumber(n.String())
	return nil
}
func (v StringOrNumber) String() string { return string(v) }

// Oem is intentionally dynamic: each vendor defines a different namespace.
type Oem map[string]json.RawMessage

// Collection retains member links and server-provided pagination metadata.
type Collection[T any] struct {
	ODataID         *string `json:"@odata.id,omitempty"`
	Members         []T     `json:"Members"`
	MembersCount    *int64  `json:"Members@odata.count,omitempty"`
	MembersNextLink *string `json:"Members@odata.nextLink,omitempty"`
	NextLink        *string `json:"@odata.nextLink,omitempty"`
}

var _fieldCache sync.Map

func jsonFields(t reflect.Type) map[string]bool {
	if cached, ok := _fieldCache.Load(t); ok {
		return cached.(map[string]bool)
	}
	fields := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		key := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if key != "" && key != "-" {
			fields[key] = true
		}
	}
	_fieldCache.Store(t, fields)
	return fields
}
func decodeExtra(data []byte, dest any) (map[string]json.RawMessage, error) {
	return decodeExtraWith(data, dest, func() error { return json.Unmarshal(data, dest) })
}

func decodeExtraWith(data []byte, dest any, decode func() error) (map[string]json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("expected resource object, got null")
	}
	if err := decode(); err != nil {
		return nil, err
	}
	// Only the extension portion is dynamic; known fields decode directly above.
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(data, &extra); err != nil {
		return nil, err
	}
	// Alternate wire names are declared by the Python model's AliasChoices.
	value := reflect.ValueOf(dest).Elem()
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		alternates := field.Tag.Get("redfish")
		if alternates == "" {
			continue
		}
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		if _, present := extra[key]; !present {
			for _, alias := range strings.Split(alternates, ",") {
				if raw, found := extra[alias]; found {
					extra[key] = raw
					if err := json.Unmarshal(raw, value.Field(i).Addr().Interface()); err != nil {
						return nil, err
					}
					break
				}
			}
		}
		for _, alias := range strings.Split(alternates, ",") {
			delete(extra, alias)
		}
	}
	nulls := map[string]bool{}
	for key := range jsonFields(typ) {
		if raw, ok := extra[key]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			nulls[key] = true
		}
	}
	if len(nulls) == 0 {
		nulls = nil
	}
	value.FieldByName("NullFields").Set(reflect.ValueOf(nulls))
	for key := range jsonFields(reflect.TypeOf(dest).Elem()) {
		delete(extra, key)
	}
	if len(extra) == 0 {
		extra = nil
	}
	return extra, nil
}
func encodeExtra(value any, extra map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	known := jsonFields(reflect.TypeOf(value))
	for k, v := range extra {
		if !known[k] {
			fields[k] = v
		}
	}
	reflected := reflect.ValueOf(value)
	typ := reflected.Type()
	var nulls map[string]bool
	if field := reflected.FieldByName("NullFields"); field.IsValid() {
		nulls = field.Interface().(map[string]bool)
	}
	for i := 0; i < reflected.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			continue
		}
		field := reflected.Field(i)
		if nulls[key] && field.IsZero() {
			fields[key] = json.RawMessage("null")
			continue
		}
		if (field.Kind() == reflect.Slice || field.Kind() == reflect.Map) && !field.IsNil() && field.Len() == 0 {
			encoded, err := json.Marshal(field.Interface())
			if err != nil {
				return nil, err
			}
			fields[key] = encoded
		}
	}
	return json.Marshal(fields)
}

// Sensitive DTOs keep source-model repr=False fields out of fmt logging.
func formatSensitiveModel(state fmt.State, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		fmt.Fprint(state, "[unprintable model]")
		return
	}
	var document map[string]any
	if err = json.Unmarshal(data, &document); err != nil {
		fmt.Fprint(state, "[unprintable model]")
		return
	}
	var secrets []string
	var gather func(any)
	sensitive := func(key string) bool {
		key = strings.ToLower(key)
		for _, part := range []string{"password", "secret", "token", "authorization", "authenticationkey", "encryptionkey", "httpheaders"} {
			if strings.Contains(key, part) {
				return true
			}
		}
		return false
	}
	gather = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			for key, child := range typed {
				if sensitive(key) {
					if text, ok := child.(string); ok && text != "" {
						secrets = append(secrets, text)
					}
				}
				gather(child)
			}
		case []any:
			for _, child := range typed {
				gather(child)
			}
		}
	}
	gather(document)
	var clean func(any) any
	clean = func(node any) any {
		switch typed := node.(type) {
		case string:
			for _, secret := range secrets {
				typed = strings.ReplaceAll(typed, secret, "[REDACTED]")
			}
			return typed
		case map[string]any:
			for key, child := range typed {
				if sensitive(key) {
					typed[key] = "[REDACTED]"
				} else {
					typed[key] = clean(child)
				}
			}
		case []any:
			for i, child := range typed {
				typed[i] = clean(child)
			}
		}
		return node
	}
	safe, err := json.Marshal(clean(document))
	if err != nil {
		fmt.Fprint(state, "[unprintable model]")
		return
	}
	fmt.Fprint(state, string(safe))
}

// OneOrMany accepts the documented array and observed singleton-object representation.
type OneOrMany[T any] []T

func (values *OneOrMany[T]) UnmarshalJSON(data []byte) error {
	if len(bytes.TrimSpace(data)) > 0 && bytes.TrimSpace(data)[0] == '{' {
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		*values = []T{item}
		return nil
	}
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	*values = items
	return nil
}

// Percentage accepts task percentages encoded as numbers or strings such as "100%".
type Percentage int64

func (value *Percentage) UnmarshalJSON(data []byte) error {
	var number int64
	if err := json.Unmarshal(data, &number); err == nil {
		*value = Percentage(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "%"))
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	*value = Percentage(number)
	return nil
}
