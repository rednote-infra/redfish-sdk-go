package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type ChangeStatus string

const (
	ChangePending   ChangeStatus = "pending"
	ChangeValidated ChangeStatus = "validated"
	ChangeSucceeded ChangeStatus = "succeeded"
	ChangeAccepted  ChangeStatus = "accepted"
	ChangeFailed    ChangeStatus = "failed"
	ChangeBlocked   ChangeStatus = "blocked"
	ChangeSkipped   ChangeStatus = "skipped"
	ChangeUncertain ChangeStatus = "uncertain"
)

type ChangeOptions struct {
	ETag        string
	IfNoneMatch string
	Headers     http.Header
}
type ChangeOperation struct {
	Method, URI, ETag string
	IfNoneMatch       string
	Body              json.RawMessage
	Before            json.RawMessage
	Headers           http.Header
}
type ChangeValidationIssue struct {
	Index   int
	Message string
}
type ChangeOperationResult struct {
	Operation ChangeOperation
	Status    ChangeStatus
	Response  *Response
	Err       error
}
type ChangeSetResult struct {
	Operations []ChangeOperationResult
	DryRun     bool
}

func (r *ChangeSetResult) Successful() bool {
	if r == nil || r.DryRun {
		return false
	}
	for _, op := range r.Operations {
		if op.Status != ChangeSucceeded {
			return false
		}
	}
	return true
}
func (r *ChangeSetResult) Pending() bool {
	if r == nil {
		return false
	}
	for _, op := range r.Operations {
		if op.Status == ChangeAccepted || op.Status == ChangePending {
			return true
		}
	}
	return false
}
func (r *ChangeSetResult) HasUncertainOutcomes() bool {
	if r == nil {
		return false
	}
	for _, op := range r.Operations {
		if op.Status == ChangeUncertain {
			return true
		}
	}
	return false
}
func (r *ChangeSetResult) IsPartial() bool {
	if r == nil {
		return false
	}
	success, failure := false, false
	for _, op := range r.Operations {
		if op.Status == ChangeSucceeded || op.Status == ChangeAccepted {
			success = true
		} else {
			failure = true
		}
	}
	return success && failure
}

type CommitOptions struct{ DryRun, SkipETagCheck, ContinueOnError bool }

// ChangeSet stages immutable request snapshots; it never retries or rolls back mutations.
type ChangeSet struct {
	client     *Client
	mu         sync.Mutex
	operations []ChangeOperation
	committed  bool
}

func (c *Client) ChangeSet() *ChangeSet { return &ChangeSet{client: c} }
func (s *ChangeSet) Stage(method, uri string, body any, options ChangeOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.committed {
		return errors.New("change set has already been committed")
	}
	method = strings.ToUpper(method)
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return errors.New("change sets support only mutation methods")
	}
	target, err := s.client.resolve(uri, s.client.base.String()+"/")
	if err != nil {
		return err
	}
	if options.ETag != "" && !validETag(options.ETag) {
		return errors.New("invalid ETag")
	}
	var raw json.RawMessage
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	headers := options.Headers.Clone()
	if headers == nil {
		headers = http.Header{}
	}
	if options.ETag != "" {
		if existing := headers.Get(HeaderIfMatch); existing != "" && existing != options.ETag {
			return errors.New("conflicting If-Match preconditions")
		}
		headers.Set(HeaderIfMatch, options.ETag)
	}
	if options.ETag == "" {
		options.ETag = headers.Get(HeaderIfMatch)
	}
	if options.IfNoneMatch != "" {
		if !validETag(options.IfNoneMatch) {
			return errors.New("invalid If-None-Match validator")
		}
		headers.Set(HeaderIfNoneMatch, options.IfNoneMatch)
	} else {
		options.IfNoneMatch = headers.Get(HeaderIfNoneMatch)
	}
	if options.ETag != "" && options.IfNoneMatch != "" {
		return errors.New("specify only one conditional validator")
	}

	s.operations = append(s.operations, ChangeOperation{Method: method, URI: target.RequestURI(), ETag: options.ETag, IfNoneMatch: options.IfNoneMatch, Headers: headers, Body: raw})
	return nil
}
func validETag(value string) bool {
	if value == "*" {
		return true
	}
	value = strings.TrimPrefix(value, "W/")
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	for _, char := range value[1 : len(value)-1] {
		if char < '!' || char == '"' || char == 127 {
			return false
		}
	}
	return true
}
func (s *ChangeSet) Validate() []ChangeValidationIssue {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.validate()
}
func (s *ChangeSet) validate() []ChangeValidationIssue {
	var issues []ChangeValidationIssue
	for i, op := range s.operations {
		if (op.ETag != "" && !validETag(op.ETag)) || (op.IfNoneMatch != "" && !validETag(op.IfNoneMatch)) {
			issues = append(issues, ChangeValidationIssue{i, "invalid ETag"})
		}
		if len(op.Body) > 0 && !json.Valid(op.Body) {
			issues = append(issues, ChangeValidationIssue{i, "invalid JSON body"})
		}
		if op.Method == http.MethodDelete && len(op.Body) > 0 {
			issues = append(issues, ChangeValidationIssue{i, "DELETE body is not supported in a change set"})
		}
	}
	return issues
}
func (s *ChangeSet) Operations() []ChangeOperation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ChangeOperation, len(s.operations))
	for i, op := range s.operations {
		out[i] = op
		out[i].Body = append(json.RawMessage(nil), op.Body...)
		out[i].Before = append(json.RawMessage(nil), op.Before...)
		out[i].Headers = op.Headers.Clone()
	}
	return out
}
func (s *ChangeSet) Plan(includeSensitive bool) ([]Object, error) {
	ops := s.Operations()
	out := make([]Object, 0, len(ops))
	for i, op := range ops {
		item := Object{"index": i, "method": op.Method, "uri": op.URI, "etag": op.ETag, "headers": op.Headers}
		if len(op.Body) > 0 {
			item["body"] = op.Body
		}
		if len(op.Before) > 0 {
			item["before"] = op.Before
		}
		if !includeSensitive {
			safe, err := Redact(item, s.client.passwordValue(), s.client.tokenSnapshot())
			if err != nil {
				return nil, err
			}
			if err = json.Unmarshal(safe, &item); err != nil {
				return nil, err
			}
		}
		out = append(out, item)
	}
	return out, nil
}
func (s *ChangeSet) ToJSON(includeSensitive bool) ([]byte, error) {
	plan, err := s.Plan(includeSensitive)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(plan, "", "  ")
}
func (s *ChangeSet) CaptureETags(ctx context.Context, includeBefore bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.committed {
		return errors.New("change set has already been committed")
	}
	pending := make([]ChangeOperation, len(s.operations))
	copy(pending, s.operations)
	for i, op := range pending {
		if op.Method == http.MethodPost || op.IfNoneMatch != "" {
			continue
		}
		r, err := s.client.Request(ctx, http.MethodGet, op.URI, nil)
		if err != nil {
			return err
		}
		tag, err := responseETag(r)
		if err != nil {
			return err
		}
		if tag == "" {
			return errors.New("resource does not advertise an ETag")
		}
		pending[i].ETag = tag
		pending[i].Headers = op.Headers.Clone()
		pending[i].Headers.Set(HeaderIfMatch, tag)
		if includeBefore {
			pending[i].Before = append(json.RawMessage(nil), r.Body...)
		}
	}
	s.operations = pending
	return nil
}
func responseETag(r *Response) (string, error) {
	if tag := r.ETag(); tag != "" {
		return tag, nil
	}
	var document struct {
		ETag string `json:"@odata.etag"`
	}
	if err := r.Decode(&document); err != nil {
		return "", err
	}
	return document.ETag, nil
}
func (s *ChangeSet) Commit(ctx context.Context, options CommitOptions) (*ChangeSetResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := &ChangeSetResult{DryRun: options.DryRun, Operations: make([]ChangeOperationResult, len(s.operations))}
	for i, op := range s.operations {
		snapshot := op
		snapshot.Body = append(json.RawMessage(nil), op.Body...)
		snapshot.Before = append(json.RawMessage(nil), op.Before...)
		snapshot.Headers = op.Headers.Clone()
		result.Operations[i] = ChangeOperationResult{Operation: snapshot, Status: ChangePending}
	}
	block := func(err error) (*ChangeSetResult, error) {
		for i := range result.Operations {
			result.Operations[i].Status = ChangeBlocked
			result.Operations[i].Err = err
		}
		return result, err
	}
	if s.committed {
		return block(errors.New("change set has already been committed"))
	}
	if issues := s.validate(); len(issues) > 0 {
		return block(fmt.Errorf("change validation failed at operation %d: %s", issues[0].Index, issues[0].Message))
	}
	if options.DryRun {
		for i := range result.Operations {
			result.Operations[i].Status = ChangeValidated
		}
		return result, nil
	}
	// Preflight every captured validator before sending the first mutation.
	if !options.SkipETagCheck {
		for _, op := range s.operations {
			if op.ETag == "" && op.IfNoneMatch == "" {
				continue
			}
			r, err := s.client.Request(ctx, http.MethodGet, op.URI, nil)
			if err != nil {
				if errors.Is(err, ErrNotFound) && op.IfNoneMatch != "" {
					continue
				}
				return block(err)
			}
			current, err := responseETag(r)
			if err != nil {
				return block(err)
			}
			if op.IfNoneMatch != "" {
				if op.IfNoneMatch == "*" || current == op.IfNoneMatch {
					return block(ErrPreconditionFailed)
				}
				continue
			}
			if op.ETag != "*" && current != op.ETag {
				return block(ErrPreconditionFailed)
			}
		}
	}
	s.committed = true
	var failures []error
	stop := false
	for i, op := range s.operations {
		if stop {
			result.Operations[i].Status = ChangeSkipped
			continue
		}
		var body any
		if len(op.Body) > 0 {
			body = op.Body
		}
		response, err := s.client.Request(ctx, op.Method, op.URI, body, RequestOptions{Headers: op.Headers})
		entry := &result.Operations[i]
		entry.Response = response
		entry.Err = err
		if err != nil {
			entry.Status = ChangeFailed
			var transport *TransportError
			if errors.As(err, &transport) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				entry.Status = ChangeUncertain
			}
			failures = append(failures, err)
			stop = !options.ContinueOnError
			continue
		}
		entry.Status = ChangeSucceeded
		if response.StatusCode == http.StatusAccepted {
			entry.Status = ChangeAccepted
		}
	}
	return result, errors.Join(failures...)
}

func (operation ChangeOperation) Format(state fmt.State, verb rune) {
	fmt.Fprintf(state, "ChangeOperation(%s)", operation.Method)
}
