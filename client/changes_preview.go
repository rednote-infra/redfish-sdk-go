package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
)

// changeDiff compares original values, then renders values from redacted documents.
// Comparing after redaction would hide changes between two different passwords.
func changeDiff(op ChangeOperation, includeSensitive bool, secrets ...string) ([]Object, error) {
	decode := func(data []byte) (any, error) {
		if len(data) == 0 {
			return nil, nil
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var value any
		err := decoder.Decode(&value)
		return value, err
	}
	before, err := decode(op.Before)
	if err != nil {
		return nil, err
	}
	after, err := decode(op.Body)
	if err != nil {
		return nil, err
	}
	safeBefore, safeAfter := before, after
	if !includeSensitive {
		body, err := redaction.Redact(before, secrets...)
		if err != nil {
			return nil, err
		}
		safeBefore, err = decode(body)
		if err != nil {
			return nil, err
		}
		body, err = redaction.Redact(after, secrets...)
		if err != nil {
			return nil, err
		}
		safeAfter, err = decode(body)
		if err != nil {
			return nil, err
		}
	}
	rows := make([]Object, 0)
	var walk func(string, any, any, any, any, bool, bool)
	walk = func(path string, old, next, safeOld, safeNext any, oldPresent, nextPresent bool) {
		if oldPresent == nextPresent && jsonEqual(old, next) {
			return
		}
		oldMap, oldObject := old.(map[string]any)
		nextMap, nextObject := next.(map[string]any)
		if (oldObject || !oldPresent) && (nextObject || !nextPresent) && (oldObject || nextObject) {
			keys := make(map[string]bool)
			for key := range oldMap {
				keys[key] = true
			}
			for key := range nextMap {
				keys[key] = true
			}
			ordered := make([]string, 0, len(keys))
			for key := range keys {
				ordered = append(ordered, key)
			}
			slices.Sort(ordered)
			safeOldMap, _ := safeOld.(map[string]any)
			safeNextMap, _ := safeNext.(map[string]any)
			for _, key := range ordered {
				oldValue, hasOld := oldMap[key]
				nextValue, hasNext := nextMap[key]
				if op.Method == http.MethodPatch && hasOld && !hasNext {
					continue
				}
				escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
				walk(path+"/"+escaped, oldValue, nextValue, safeOldMap[key], safeNextMap[key], hasOld, hasNext)
			}
			return
		}
		row := Object{"path": path, "before_present": oldPresent, "after_present": nextPresent}
		if oldPresent {
			row["before"] = safeOld
		}
		if nextPresent {
			row["after"] = safeNext
		}
		rows = append(rows, row)
	}
	walk("", before, after, safeBefore, safeAfter, len(op.Before) > 0, len(op.Body) > 0 && op.Method != http.MethodDelete)
	return rows, nil
}
