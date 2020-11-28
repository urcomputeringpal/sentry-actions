package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v32/github"
)

var update = flag.Bool("update", false, "update .golden files")

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
	jobs           []*github.WorkflowJob
	err            bool
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
			name:           "example",
			actionsService: &happyPathActionsClient{},
			workflowRun: &github.WorkflowRun{
				Conclusion: github.String("success"),
				Event:      github.String("push"),
				NodeID:     github.String("id"),
				CreatedAt:  &github.Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)},
				UpdatedAt:  &github.Timestamp{time.Date(2020, time.January, 02, 15, 04, 06, 0, time.UTC)},
			},
			jobs: []*github.WorkflowJob{
				{
					ID:          github.Int64(1234),
					NodeID:      github.String("test"),
					Name:        github.String("test"),
					StartedAt:   &github.Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)},
					CompletedAt: &github.Timestamp{time.Date(2020, time.January, 02, 15, 04, 06, 0, time.UTC)},

					Steps: []*github.TaskStep{
						{
							Number:      github.Int64(1234),
							Name:        github.String("test"),
							StartedAt:   &github.Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)},
							CompletedAt: &github.Timestamp{time.Date(2020, time.January, 02, 15, 04, 06, 0, time.UTC)},
						},
					},
				},
			},
			event: &sentry.Event{},
			err:   false,
		},
	}
}

func TestTable(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.actionsService.MockWorkflowRun(tt.workflowRun)
			tt.actionsService.MockWorkflowJobs(&github.Jobs{Jobs: tt.jobs})
			event, err := sentryEventFromActionsRun(context.Background(), "workflow", "owner", "repo", 123, "actor", tt.actionsService, strings.NewReader("random"))

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
			got, err := json.MarshalIndent(event, "", "    ")
			if err != nil {
				t.Error(err)
			}

			golden := filepath.Join(".", "testdata", fmt.Sprintf("%s.event.json", tt.name))
			if *update {
				err := ioutil.WriteFile(golden, got, 0600)
				if err != nil {
					t.Fatal(err)
				}
			}

			want, err := ioutil.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("struct %s mismatch (-want +got):\n%s", tt.name, diff)
			}

		})
	}
}
