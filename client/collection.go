package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

type CollectionFailure struct {
	Stage, URI string
	Err        error
}

type CollectionResult[T any] struct {
	Members  []T
	Failures []CollectionFailure
}

func (r CollectionResult[T]) IsPartial() bool { return len(r.Failures) > 0 }

func (c *Client) Root(ctx context.Context) (*models.RootService, error) {
	c.rootMu.Lock()
	defer c.rootMu.Unlock()
	if c.root == nil {
		var root models.RootService
		if err := c.Get(ctx, c.ServiceRoot(), &root); err != nil {
			return nil, err
		}
		c.root = &root
	}
	return cloneDTO(c.root), nil
}

func requireLink(link *models.Link, name string) (string, error) {
	if link == nil || strings.TrimSpace(pointer.Value(link.ODataID)) == "" {
		return "", fmt.Errorf("resource does not advertise %s: %w", name, sdkerrors.ErrNotFound)
	}
	return pointer.Value(link.ODataID), nil
}

func (c *Client) rootLink(ctx context.Context, key string) (string, error) {
	root, err := c.Root(ctx)
	if err != nil {
		return "", err
	}
	var link *models.Link
	switch key {
	case "Systems":
		link = root.Systems
	case "Chassis":
		link = root.Chassis
	case "Managers":
		link = root.Managers
	case "AccountService":
		link = root.AccountService
	case "EventService":
		link = root.EventService
	case "SessionService":
		link = root.SessionService
	case "TaskService":
		link = root.TaskService
	case "UpdateService":
		link = root.UpdateService
	case "Registries":
		link = root.Registries
	case "TelemetryService":
		link = root.TelemetryService
	case "CompositionService":
		link = root.CompositionService
	case "PolicyService":
		link = root.PolicyService
	}
	return requireLink(link, key)
}

func getResource[T any](ctx context.Context, c *Client, uri string) (*T, error) {
	var result T
	if err := c.Get(ctx, uri, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func collection[T any](ctx context.Context, c *Client, uri string) ([]T, error) {
	var result []T
	err := c.GetCollection(ctx, uri, &result)
	return result, err
}

// GetCollection decodes each linked or expanded member directly into dest's element type.
// Structural failures and failed reads are returned; use Collect for explicit best-effort reads.
func (c *Client) GetCollection(ctx context.Context, uri string, dest any) error {
	output := reflect.ValueOf(dest)
	if output.Kind() != reflect.Pointer || output.IsNil() || output.Elem().Kind() != reflect.Slice {
		return errors.New("collection destination must be a non-nil pointer to a slice")
	}
	output = output.Elem()
	result := reflect.MakeSlice(output.Type(), 0, 0)
	failures, err := c.walkCollection(ctx, uri, func(data []byte, resourceURI string) error {
		item := reflect.New(output.Type().Elem())
		if err := json.Unmarshal(data, item.Interface()); err != nil {
			return err
		}
		if err := validateResourceResponse(item.Interface(), resourceURI); err != nil {
			return err
		}
		result = reflect.Append(result, item.Elem())
		return nil
	}, !c.cfg.strictCollections)
	if err != nil {
		return err
	}
	output.Set(result)
	if len(failures) > 0 {
		return &PartialCollectionError{Failures: failures}
	}
	return nil
}

func Collect[T any](ctx context.Context, c *Client, uri string) (CollectionResult[T], error) {
	result := CollectionResult[T]{Members: make([]T, 0)}
	failures, err := c.walkCollection(ctx, uri, func(data []byte, resourceURI string) error {
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		if err := validateResourceResponse(&item, resourceURI); err != nil {
			return err
		}
		result.Members = append(result.Members, item)
		return nil
	}, true)
	result.Failures = failures
	return result, err
}

func (c *Client) walkCollection(ctx context.Context, uri string, consume func([]byte, string) error, bestEffort bool) ([]CollectionFailure, error) {
	seenPages, seenMembers := map[string]bool{}, map[string]bool{}
	var failures []CollectionFailure
	pageURL, err := c.resolve(uri, c.base.String()+"/")
	if err != nil {
		return nil, err
	}
	for pageURL != nil {
		current := pageURL.String()
		if seenPages[current] {
			return failures, errors.New("collection pagination cycle detected")
		}
		seenPages[current] = true
		r, err := c.Request(ctx, "GET", current, nil)
		if err != nil {
			if bestEffort {
				failures = append(failures, CollectionFailure{Stage: "page", URI: current, Err: err})
				return failures, nil
			}
			return failures, err
		}
		var page models.Collection[json.RawMessage]
		if err = r.Decode(&page); err != nil {
			return failures, err
		}
		if page.Members == nil {
			return failures, errors.New("collection is missing Members")
		}
		for _, raw := range page.Members {
			var identity models.Entity
			if err = json.Unmarshal(raw, &identity); err != nil {
				return failures, err
			}
			link := pointer.Value(identity.ODataID)
			if strings.TrimSpace(link) == "" {
				return failures, errors.New("collection member is missing @odata.id")
			}
			target, err := c.resolve(link, current)
			if err != nil {
				return failures, err
			}
			if seenMembers[target.String()] {
				continue
			}
			seenMembers[target.String()] = true
			// An expanded member includes resource properties beyond its OData identity.
			var keys map[string]json.RawMessage
			if err = json.Unmarshal(raw, &keys); err != nil {
				return failures, err
			}
			expanded := false
			for k := range keys {
				if !strings.HasPrefix(k, "@odata.") {
					expanded = true
					break
				}
			}
			data := []byte(raw)
			if !expanded {
				response, getErr := c.Request(ctx, "GET", target.String(), nil)
				if getErr != nil {
					if bestEffort {
						failures = append(failures, CollectionFailure{Stage: "member", URI: target.String(), Err: getErr})
						continue
					}
					return failures, getErr
				}
				data = response.Body
			}
			if err = consume(data, target.String()); err != nil {
				if bestEffort {
					failures = append(failures, CollectionFailure{Stage: "member", URI: target.String(), Err: err})
					continue
				}
				return failures, err
			}
		}
		next := pointer.Value(page.MembersNextLink)
		if next == "" {
			next = pointer.Value(page.NextLink)
		}
		if next == "" {
			pageURL = nil
		} else {
			pageURL, err = c.resolve(next, current)
			if err != nil {
				return failures, err
			}
		}
	}
	return failures, nil
}

func selectResource[T any](ctx context.Context, c *Client, collectionURI, id string) (*T, error) {
	var result *T
	var count int
	_, err := c.walkCollection(ctx, collectionURI, func(raw []byte, resourceURI string) error {
		var identity models.Entity
		if err := json.Unmarshal(raw, &identity); err != nil {
			return err
		}
		match := id == "" || (identity.ID != nil && string(*identity.ID) == id) || pointer.Value(identity.ODataID) == id
		if !match {
			u, err := url.Parse(pointer.Value(identity.ODataID))
			match = err == nil && strings.TrimSuffix(u.Path, "/") != "" && strings.TrimSuffix(u.Path, "/")[strings.LastIndex(strings.TrimSuffix(u.Path, "/"), "/")+1:] == id
		}
		if match {
			var item T
			if err := json.Unmarshal(raw, &item); err != nil {
				return err
			}
			if err := validateResourceResponse(&item, resourceURI); err != nil {
				return err
			}
			result = &item
			count++
		}
		return nil
	}, false)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("%w: %s", sdkerrors.ErrNotFound, id)
	}
	if count > 1 {
		return nil, errors.New("resource selection is ambiguous; specify an identifier")
	}
	return result, nil
}

func optionalID(ids []string) (string, error) {
	if len(ids) > 1 {
		return "", errors.New("at most one resource identifier is allowed")
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	return "", nil
}

func rootResource[T any](ctx context.Context, c *Client, key string) (*T, error) {
	uri, err := c.rootLink(ctx, key)
	if err != nil {
		return nil, err
	}
	return getResource[T](ctx, c, uri)
}

func rootCollection[T any](ctx context.Context, c *Client, key string) ([]T, error) {
	uri, err := c.rootLink(ctx, key)
	if err != nil {
		return nil, err
	}
	return collection[T](ctx, c, uri)
}

func rootSelect[T any](ctx context.Context, c *Client, key string, ids []string) (*T, error) {
	id, err := optionalID(ids)
	if err != nil {
		return nil, err
	}
	uri, err := c.rootLink(ctx, key)
	if err != nil {
		return nil, err
	}
	return selectResource[T](ctx, c, uri, id)
}

func linkedResource[T any](ctx context.Context, c *Client, link *models.Link, key string) (*T, error) {
	uri, err := requireLink(link, key)
	if err != nil {
		return nil, err
	}
	return getResource[T](ctx, c, uri)
}

func linkedCollection[T any](ctx context.Context, c *Client, link *models.Link, key string) ([]T, error) {
	uri, err := requireLink(link, key)
	if err != nil {
		return nil, err
	}
	return collection[T](ctx, c, uri)
}

// GetResourceODataID reads the standard identity without guessing a resource path.
func GetResourceODataID(resource any) (string, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}
	var link models.Link
	if err = json.Unmarshal(data, &link); err != nil {
		return "", err
	}
	return requireLink(&link, "@odata.id")
}

func (c *Client) GetODataID(ctx context.Context, key models.RedfishResource) (string, error) {
	root, err := c.GetRaw(ctx, c.ServiceRoot())
	if err != nil {
		return "", err
	}
	find := func(data Object) (string, error) {
		raw, err := json.Marshal(data)
		if err != nil {
			return "", err
		}
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(raw, &fields); err != nil {
			return "", err
		}
		uri, err := findOEMLink(fields, string(key))
		if errors.Is(err, sdkerrors.ErrNotFound) {
			return "", nil
		}
		return uri, err
	}
	if uri, err := find(root); uri != "" || err != nil {
		return uri, err
	}
	for _, name := range []string{"Systems", "Chassis", "Managers"} {
		raw, ok := root[name]
		if !ok {
			continue
		}
		encoded, err := json.Marshal(raw)
		if err != nil {
			return "", err
		}
		var link models.Link
		if err = json.Unmarshal(encoded, &link); err != nil {
			return "", err
		}
		uri, err := requireLink(&link, name)
		if err != nil {
			return "", err
		}
		var page models.Collection[models.Link]
		if err = c.Get(ctx, uri, &page); err != nil {
			if errors.Is(err, sdkerrors.ErrNotFound) {
				continue
			}
			return "", err
		}
		if len(page.Members) == 0 {
			continue
		}
		data, err := c.GetRaw(ctx, pointer.Value(page.Members[0].ODataID))
		if err != nil {
			return "", err
		}
		if uri, err := find(data); uri != "" || err != nil {
			return uri, err
		}
	}
	return "", nil
}

func (c *Client) GetResourceByAlias(ctx context.Context, collectionURI, alias string, dest any) error {
	uri, err := c.ResourceAliasURI(ctx, collectionURI, alias)
	if err != nil {
		return err
	}
	return c.Get(ctx, uri, dest)
}

func validateAlias(alias string, allowEmpty bool) error {
	if alias == "" {
		if allowEmpty {
			return nil
		}
		return errors.New("alias must not be empty")
	}
	for _, char := range alias {
		if !(char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-') {
			return errors.New("alias contains unsupported characters")
		}
	}
	return nil
}

func (c *Client) requireAliasCapability(ctx context.Context) error {
	root, err := c.Root(ctx)
	if err != nil {
		return err
	}
	if root.ProtocolFeaturesSupported == nil || !pointer.Value(root.ProtocolFeaturesSupported.Alias) {
		return errors.New("service does not advertise alias capability")
	}
	return nil
}

func (c *Client) ResourceAliasURI(ctx context.Context, collectionURI, alias string) (string, error) {
	if err := validateAlias(alias, false); err != nil {
		return "", err
	}
	if err := c.requireAliasCapability(ctx); err != nil {
		return "", err
	}
	target, err := c.resolve(collectionURI, c.base.String()+"/")
	if err != nil {
		return "", err
	}
	if target.RawQuery != "" {
		return "", errors.New("alias collection URL must not contain a query")
	}
	return strings.TrimRight(target.EscapedPath(), "/") + "/~" + alias, nil
}

func (c *Client) SetResourceAlias(ctx context.Context, uri, alias string) (*sdkresponse.Response, error) {
	if err := validateAlias(alias, true); err != nil {
		return nil, err
	}
	if err := c.requireAliasCapability(ctx); err != nil {
		return nil, err
	}
	return c.patchWithETag(ctx, uri, Object{"@Redfish.Alias": alias})
}

func (c *Client) ClearResourceAlias(ctx context.Context, uri string) (*sdkresponse.Response, error) {
	return c.SetResourceAlias(ctx, uri, "")
}

func (c *Client) GetResourceODataID(resource any) (string, error) {
	return GetResourceODataID(resource)
}

func validJSON(data []byte) bool { return len(bytes.TrimSpace(data)) > 0 && json.Valid(data) }

// PartialCollectionError reports failures after all available members were decoded.
type PartialCollectionError struct{ Failures []CollectionFailure }

func (e *PartialCollectionError) Error() string {
	return fmt.Sprintf("collection is partial: %d retrieval failure(s)", len(e.Failures))
}

func (e *PartialCollectionError) Unwrap() []error {
	out := make([]error, len(e.Failures))
	for i, failure := range e.Failures {
		out[i] = failure.Err
	}
	return out
}

// cloneDTO keeps cached response data private without JSON conversion.
func cloneDTO[T any](source T) T {
	var copyValue func(reflect.Value) reflect.Value
	copyValue = func(value reflect.Value) reflect.Value {
		switch value.Kind() {
		case reflect.Pointer:
			if value.IsNil() {
				return reflect.Zero(value.Type())
			}
			out := reflect.New(value.Type().Elem())
			out.Elem().Set(copyValue(value.Elem()))
			return out
		case reflect.Struct:
			out := reflect.New(value.Type()).Elem()
			for i := 0; i < value.NumField(); i++ {
				out.Field(i).Set(copyValue(value.Field(i)))
			}
			return out
		case reflect.Slice:
			if value.IsNil() {
				return reflect.Zero(value.Type())
			}
			out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
			for i := 0; i < value.Len(); i++ {
				out.Index(i).Set(copyValue(value.Index(i)))
			}
			return out
		case reflect.Map:
			if value.IsNil() {
				return reflect.Zero(value.Type())
			}
			out := reflect.MakeMapWithSize(value.Type(), value.Len())
			it := value.MapRange()
			for it.Next() {
				out.SetMapIndex(it.Key(), copyValue(it.Value()))
			}
			return out
		default:
			return value
		}
	}
	return copyValue(reflect.ValueOf(source)).Interface().(T)
}
