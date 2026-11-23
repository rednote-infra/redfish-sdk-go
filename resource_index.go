package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	DefaultDiscoveryMaxDepth     = 6
	DefaultDiscoveryMaxResources = 1000
)

type DiscoveryOptions struct {
	Start                  string
	MaxDepth, MaxResources int
	IncludeExpensive       bool
}
type ResourceDescriptor struct {
	URI        string
	ODataType  string
	Properties json.RawMessage
}

func (r ResourceDescriptor) String() string { return "ResourceDescriptor(" + r.ODataType + ")" }

type ResourceDiscoveryFailure struct {
	URI, Reason string
	Err         error
}
type ResourceIndex struct {
	Resources []ResourceDescriptor
	Failures  []ResourceDiscoveryFailure
	Truncated bool
}

func (index *ResourceIndex) IsPartial() bool {
	return index != nil && (index.Truncated || len(index.Failures) > 0)
}
func (index *ResourceIndex) Types() []string {
	seen := map[string]bool{}
	var types []string
	for _, resource := range index.Resources {
		if !seen[resource.ODataType] {
			types = append(types, resource.ODataType)
			seen[resource.ODataType] = true
		}
	}
	slices.Sort(types)
	return types
}
func (index *ResourceIndex) Find(odataType string, where Object) *ResourceIndex {
	out := &ResourceIndex{Failures: slices.Clone(index.Failures), Truncated: index.Truncated}
	for _, resource := range index.Resources {
		if odataType != "" && resource.ODataType != odataType && !strings.HasSuffix(resource.ODataType, "."+odataType) {
			continue
		}
		var properties Object
		if json.Unmarshal(resource.Properties, &properties) != nil {
			continue
		}
		matches := true
		for key, value := range where {
			actual, ok := lookupProperty(properties, key)
			if !ok || !jsonEqual(actual, value) {
				matches = false
				break
			}
		}
		if matches {
			copy := resource
			copy.Properties = append(json.RawMessage(nil), resource.Properties...)
			out.Resources = append(out.Resources, copy)
		}
	}
	return out
}
func jsonEqual(a, b any) bool {
	aa, ea := json.Marshal(a)
	bb, eb := json.Marshal(b)
	return ea == nil && eb == nil && string(aa) == string(bb)
}
func lookupProperty(value any, path string) (any, bool) {
	switch object := value.(type) {
	case Object:
		if v, ok := object[path]; ok {
			return v, true
		}
	case map[string]any:
		if v, ok := object[path]; ok {
			return v, true
		}
	}
	parts := strings.Split(path, ".")
	if strings.HasPrefix(path, "/") {
		parts = strings.Split(strings.TrimPrefix(path, "/"), "/")
	}
	for _, part := range parts {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch object := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = object[part]
			if !ok {
				return nil, false
			}
		case Object:
			var ok bool
			value, ok = object[part]
			if !ok {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return value, true
}
func (c *Client) DiscoverResources(ctx context.Context, options DiscoveryOptions) (*ResourceIndex, error) {
	if options.MaxDepth < 0 || options.MaxResources < 0 {
		return nil, errors.New("discovery bounds must not be negative")
	}
	if options.MaxDepth == 0 {
		options.MaxDepth = DefaultDiscoveryMaxDepth
	}
	if options.MaxResources == 0 {
		options.MaxResources = DefaultDiscoveryMaxResources
	}
	if options.Start == "" {
		options.Start = c.ServiceRoot()
	}
	start, err := c.resolve(options.Start, c.base.String()+"/")
	if err != nil {
		return nil, err
	}
	type node struct {
		uri   string
		depth int
	}
	queue := []node{{start.String(), 0}}
	seen := map[string]bool{}
	index := &ResourceIndex{Resources: []ResourceDescriptor{}}
	expensive := map[string]bool{"Entries": true, "MetricReports": true, "Tasks": true, "JsonSchemas": true, "Registries": true, "Sessions": true}
	for len(queue) > 0 {
		if err = ctx.Err(); err != nil {
			return index, err
		}
		current := queue[0]
		queue = queue[1:]
		if seen[current.uri] {
			continue
		}
		seen[current.uri] = true
		if len(seen) > options.MaxResources {
			index.Truncated = true
			index.Failures = append(index.Failures, ResourceDiscoveryFailure{current.uri, "max_resources", nil})
			break
		}
		r, err := c.Request(ctx, "GET", current.uri, nil)
		if err != nil {
			index.Failures = append(index.Failures, ResourceDiscoveryFailure{current.uri, "request", err})
			continue
		}
		var document map[string]json.RawMessage
		if err = r.Decode(&document); err != nil {
			index.Failures = append(index.Failures, ResourceDiscoveryFailure{current.uri, "decode", err})
			continue
		}
		var identity struct {
			URI  string `json:"@odata.id"`
			Type string `json:"@odata.type"`
		}
		if err = r.Decode(&identity); err != nil {
			index.Failures = append(index.Failures, ResourceDiscoveryFailure{current.uri, "identity", err})
			continue
		}
		if identity.URI != "" && strings.HasPrefix(identity.Type, "#") {
			index.Resources = append(index.Resources, ResourceDescriptor{identity.URI, identity.Type, append(json.RawMessage(nil), r.Body...)})
		}
		var follow func(map[string]json.RawMessage, string)
		add := func(link, key string) {
			target, err := c.resolve(link, current.uri)
			if err != nil {
				index.Failures = append(index.Failures, ResourceDiscoveryFailure{link, "link", err})
				return
			}
			if target.String() == current.uri || seen[target.String()] {
				return
			}
			if current.depth >= options.MaxDepth {
				index.Truncated = true
				return
			}
			queue = append(queue, node{target.String(), current.depth + 1})
		}
		follow = func(fields map[string]json.RawMessage, parent string) {
			keys := make([]string, 0, len(fields))
			for key := range fields {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				raw := fields[key]
				if key == "Actions" || key == "target" || strings.Contains(key, "@Redfish.ActionInfo") {
					continue
				}
				if !options.IncludeExpensive && expensive[key] {
					continue
				}
				if key == "@odata.id" && parent != "" {
					var link string
					if json.Unmarshal(raw, &link) == nil && link != "" {
						add(link, key)
					}
					continue
				}
				if key == "Members@odata.nextLink" || key == "@odata.nextLink" {
					var link string
					if json.Unmarshal(raw, &link) == nil && link != "" {
						add(link, key)
					}
					continue
				}
				var object map[string]json.RawMessage
				if json.Unmarshal(raw, &object) == nil && object != nil {
					follow(object, key)
					continue
				}
				var array []map[string]json.RawMessage
				if json.Unmarshal(raw, &array) == nil {
					for _, object := range array {
						follow(object, key)
					}
				}
			}
		}
		follow(document, "")
	}
	return index, nil
}
func (c *Client) FindResources(ctx context.Context, odataType string, where Object, options DiscoveryOptions) (*ResourceIndex, error) {
	index, err := c.DiscoverResources(ctx, options)
	if err != nil {
		return index, err
	}
	return index.Find(odataType, where), nil
}

var _ = fmt.Stringer(ResourceDescriptor{})

func (r ResourceDescriptor) Format(state fmt.State, verb rune) { fmt.Fprint(state, r.String()) }
