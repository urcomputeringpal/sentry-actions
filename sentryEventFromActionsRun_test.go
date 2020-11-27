package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/go-github/v32/github"
)

type happyPathActionsClient struct {
	workflowRun *github.WorkflowRun
	jobs        *github.Jobs
}

type mockableActionsService interface {
	GetWorkflowRunByID(ctx context.Context, owner string, repo string, runID int64) (*github.WorkflowRun, *github.Response, error)
	ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64, opts *github.ListWorkflowJobsOptions) (*github.Jobs, *github.Response, error)
	MockWorkflowRun(*github.WorkflowRun)
	MockWorkflowJobs(*github.Jobs)
}

type test struct {
	name           string
	actionsService mockableActionsService
	workflowRun    *github.WorkflowRun
	event          *sentry.Event
	err            bool
	wantTraceID    string
	wantSpanID     string
}

var (
	tests   []test
	http200 = &github.Response{
		Response: &http.Response{StatusCode: 200},
	}
)

func (c *happyPathActionsClient) GetWorkflowRunByID(ctx context.Context, owner string, repo string, runID int64) (*github.WorkflowRun, *github.Response, error) {
	return c.workflowRun, http200, nil
}
func (c *happyPathActionsClient) MockWorkflowRun(w *github.WorkflowRun) {
	c.workflowRun = w
}
func (c *happyPathActionsClient) ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64, opts *github.ListWorkflowJobsOptions) (*github.Jobs, *github.Response, error) {
	return c.jobs, http200, nil
}
func (c *happyPathActionsClient) MockWorkflowJobs(j *github.Jobs) {
	c.jobs = j
}

func init() {
	tests = []test{
		{
			name:           "sample",
			actionsService: &happyPathActionsClient{},
			workflowRun: &github.WorkflowRun{
				Conclusion: github.String("success"),
				Event:      github.String("push"),
				NodeID:     github.String("id"),
				CreatedAt:  &github.Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)},
				UpdatedAt:  &github.Timestamp{time.Date(2020, time.January, 02, 15, 04, 06, 0, time.UTC)},
			},
			event:       &sentry.Event{},
			err:         false,
			wantTraceID: "69640000000000000000000000000000",
			wantSpanID:  "6964000000000000",
		},
	}
}

func TestTable(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.actionsService.MockWorkflowRun(tt.workflowRun)
			// TODO
			tt.actionsService.MockWorkflowJobs(&github.Jobs{})
			event, err := sentryEventFromActionsRun(context.Background(), "workflow", "owner", "repo", 123, tt.actionsService)

			if tt.err && err == nil {
				t.Errorf("%s: expected an error, didn't get one", tt.name)
				return
			}

			if !tt.err && err != nil {
				t.Errorf("%s: %+v", tt.name, err)
				return
			}
			if event == nil {
				t.Errorf("%s: event is nil", tt.name)
				return
			}
			// if tt.wantTraceID != event.Spans[0].TraceID {
			// 	log.Printf("%#v", event)
			// 	t.Errorf("%s.TraceID: want %s, got %s", tt.name, tt.wantTraceID, event.Spans[0].TraceID)
			// 	return
			// }
			// if tt.wantSpanID != event.Spans[0].SpanID {
			// 	log.Printf("%#v", event)
			// 	t.Errorf("%s.SpanID: want %s, got %s", tt.name, tt.wantSpanID, event.Spans[0].SpanID)
			// 	return
			// }
		})
	}
}
