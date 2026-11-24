package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
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
	Excluded      []string
	LimitsReached []string
	Resources     []ResourceDescriptor
	Failures      []ResourceDiscoveryFailure
	Truncated     bool
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
	out := &ResourceIndex{Failures: slices.Clone(index.Failures), Truncated: index.Truncated, Excluded: slices.Clone(index.Excluded), LimitsReached: slices.Clone(index.LimitsReached)}
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

const (
	_discoveryDepthLimit    = "max_depth"
	_discoveryResourceLimit = "max_resources"
)

var _resourceType = regexp.MustCompile(`^#[A-Za-z_][A-Za-z_0-9]*(\.[A-Za-z_][A-Za-z_0-9]*)+$`)

func expensiveResource(value string) bool {
	switch strings.ToLower(value) {
	case "entries", "tasks", "jsonschemas", "registries", "sessions", "logservices", "logservice", "logservicecollection", "logentry", "logentrycollection", "telemetryservice", "metricreports", "metricreport", "metricreportcollection", "metricreportdefinitions", "metricreportdefinitioncollection", "metricdefinitions", "metricdefinitioncollection", "triggers", "triggercollection":
		return true
	}
	return false
}

func (c *Client) discoveryURL(link, parent string) (*url.URL, error) {
	if strings.HasPrefix(link, "//") || strings.Contains(link, "\\") || strings.IndexFunc(link, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil, errors.New("invalid discovered resource link")
	}
	target, err := c.resolve(link, parent)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(target.Path, "/") {
		if part == "." || part == ".." || strings.Contains(part, "\\") || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return nil, errors.New("ambiguous discovered resource path")
		}
	}
	return target, nil
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
	start, err := c.discoveryURL(options.Start, c.base.String()+"/")
	if err != nil {
		return nil, err
	}
	scope := strings.TrimRight(start.Path, "/")
	excludedPath := func(target *url.URL) bool {
		if strings.TrimRight(target.Path, "/") != scope && !strings.HasPrefix(target.Path, scope+"/") {
			return true
		}
		if !options.IncludeExpensive {
			for _, part := range strings.Split(target.Path, "/") {
				if expensiveResource(part) {
					return true
				}
			}
		}
		return false
	}
	type node struct {
		uri   string
		depth int
	}
	queue := []node{{uri: start.String(), depth: 0}}
	scheduled := map[string]bool{start.String(): true}
	resources := map[string]bool{}
	excluded := map[string]bool{}
	limits := map[string]bool{}
	budget := 1
	index := &ResourceIndex{Resources: []ResourceDescriptor{}}
	defer func() {
		for uri := range excluded {
			index.Excluded = append(index.Excluded, uri)
		}
		for limit := range limits {
			index.LimitsReached = append(index.LimitsReached, limit)
		}
		slices.Sort(index.Excluded)
		slices.Sort(index.LimitsReached)
		index.Truncated = len(limits) > 0
	}()
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return index, err
		}
		current := queue[0]
		queue = queue[1:]
		target, _ := url.Parse(current.uri)
		if excludedPath(target) {
			excluded[target.RequestURI()] = true
			continue
		}
		response, err := c.Request(ctx, http.MethodGet, current.uri, nil)
		if err != nil {
			index.Failures = append(index.Failures, ResourceDiscoveryFailure{URI: target.RequestURI(), Reason: "request", Err: err})
			continue
		}
		var identity struct {
			URI  string `json:"@odata.id"`
			Type string `json:"@odata.type"`
		}
		if err := response.Decode(&identity); err != nil {
			index.Failures = append(index.Failures, ResourceDiscoveryFailure{URI: target.RequestURI(), Reason: "identity", Err: err})
			continue
		}
		identityURL, err := c.discoveryURL(identity.URI, current.uri)
		if err != nil || !_resourceType.MatchString(identity.Type) {
			if err == nil {
				err = errors.New("resource has missing or invalid @odata.id or @odata.type")
			}
			index.Failures = append(index.Failures, ResourceDiscoveryFailure{URI: target.RequestURI(), Reason: "identity", Err: err})
			continue
		}
		parts := strings.Split(identity.Type, ".")
		if excludedPath(identityURL) || (!options.IncludeExpensive && expensiveResource(parts[len(parts)-1])) {
			excluded[target.RequestURI()] = true
			continue
		}
		if !resources[identityURL.String()] {
			resources[identityURL.String()] = true
			index.Resources = append(index.Resources, ResourceDescriptor{URI: identityURL.RequestURI(), ODataType: identity.Type, Properties: append(json.RawMessage(nil), response.Body...)})
		}
		var document map[string]json.RawMessage
		if err := response.Decode(&document); err != nil {
			index.Failures = append(index.Failures, ResourceDiscoveryFailure{URI: target.RequestURI(), Reason: "decode", Err: err})
			continue
		}
		add := func(link string, paging bool) {
			destination, err := c.discoveryURL(link, current.uri)
			if err != nil {
				if budget >= options.MaxResources {
					limits[_discoveryResourceLimit] = true
					return
				}
				budget++
				index.Failures = append(index.Failures, ResourceDiscoveryFailure{URI: link, Reason: "link", Err: err})
				return
			}
			if scheduled[destination.String()] {
				return
			}
			if excludedPath(destination) {
				excluded[destination.RequestURI()] = true
				return
			}
			depth := current.depth
			if !paging {
				depth++
			}
			if depth > options.MaxDepth {
				limits[_discoveryDepthLimit] = true
				return
			}
			if budget >= options.MaxResources {
				limits[_discoveryResourceLimit] = true
				return
			}
			budget++
			scheduled[destination.String()] = true
			next := node{uri: destination.String(), depth: depth}
			if paging {
				queue = append([]node{next}, queue...)
			} else {
				queue = append(queue, next)
			}
		}
		var follow func(map[string]json.RawMessage, bool)
		follow = func(fields map[string]json.RawMessage, top bool) {
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
				if !options.IncludeExpensive && expensiveResource(key) {
					var link struct {
						URI string `json:"@odata.id"`
					}
					if json.Unmarshal(raw, &link) == nil && link.URI != "" {
						excluded[link.URI] = true
					}
					continue
				}
				if key == "@odata.id" {
					if !top {
						var link string
						if json.Unmarshal(raw, &link) == nil {
							add(link, false)
						}
					}
					continue
				}
				if key == "Members@odata.nextLink" || key == "@odata.nextLink" {
					var link string
					if json.Unmarshal(raw, &link) == nil {
						add(link, true)
					}
					continue
				}
				var object map[string]json.RawMessage
				if json.Unmarshal(raw, &object) == nil && object != nil {
					follow(object, false)
					continue
				}
				var array []map[string]json.RawMessage
				if json.Unmarshal(raw, &array) == nil {
					for _, object := range array {
						follow(object, false)
					}
				}
			}
		}
		follow(document, true)
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
