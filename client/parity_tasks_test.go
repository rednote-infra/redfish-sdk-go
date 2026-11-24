package client

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

func TestExpiredMonitorVerifiesTaskWithoutReplayingMutation(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			writes := 0
			sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
				}
				if r.URL.Path == "/monitor" {
					w.WriteHeader(status)
					return
				}
				fmt.Fprint(w, `{"@odata.id":"/task","TaskState":"Completed","TaskStatus":"OK"}`)
			})
			result, err := sdk.WaitForOperation(context.Background(), &models.RedfishResponse{
				HTTPStatus: models.Ptr(int64(http.StatusAccepted)), TaskMonitor: models.Ptr("/monitor"), TaskURI: models.Ptr("/task"),
			}, PollOptions{Interval: time.Millisecond, Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			if !result.MonitorMissing || result.Task == nil || models.Value(result.Task.TaskState) != TaskCompleted || writes != 0 {
				t.Fatalf("incorrect monitor recovery: %+v; writes=%d", result, writes)
			}
			if result.InitialHTTPStatus != http.StatusAccepted || result.TaskMonitorURI != "/monitor" || result.TaskURI != "/task" {
				t.Fatal("original operation metadata was lost")
			}
		})
	}
}

func TestExpiredMonitorCannotDeclareRunningTaskSuccessful(t *testing.T) {
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/monitor" {
			w.WriteHeader(http.StatusGone)
			return
		}
		fmt.Fprint(w, `{"@odata.id":"/task","TaskState":"Running"}`)
	})
	_, err := sdk.WaitForOperation(context.Background(), &models.RedfishResponse{
		HTTPStatus: models.Ptr(int64(http.StatusAccepted)), TaskMonitor: models.Ptr("/monitor"), TaskURI: models.Ptr("/task"),
	}, PollOptions{Interval: time.Millisecond, Timeout: time.Second})
	if err == nil {
		t.Fatal("a missing monitor does not prove task completion")
	}
}
