package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestChangeSetRequiresMutationBodiesAndMarksServerErrorsUncertain(t *testing.T) {
	writes := 0
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { writes++; w.WriteHeader(http.StatusInternalServerError) })
	empty := sdk.ChangeSet()
	if err := empty.Stage(http.MethodPatch, "/resource", nil, ChangeOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(empty.Validate()) == 0 {
		t.Fatal("PATCH needs an explicit body")
	}
	plan := sdk.ChangeSet()
	if err := plan.Stage(http.MethodPatch, "/resource", Object{"Enabled": true}, ChangeOptions{}); err != nil {
		t.Fatal(err)
	}
	result, err := plan.Commit(context.Background(), CommitOptions{})
	if err == nil || !result.HasUncertainOutcomes() || !result.Operations[0].OutcomeUnknown || writes != 1 {
		t.Fatalf("server error outcome must remain uncertain: %+v, %v", result, err)
	}
}

func TestChangeSetDiffAndCustomValidation(t *testing.T) {
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("local planning must not send requests") })
	before := Object{"Enabled": false, "Password": "old-sample", "Keep": 1}
	plan := sdk.ChangeSet()
	if err := plan.Stage(http.MethodPatch, "/resource", Object{"Enabled": true, "Password": "new-sample"}, ChangeOptions{Before: before}); err != nil {
		t.Fatal(err)
	}
	before["Enabled"] = true
	preview, err := plan.Plan(false)
	if err != nil {
		t.Fatal(err)
	}
	if preview[0]["baseline_available"] != true || preview[0]["body_present"] != true {
		t.Fatal("presence context missing")
	}
	diff := preview[0]["diff"].([]any)
	if len(diff) != 2 {
		t.Fatalf("PATCH diff must retain unchanged fields: %#v", diff)
	}
	data, _ := plan.ToJSON(false)
	if containsAnySecret(string(data), "old-sample", "new-sample") {
		t.Fatal("diff leaked a password")
	}
	rejected := errors.New("local validation rejected the change")
	validator := func(op ChangeOperation) error {
		if err := plan.Stage(http.MethodDelete, "/other", nil, ChangeOptions{}); err == nil {
			t.Error("validator changed the plan")
		}
		return rejected
	}
	issues := plan.ValidateWith(validator)
	if len(issues) != 1 || !errors.Is(issues[0].Err, rejected) {
		t.Fatal("custom validation error must be retained")
	}
	_, err = plan.Commit(context.Background(), CommitOptions{Validator: validator})
	if !errors.Is(err, rejected) {
		t.Fatalf("custom validation error changed: %v", err)
	}
}

func containsAnySecret(value string, secrets ...string) bool {
	for _, secret := range secrets {
		if strings.Contains(value, secret) {
			return true
		}
	}
	return false
}

func TestChangeSetNeverInheritsMutationReplayPolicy(t *testing.T) {
	writes := 0
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { writes++; w.WriteHeader(http.StatusServiceUnavailable) })
	sdk.cfg.retry = RetryPolicy{MaxAttempts: 2, Safety: RetrySafetyIdempotent}
	plan := sdk.ChangeSet()
	if err := plan.Stage(http.MethodPatch, "/resource", Object{"Enabled": true}, ChangeOptions{}); err != nil {
		t.Fatal(err)
	}
	_, _ = plan.Commit(context.Background(), CommitOptions{})
	if writes != 1 {
		t.Fatalf("a change-set mutation was replayed %d times", writes)
	}
}

func TestChangeSetPreviewRetainsExactIntegerValues(t *testing.T) {
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("preview must remain offline") })
	plan := sdk.ChangeSet()
	if err := plan.Stage(http.MethodPatch, "/resource", Object{"Counter": int64(9007199254740993)}, ChangeOptions{Before: Object{"Counter": int64(9007199254740992)}}); err != nil {
		t.Fatal(err)
	}
	data, err := plan.ToJSON(false)
	if err != nil || !strings.Contains(string(data), "9007199254740993") {
		t.Fatalf("preview changed an integer value: %s %v", data, err)
	}
}
