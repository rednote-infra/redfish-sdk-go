package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

const (
	_indicatorLit = "Lit"
	_indicatorOff = "Off"
)

// Optional OEM switches accept the spellings observed in Python SDK contracts.
type optionalSwitch struct {
	value   *bool
	present bool
}

func (s *optionalSwitch) UnmarshalJSON(data []byte) error {
	s.present = true
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	var value bool
	if json.Unmarshal(data, &value) == nil {
		s.value = &value
		return nil
	}
	var number float64
	if json.Unmarshal(data, &number) == nil {
		s.value = Ptr(number != 0)
		return nil
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		switch strings.ToLower(strings.TrimSpace(text)) {
		case "true", "yes", "on", "1", "enabled", "enable":
			s.value = Ptr(true)
		case "false", "no", "off", "0", "disabled", "disable", "":
			s.value = Ptr(false)
		}
	}
	return nil
}

type optionalInteger struct {
	value   *int64
	present bool
}

func (i *optionalInteger) UnmarshalJSON(data []byte) error {
	i.present = true
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return err
		}
		i.value = &value
		return nil
	}
	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	i.value = &value
	return nil
}

func decodeSyslogServer(data []byte, model *SyslogServer) (map[string]json.RawMessage, error) {
	type plain SyslogServer
	wire := struct {
		*plain
		MemberID optionalInteger `json:"MemberId"`
		Enabled  optionalSwitch  `json:"Enabled"`
		Port     optionalInteger `json:"Port"`
	}{plain: (*plain)(model)}
	extra, err := decodeExtraWith(data, (*plain)(model), func(data []byte) error {
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		model.MemberID, model.Enabled, model.Port = wire.MemberID.value, wire.Enabled.value, wire.Port.value
		return nil
	})
	if err != nil {
		return nil, err
	}
	markNull := func(field string, present, missing bool) {
		if present && missing {
			if model.NullFields == nil {
				model.NullFields = map[string]bool{}
			}
			model.NullFields[field] = true
		}
	}
	markNull("MemberId", wire.MemberID.present, model.MemberID == nil)
	markNull("Enabled", wire.Enabled.present, model.Enabled == nil)
	markNull("Port", wire.Port.present, model.Port == nil)
	return extra, nil
}

func decodeSyslogService(data []byte, model *SyslogService) (map[string]json.RawMessage, error) {
	model.supermicroSyslog = false
	model.scalarSyslog = false
	type plain SyslogService
	wire := struct {
		*plain
		Enabled           optionalSwitch  `json:"ServiceEnabled"`
		SupermicroEnabled optionalSwitch  `json:"EnableSyslog"`
		SupermicroServers json.RawMessage `json:"SyslogServer"`
		SupermicroPort    *int64          `json:"SyslogPortNumber"`
	}{plain: (*plain)(model)}
	return decodeExtraWith(data, (*plain)(model), func(data []byte) error {
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		// Unlike optional server entries, an invalid service switch is an invalid contract.
		var presence struct {
			Enabled           json.RawMessage `json:"ServiceEnabled"`
			SupermicroEnabled json.RawMessage `json:"EnableSyslog"`
		}
		if err := json.Unmarshal(data, &presence); err != nil {
			return err
		}
		if len(presence.Enabled) > 0 && !bytes.Equal(presence.Enabled, []byte("null")) && wire.Enabled.value == nil {
			return errors.New("ServiceEnabled must be a boolean switch")
		}
		model.ServiceEnabled = wire.Enabled.value
		model.supermicroSyslog = wire.SupermicroEnabled.present || len(wire.SupermicroServers) > 0
		if !wire.Enabled.present && wire.SupermicroEnabled.present && !bytes.Equal(bytes.TrimSpace(presence.SupermicroEnabled), []byte("null")) && wire.SupermicroEnabled.value == nil {
			return errors.New("EnableSyslog must be a boolean switch")
		}
		if !wire.Enabled.present && wire.SupermicroEnabled.present {
			model.ServiceEnabled = wire.SupermicroEnabled.value
		}
		if len(wire.SupermicroServers) > 0 && !bytes.Equal(bytes.TrimSpace(wire.SupermicroServers), []byte("null")) && model.SyslogServers == nil {
			var server string
			if json.Unmarshal(wire.SupermicroServers, &server) == nil {
				model.scalarSyslog = true
				model.SyslogServers = []SyslogServer{{Address: Ptr(server), Port: wire.SupermicroPort}}
			} else if err := json.Unmarshal(wire.SupermicroServers, &model.SyslogServers); err != nil {
				return err
			}
		}
		return nil
	})
}

func decodeDrive(data []byte, model *Drive) (map[string]json.RawMessage, error) {
	model.inferredIndicatorLED = nil
	model.inferredIndicatorValue = ""
	type plain Drive
	wire := struct {
		*plain
		Location OneOrMany[Location] `json:"Location"`
	}{plain: (*plain)(model)}
	return decodeExtraWith(data, (*plain)(model), func(data []byte) error {
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		model.Location = wire.Location
		var presence struct {
			IndicatorLED json.RawMessage `json:"IndicatorLED"`
		}
		if err := json.Unmarshal(data, &presence); err != nil {
			return err
		}
		if len(presence.IndicatorLED) == 0 && model.LocationIndicatorActive != nil {
			indicator := _indicatorOff
			if *model.LocationIndicatorActive {
				indicator = _indicatorLit
			}
			model.IndicatorLed = &indicator
			model.inferredIndicatorLED = model.IndicatorLed
			model.inferredIndicatorValue = indicator
		}
		return nil
	})
}

func decodeProcessorID(data []byte, model *ProcessorId) (map[string]json.RawMessage, error) {
	type plain ProcessorId
	wire := struct {
		*plain
		EffectiveFamily *StringOrNumber `json:"EffectiveFamily"`
		EffectiveModel  *StringOrNumber `json:"EffectiveModel"`
		Step            *StringOrNumber `json:"Step"`
	}{plain: (*plain)(model)}
	return decodeExtraWith(data, (*plain)(model), func(data []byte) error {
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		convert := func(value *StringOrNumber) *string {
			if value == nil {
				return nil
			}
			return Ptr(string(*value))
		}
		model.EffectiveFamily, model.EffectiveModel, model.Step = convert(wire.EffectiveFamily), convert(wire.EffectiveModel), convert(wire.Step)
		return nil
	})
}

func decodeProcessor(data []byte, model *Processor) (map[string]json.RawMessage, error) {
	type plain Processor
	extra, err := decodeExtra(data, (*plain)(model))
	if err != nil {
		return nil, err
	}
	var presence struct {
		SerialNumber json.RawMessage `json:"SerialNumber"`
	}
	if err := json.Unmarshal(data, &presence); err != nil {
		return nil, err
	}
	if len(presence.SerialNumber) == 0 && model.OEM != nil {
		var bmc struct {
			SerialNumber *string `json:"SerialNumber"`
		}
		if raw := (*model.OEM)["BMC"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &bmc); err != nil {
				return nil, err
			}
			model.SerialNumber = bmc.SerialNumber
		}
	}
	return extra, nil
}

// Preserve the service's OEM wire names when a fetched model is used for a mutation.
func encodeSyslogService(model SyslogService) ([]byte, error) {
	type plain SyslogService
	data, err := encodeExtra(plain(model), model.Extra)
	if err != nil || !model.supermicroSyslog {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if enabled, ok := fields["ServiceEnabled"]; ok {
		fields["EnableSyslog"] = enabled
		delete(fields, "ServiceEnabled")
	}
	delete(fields, "SyslogServers")
	if model.SyslogServers == nil {
		if model.NullFields["SyslogServers"] {
			fields["SyslogServer"] = json.RawMessage("null")
		}
		return json.Marshal(fields)
	}
	if model.scalarSyslog {
		if len(model.SyslogServers) != 1 {
			return nil, errors.New("scalar SyslogServer requires exactly one server")
		}
		fields["SyslogServer"], err = json.Marshal(model.SyslogServers[0].Address)
		if err != nil {
			return nil, err
		}
		fields["SyslogPortNumber"], err = json.Marshal(model.SyslogServers[0].Port)
		if err != nil {
			return nil, err
		}
	} else {
		servers := make([]json.RawMessage, 0, len(model.SyslogServers))
		for _, server := range model.SyslogServers {
			encoded, err := json.Marshal(server)
			if err != nil {
				return nil, err
			}
			var entry map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &entry); err != nil {
				return nil, err
			}
			if address, ok := entry["Address"]; ok {
				entry["ServerIP"] = address
				delete(entry, "Address")
			}
			encoded, err = json.Marshal(entry)
			if err != nil {
				return nil, err
			}
			servers = append(servers, encoded)
		}
		fields["SyslogServer"], err = json.Marshal(servers)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}
