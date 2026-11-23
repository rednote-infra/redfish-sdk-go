package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

const _maxRegistryCacheEntries = 32

var _messageArgument = regexp.MustCompile(`%([0-9]+)`)

func (c *Client) GetRegistry(ctx context.Context, id string) (*models.Registry, error) {
	registries, err := c.GetRegistries(ctx)
	if err != nil {
		return nil, err
	}
	return selectItem(registries, id)
}

func parseRegistryVersion(value string) ([3]int, bool) {
	var version [3]int
	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return version, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version, false
		}
		version[i] = n
	}
	return version, true
}

func registryIdentity(registry models.Registry) (string, [3]int) {
	prefix := pointer.Value(registry.RegistryPrefix)
	version, _ := parseRegistryVersion(pointer.Value(registry.RegistryVersion))
	parts := strings.Split(prefix, ".")
	if len(parts) >= 3 {
		if parsed, ok := parseRegistryVersion(strings.Join(parts[1:], ".")); ok {
			return parts[0], parsed
		}
	}
	if prefix == "" {
		parts = strings.Split(registry.ResourceID(), ".")
		if len(parts) >= 3 {
			if parsed, ok := parseRegistryVersion(strings.Join(parts[1:], ".")); ok {
				return parts[0], parsed
			}
		}
	}
	return prefix, version
}

func (c *Client) findRegistry(ctx context.Context, reference string) (*models.Registry, error) {
	if strings.TrimSpace(reference) == "" {
		return nil, errors.New("registry reference is required")
	}
	registries, err := c.GetRegistries(ctx)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(reference, ".")
	requestedPrefix := parts[0]
	var required [3]int
	pinned := false
	versioned := false
	if len(parts) >= 3 {
		required, versioned = parseRegistryVersion(strings.Join(parts[1:], "."))
		pinned = len(parts) == 4
	}
	var candidates []models.Registry
	for _, registry := range registries {
		if registry.ResourceURI() == reference {
			return &registry, nil
		}
		prefix, version := registryIdentity(registry)
		matches := registry.ResourceID() == reference || strings.EqualFold(prefix, reference)
		if versioned {
			matches = strings.EqualFold(prefix, requestedPrefix) && version[0] == required[0] && version[1] == required[1] && (!pinned || version == required)
		}
		if matches {
			candidates = append(candidates, registry)
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("requested registry/version is not advertised")
	}
	slices.SortFunc(candidates, func(a, b models.Registry) int {
		_, av := registryIdentity(a)
		_, bv := registryIdentity(b)
		for i := range av {
			if av[i] != bv[i] {
				return bv[i] - av[i]
			}
		}
		return strings.Compare(a.ResourceID(), b.ResourceID())
	})
	return &candidates[0], nil
}

func registryLocation(registry *models.Registry, language string) (*models.RegistryLocation, error) {
	if len(registry.Location) == 0 {
		return nil, errors.New("registry has no advertised locations")
	}
	locations := slices.Clone(registry.Location)
	rank := func(location models.RegistryLocation) int {
		candidate := strings.ToLower(pointer.Value(location.Language))
		requested := strings.ToLower(language)
		if pointer.Value(location.URI) == "" && pointer.Value(location.PublicationURI) == "" {
			return 5
		}
		if requested != "" && candidate == requested {
			return 0
		}
		if requested != "" && strings.Split(candidate, "-")[0] == strings.Split(requested, "-")[0] {
			return 1
		}
		if candidate == "en" {
			return 2
		}
		return 3
	}
	slices.SortFunc(locations, func(a, b models.RegistryLocation) int {
		if rank(a) != rank(b) {
			return rank(a) - rank(b)
		}
		return strings.Compare(pointer.Value(a.Language)+pointer.Value(a.URI), pointer.Value(b.Language)+pointer.Value(b.URI))
	})
	return &locations[0], nil
}

func (c *Client) fetchRegistry(ctx context.Context, source string, dest any) error {
	parsed, err := url.Parse(source)
	if err != nil {
		return err
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return errors.New("registry URL must not contain credentials or fragments")
	}
	if _, err = c.resolve(source, c.base.String()+"/"); err == nil {
		return c.Get(ctx, source, dest)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("registry publication must use HTTP(S)")
	}
	// Publication hosts get a separate unauthenticated client and trust configuration.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: c.cfg.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, _maxJSONBytes+1))
	if err != nil {
		return err
	}
	r := &sdkresponse.Response{StatusCode: response.StatusCode, Header: response.Header, Body: data, URL: source}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sdkerrors.NewHTTPError(r)
	}
	if int64(len(data)) > _maxJSONBytes {
		return errors.New("registry exceeds size limit")
	}
	return json.Unmarshal(data, dest)
}

func (c *Client) ClearRegistryCache() {
	c.registryMu.Lock()
	defer c.registryMu.Unlock()
	c.registryCache = make(map[string]*models.MessageRegistry)
}

func (c *Client) messageRegistry(ctx context.Context, registry *models.Registry, language string) (*models.MessageRegistry, error) {
	location, err := registryLocation(registry, language)
	if err != nil {
		return nil, err
	}
	var last error
	for _, source := range []string{pointer.Value(location.URI), pointer.Value(location.PublicationURI)} {
		if source == "" {
			continue
		}
		c.registryMu.Lock()
		cached := c.registryCache[source]
		c.registryMu.Unlock()
		if cached != nil {
			return cached, nil
		}
		var document models.MessageRegistry
		if err = c.fetchRegistry(ctx, source, &document); err != nil {
			if sdkerrors.IsStatus(err, 401) || sdkerrors.IsStatus(err, 403) {
				return nil, err
			}
			last = err
			continue
		}
		if strings.Contains(pointer.Value(document.ODataType), "AttributeRegistry") || document.Extra["RegistryEntries"] != nil {
			return nil, errors.New("attribute registry cannot be used as a message registry")
		}
		prefix, version := registryIdentity(*registry)
		if pointer.Value(document.RegistryPrefix) != "" && !strings.EqualFold(pointer.Value(document.RegistryPrefix), prefix) {
			return nil, errors.New("registry document prefix differs from advertised metadata")
		}
		if document.RegistryVersion != nil {
			actual, ok := parseRegistryVersion(*document.RegistryVersion)
			if !ok || (registryVersionKnown(*registry) && actual != version) {
				return nil, errors.New("registry document version differs from advertised metadata")
			}
		}
		c.registryMu.Lock()
		if len(c.registryCache) >= _maxRegistryCacheEntries {
			c.registryCache = make(map[string]*models.MessageRegistry)
		}
		c.registryCache[source] = &document
		c.registryMu.Unlock()
		return &document, nil
	}
	if last == nil {
		last = errors.New("registry location has no usable URI")
	}
	return nil, last
}

func (c *Client) ResolveMessage(ctx context.Context, messageID string, args []any, language string) (*models.ResolvedMessage, error) {
	parts := strings.Split(messageID, ".")
	if len(parts) != 4 {
		return nil, errors.New("MessageId must be Prefix.Major.Minor.MessageKey")
	}
	reference := strings.Join(parts[:3], ".")
	key := parts[3]
	trail := map[string]bool{}
	for {
		if trail[reference] {
			return nil, errors.New("message registry reference cycle")
		}
		trail[reference] = true
		registry, err := c.findRegistry(ctx, reference)
		if err != nil {
			return nil, err
		}
		document, err := c.messageRegistry(ctx, registry, language)
		if err != nil {
			return nil, err
		}
		if next := pointer.Value(document.ReferencedRegistry); next != "" {
			reference = next
			continue
		}
		definition, ok := document.Messages[key]
		if !ok {
			return nil, errors.New("message key is not present in registry")
		}
		if next := pointer.Value(definition.ReferencedRegistry); next != "" {
			reference = next
			continue
		}
		if definition.Message == nil {
			return nil, errors.New("registry message has no text")
		}
		values := make([]string, len(args))
		for i, arg := range args {
			values[i] = fmt.Sprint(arg)
		}
		if args != nil && definition.NumberOfArgs != nil && int64(len(args)) != *definition.NumberOfArgs {
			return nil, errors.New("registry message argument count mismatch")
		}
		text := *definition.Message
		var substitutionErr error
		if args != nil {
			text = _messageArgument.ReplaceAllStringFunc(text, func(token string) string {
				index, _ := strconv.Atoi(token[1:])
				if index < 1 || index > len(values) {
					substitutionErr = errors.New("registry message references a missing argument")
					return token
				}
				return values[index-1]
			})
		}
		if substitutionErr != nil {
			return nil, substitutionErr
		}
		prefix, version := registryIdentity(*registry)
		return &models.ResolvedMessage{RequestedMessageID: messageID, MessageID: fmt.Sprintf("%s.%d.%d.%s", prefix, version[0], version[1], key), RegistryPrefix: prefix, RegistryVersion: fmt.Sprintf("%d.%d.%d", version[0], version[1], version[2]), MessageKey: key, Message: text, MessageArgs: values, MessageSeverity: clonePointer(definition.MessageSeverity), Resolution: clonePointer(definition.Resolution)}, nil
	}
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return pointer.Ptr(*value)
}

func (c *Client) GetAttributeRegistry(ctx context.Context, reference, language string) (*models.AttributeRegistry, error) {
	registry, err := c.findRegistry(ctx, reference)
	if err != nil {
		return nil, err
	}
	location, err := registryLocation(registry, language)
	if err != nil {
		return nil, err
	}
	var last error
	for _, source := range []string{pointer.Value(location.URI), pointer.Value(location.PublicationURI)} {
		if source == "" {
			continue
		}
		var document models.AttributeRegistry
		if err = c.fetchRegistry(ctx, source, &document); err != nil {
			if sdkerrors.IsStatus(err, 401) || sdkerrors.IsStatus(err, 403) {
				return nil, err
			}
			last = err
			continue
		}
		if document.RegistryEntries == nil {
			return nil, errors.New("attribute registry is missing RegistryEntries")
		}
		if document.RegistryVersion != nil {
			_, expected := registryIdentity(*registry)
			actual, ok := parseRegistryVersion(*document.RegistryVersion)
			if !ok || (registryVersionKnown(*registry) && actual != expected) {
				return nil, errors.New("attribute registry version differs from advertised metadata")
			}
		}
		return &document, nil
	}
	if last == nil {
		last = errors.New("attribute registry has no usable location")
	}
	return nil, last
}

func (c *Client) GetBIOSAttributeRegistry(ctx context.Context, systemID, language string) (*models.AttributeRegistry, error) {
	bios, err := c.GetBIOS(ctx, systemID)
	if err != nil {
		return nil, err
	}
	return c.GetAttributeRegistry(ctx, pointer.Value(bios.AttributeRegistry), language)
}

func (c *Client) ValidateBIOSAttributes(ctx context.Context, attributes Object, systemID, language string) error {
	registry, err := c.GetBIOSAttributeRegistry(ctx, systemID, language)
	if err != nil {
		return err
	}
	return ValidateAttributeValues(registry, attributes)
}

// ValidateAttributeValues checks static constraints. Runtime dependencies remain BMC decisions.
func ValidateAttributeValues(registry *models.AttributeRegistry, values Object) error {
	if registry == nil || registry.RegistryEntries == nil {
		return errors.New("attribute registry entries are missing")
	}
	definitions := map[string]models.AttributeDefinition{}
	for _, definition := range registry.RegistryEntries.Attributes {
		definitions[definition.AttributeName] = definition
	}
	for name, value := range values {
		definition, ok := definitions[name]
		if !ok {
			return fmt.Errorf("attribute %q is not advertised", name)
		}
		reason := ""
		switch {
		case pointer.Value(definition.ReadOnly) || pointer.Value(definition.Immutable):
			reason = "is read-only or immutable"
		default:
			switch pointer.Value(definition.Type) {
			case "Boolean":
				if _, ok := value.(bool); !ok {
					reason = "requires a boolean"
				}
			case "Integer":
				var number int64
				kind := reflect.Invalid
				if value != nil {
					kind = reflect.TypeOf(value).Kind()
				}
				switch kind {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					number = reflect.ValueOf(value).Int()
				default:
					reason = "requires an integer"
				}
				if reason == "" && ((definition.LowerBound != nil && number < *definition.LowerBound) || (definition.UpperBound != nil && number > *definition.UpperBound)) {
					reason = "is outside its advertised bounds"
				}
			case "String", wire.FieldPassword, "Enumeration":
				text, ok := value.(string)
				if !ok {
					reason = "requires a string"
				} else if (definition.MinLength != nil && int64(utf8.RuneCountInString(text)) < *definition.MinLength) || (definition.MaxLength != nil && int64(utf8.RuneCountInString(text)) > *definition.MaxLength) {
					reason = "does not satisfy its advertised string length"
				} else if pointer.Value(definition.Type) == "Enumeration" {
					valid := false
					for _, entry := range definition.Values {
						if entry.ValueName == text {
							valid = true
							break
						}
					}
					if !valid {
						reason = "is not an advertised enumeration value"
					}
				}
			default:
				reason = "has no supported type for local validation"
			}
		}
		if reason != "" {
			return fmt.Errorf("attribute %q %s", name, reason)
		}
	}
	return nil
}

func registryVersionKnown(registry models.Registry) bool {
	if _, ok := parseRegistryVersion(pointer.Value(registry.RegistryVersion)); ok {
		return true
	}
	for _, name := range []string{pointer.Value(registry.RegistryPrefix), registry.ResourceID()} {
		parts := strings.Split(name, ".")
		if len(parts) >= 3 {
			if _, ok := parseRegistryVersion(strings.Join(parts[1:], ".")); ok {
				return true
			}
		}
	}
	return false
}
