package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
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
	// Before is a detached preview baseline, not an enforced server precondition.
	Before any
	// BeforeProvided distinguishes an explicit null baseline from an absent baseline.
	BeforeProvided bool
	ETag           string
	IfNoneMatch    string
	Headers        http.Header
}

type ChangeOperation struct {
	Method, URI, ETag string
	IfNoneMatch       string
	Body              json.RawMessage
	Before            json.RawMessage
	Headers           http.Header
}

type ChangeValidationIssue struct {
	Err     error
	Index   int
	Message string
}

type ChangeOperationResult struct {
	OutcomeUnknown bool
	Operation      ChangeOperation
	Status         ChangeStatus
	Response       *sdkresponse.Response
	Err            error
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
		if op.Status == ChangeUncertain || op.OutcomeUnknown {
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

type ChangeValidator func(ChangeOperation) error

type CommitOptions struct {
	DryRun, SkipETagCheck, ContinueOnError bool
	Validator                              ChangeValidator
}

// ChangeSet stages immutable request snapshots; it never retries or rolls back mutations.
type ChangeSet struct {
	client     *Client
	mu         sync.Mutex
	operations []ChangeOperation
	committed  bool
	validating bool
}

func (c *Client) ChangeSet() *ChangeSet { return &ChangeSet{client: c} }

func (s *ChangeSet) Stage(method, uri string, body any, options ChangeOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.validating {
		return errors.New("change set cannot be modified during validation")
	}
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
		if existing := headers.Get(wire.HeaderIfMatch); existing != "" && existing != options.ETag {
			return errors.New("conflicting If-Match preconditions")
		}
		headers.Set(wire.HeaderIfMatch, options.ETag)
	}
	if options.ETag == "" {
		options.ETag = headers.Get(wire.HeaderIfMatch)
	}
	if options.IfNoneMatch != "" {
		if !validETag(options.IfNoneMatch) {
			return errors.New("invalid If-None-Match validator")
		}
		headers.Set(wire.HeaderIfNoneMatch, options.IfNoneMatch)
	} else {
		options.IfNoneMatch = headers.Get(wire.HeaderIfNoneMatch)
	}
	if options.ETag != "" && options.IfNoneMatch != "" {
		return errors.New("specify only one conditional validator")
	}

	var before json.RawMessage
	if options.Before != nil || options.BeforeProvided {
		before, err = json.Marshal(options.Before)
		if err != nil {
			return err
		}
	}
	s.operations = append(s.operations, ChangeOperation{Method: method, URI: target.RequestURI(), ETag: options.ETag, IfNoneMatch: options.IfNoneMatch, Headers: headers, Body: raw, Before: before})
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
	return s.ValidateWith(nil)
}

// ValidateWith runs caller validation against detached snapshots, without network I/O.
func (s *ChangeSet) ValidateWith(validator ChangeValidator) []ChangeValidationIssue {
	s.mu.Lock()
	if s.validating {
		s.mu.Unlock()
		return []ChangeValidationIssue{{Index: 0, Message: "change set is already being validated"}}
	}
	s.validating = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.validating = false; s.mu.Unlock() }()
	return validateChanges(s.Operations(), validator)
}

func validateChanges(operations []ChangeOperation, validator ChangeValidator) []ChangeValidationIssue {
	var issues []ChangeValidationIssue
	for i, op := range operations {
		var err error
		switch {
		case (op.ETag != "" && !validETag(op.ETag)) || (op.IfNoneMatch != "" && !validETag(op.IfNoneMatch)):
			err = errors.New("invalid ETag")
		case (op.Method == http.MethodPatch || op.Method == http.MethodPut) && len(op.Body) == 0:
			err = errors.New("PATCH and PUT changes require an explicit body")
		case len(op.Body) > 0 && !json.Valid(op.Body):
			err = errors.New("invalid JSON body")
		case op.Method == http.MethodDelete && len(op.Body) > 0:
			err = errors.New("DELETE body is not supported in a change set")
		}
		if err == nil && validator != nil {
			err = validator(op)
		}
		if err != nil {
			issues = append(issues, ChangeValidationIssue{Index: i, Message: err.Error(), Err: err})
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
		item := Object{"index": i, "method": op.Method, "uri": op.URI, "etag": op.ETag, "headers": op.Headers,
			"baseline_available": len(op.Before) > 0, "body_present": len(op.Body) > 0}
		diff, err := changeDiff(op, includeSensitive, s.client.passwordValue(), s.client.tokenSnapshot())
		if err != nil {
			return nil, err
		}
		item["diff"] = diff
		if len(op.Body) > 0 {
			item["body"] = op.Body
		}
		if len(op.Before) > 0 {
			item["before"] = op.Before
		}
		if !includeSensitive {
			safe, err := redaction.Redact(item, s.client.passwordValue(), s.client.tokenSnapshot())
			if err != nil {
				return nil, err
			}
			decoder := json.NewDecoder(bytes.NewReader(safe))
			decoder.UseNumber()
			if err = decoder.Decode(&item); err != nil {
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
	if s.validating {
		return errors.New("change set cannot be modified during validation")
	}
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
		pending[i].Headers.Set(wire.HeaderIfMatch, tag)
		if includeBefore {
			pending[i].Before = append(json.RawMessage(nil), r.Body...)
		}
	}
	s.operations = pending
	return nil
}

func responseETag(r *sdkresponse.Response) (string, error) {
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
	if s.validating {
		s.mu.Unlock()
		return nil, errors.New("change set is already being validated")
	}
	s.validating = true
	s.mu.Unlock()
	issues := validateChanges(s.Operations(), options.Validator)
	s.mu.Lock()
	defer func() { s.validating = false; s.mu.Unlock() }()
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
	if len(issues) > 0 {
		return block(issues[0].Err)
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
				if errors.Is(err, sdkerrors.ErrNotFound) && op.IfNoneMatch != "" {
					continue
				}
				return block(err)
			}
			current, err := responseETag(r)
			if err != nil {
				return block(err)
			}
			if op.IfNoneMatch != "" {
				if op.IfNoneMatch == "*" || strings.TrimPrefix(current, "W/") == strings.TrimPrefix(op.IfNoneMatch, "W/") {
					return block(sdkerrors.ErrPreconditionFailed)
				}
				if current == "" {
					return block(sdkerrors.ErrPreconditionFailed)
				}
				continue
			}
			if op.ETag != "*" && current != op.ETag {
				return block(sdkerrors.ErrPreconditionFailed)
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
		response, err := s.client.Request(ctx, op.Method, op.URI, body, RequestOptions{Headers: op.Headers, Retry: &RetryPolicy{MaxAttempts: 1, Safety: RetrySafetyDefault}})
		entry := &result.Operations[i]
		entry.Response = response
		entry.Err = err
		if err != nil {
			entry.Status = ChangeFailed
			var transport *sdkerrors.TransportError
			if errors.As(err, &transport) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				entry.Status = ChangeUncertain
			}
			entry.OutcomeUnknown = entry.Status == ChangeUncertain
			var httpError *sdkerrors.Error
			if errors.As(err, &httpError) && (httpError.StatusCode >= http.StatusInternalServerError || (httpError.StatusCode >= http.StatusMultipleChoices && httpError.StatusCode < http.StatusBadRequest)) {
				entry.OutcomeUnknown = true
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
