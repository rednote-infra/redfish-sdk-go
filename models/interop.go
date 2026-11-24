package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
)

// ExactLowerBound returns a copy of the advertised bound, including values outside int64.
func (m AttributeDefinition) ExactLowerBound() *big.Int {
	return exactInteger(m.LowerBound, m.exactLowerBound)
}

// ExactUpperBound returns a copy of the advertised bound, including values outside int64.
func (m AttributeDefinition) ExactUpperBound() *big.Int {
	return exactInteger(m.UpperBound, m.exactUpperBound)
}

// ExactScalarIncrement returns a copy of the advertised increment.
func (m AttributeDefinition) ExactScalarIncrement() *big.Int {
	return exactInteger(m.ScalarIncrement, m.exactScalarIncrement)
}
func exactInteger(legacy *int64, exact *big.Int) *big.Int {
	if legacy != nil {
		return big.NewInt(*legacy)
	}
	if exact != nil {
		return new(big.Int).Set(exact)
	}
	return nil
}
func decodeAttributeDefinition(data []byte, m *AttributeDefinition) (map[string]json.RawMessage, error) {
	type plain AttributeDefinition
	wire := struct {
		*plain
		Lower     json.RawMessage `json:"LowerBound"`
		Upper     json.RawMessage `json:"UpperBound"`
		Increment json.RawMessage `json:"ScalarIncrement"`
	}{plain: (*plain)(m)}
	return decodeExtraWith(data, (*plain)(m), func(data []byte) error {
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		fields := []struct {
			raw    json.RawMessage
			legacy **int64
			exact  **big.Int
		}{{wire.Lower, &m.LowerBound, &m.exactLowerBound}, {wire.Upper, &m.UpperBound, &m.exactUpperBound}, {wire.Increment, &m.ScalarIncrement, &m.exactScalarIncrement}}
		for _, f := range fields {
			*f.legacy = nil
			*f.exact = nil
			if len(f.raw) == 0 || bytes.Equal(f.raw, []byte("null")) {
				continue
			}
			n, ok := new(big.Int).SetString(string(f.raw), 10)
			if !ok {
				return fmt.Errorf("invalid registry integer: %s", f.raw)
			}
			*f.exact = n
			if n.IsInt64() {
				*f.legacy = Ptr(n.Int64())
			}
		}
		return nil
	})
}
func encodeAttributeDefinition(m AttributeDefinition) ([]byte, error) {
	type plain AttributeDefinition
	data, err := encodeExtra(plain(m), m.Extra)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, n := range map[string]*big.Int{"LowerBound": m.ExactLowerBound(), "UpperBound": m.ExactUpperBound(), "ScalarIncrement": m.ExactScalarIncrement()} {
		if n != nil {
			fields[key] = json.RawMessage(n.String())
		}
	}
	return json.Marshal(fields)
}

// DriveOperation describes a running drive operation and its optional task.
type DriveOperation struct {
	emptyAssociatedTask bool
	Extra               map[string]json.RawMessage `json:"-"`
	NullFields          map[string]bool            `json:"-"`
	OperationName       *string                    `json:"OperationName,omitempty"`
	PercentageComplete  *float64                   `json:"PercentageComplete,omitempty"`
	AssociatedTask      *Link                      `json:"AssociatedTask,omitempty"`
}

func (m *DriveOperation) UnmarshalJSON(data []byte) error {
	type plain DriveOperation
	wire := struct {
		*plain
		Task json.RawMessage `json:"AssociatedTask"`
	}{plain: (*plain)(m)}

	extra, err := decodeExtraWith(data, (*plain)(m), func(data []byte) error {
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		m.AssociatedTask = nil
		m.emptyAssociatedTask = bytes.Equal(bytes.TrimSpace(wire.Task), []byte("[]"))
		if len(wire.Task) == 0 || m.emptyAssociatedTask {
			return nil
		}
		return json.Unmarshal(wire.Task, &m.AssociatedTask)
	})
	m.Extra = extra
	return err
}
func (m DriveOperation) MarshalJSON() ([]byte, error) {
	type plain DriveOperation
	data, err := encodeExtra(plain(m), m.Extra)
	if err != nil || !m.emptyAssociatedTask || m.AssociatedTask != nil {
		return data, err
	}
	return withEmptyArray(data, "AssociatedTask")
}
func withEmptyArray(data []byte, key string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	fields[key] = json.RawMessage("[]")
	return json.Marshal(fields)
}

func decodeStorageController(data []byte, m *StorageController) (map[string]json.RawMessage, error) {
	type plain StorageController
	wire := struct {
		*plain
		PCIe json.RawMessage `json:"PCIeInterface"`
	}{plain: (*plain)(m)}
	return decodeExtraWith(data, (*plain)(m), func(data []byte) error {
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		m.PCIeInterface = nil
		m.emptyPCIeInterface = bytes.Equal(bytes.TrimSpace(wire.PCIe), []byte("[]"))
		if len(wire.PCIe) == 0 || bytes.Equal(bytes.TrimSpace(wire.PCIe), []byte("[]")) {
			return nil
		}
		return json.Unmarshal(wire.PCIe, &m.PCIeInterface)
	})
}
