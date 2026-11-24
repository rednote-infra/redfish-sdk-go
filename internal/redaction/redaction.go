package redaction

import (
	"bytes"
	"encoding/json"
	"strings"
)

func String(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

// Redact produces log-safe JSON without mutating the caller's raw data.
func Redact(value any, secrets ...string) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err = decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	var visit func(any) any
	visit = func(node any) any {
		switch v := node.(type) {
		case string:
			return String(v, secrets...)
		case map[string]any:
			for key, child := range v {
				lower := strings.ToLower(key)
				sensitive := false
				for _, word := range []string{"password", "secret", "token", "authorization", "authenticationkey", "encryptionkey", "cookie", "privatekey", "keytab"} {
					if strings.Contains(lower, word) {
						sensitive = true
						break
					}
				}
				if sensitive {
					v[key] = "[REDACTED]"
				} else {
					v[key] = visit(child)
				}
			}
		case []any:
			for i, child := range v {
				v[i] = visit(child)
			}
		}
		return node
	}
	return json.Marshal(visit(decoded))
}

func Secrets(data []byte) []string {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	var secrets []string
	var visit func(any)
	visit = func(node any) {
		switch fields := node.(type) {
		case map[string]any:
			for key, child := range fields {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "privatekey") || strings.Contains(lower, "keytab") {
					if text, ok := child.(string); ok && text != "" {
						secrets = append(secrets, text)
					}
				}
				visit(child)
			}
		case []any:
			for _, child := range fields {
				visit(child)
			}
		}
	}
	visit(value)
	return secrets
}
