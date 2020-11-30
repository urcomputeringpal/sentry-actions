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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v32/github"
)

var update = flag.Bool("update", false, "update .golden files")

type happyPathActionsClient struct {
	jobs *github.Jobs
}

type mockableActionsService interface {
	ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64, opts *github.ListWorkflowJobsOptions) (*github.Jobs, *github.Response, error)
	MockWorkflowJobs(*github.Jobs)
}

type test struct {
	name           string
	actionsService mockableActionsService
	workflowRun    *github.WorkflowRun
	jobs           []*github.WorkflowJob
	err            bool
}

var (
	tests   []test
	http200 = &github.Response{
		Response: &http.Response{StatusCode: 200},
	}
)

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
			err: false,
		},
		{
			name:           "failure",
			actionsService: &happyPathActionsClient{},
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
			err: false,
		}}
}

func TestTable(t *testing.T) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := filepath.Join(".", "testdata", fmt.Sprintf("workflow_run.%s.json", tt.name))
			fixtureBytes, err := ioutil.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}

			var workflowRunEvent CompleteWorkflowRunEvent
			jsonErr := json.Unmarshal(fixtureBytes, &workflowRunEvent)
			if jsonErr != nil {
				t.Fatal(jsonErr)
			}

			tt.actionsService.MockWorkflowJobs(&github.Jobs{Jobs: tt.jobs})
			event, err := sentryEventFromWorkflowRun(context.Background(), &workflowRunEvent, tt.actionsService, strings.NewReader("random"))

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

			golden := filepath.Join(".", "testdata", fmt.Sprintf("sentry_event.%s.json", tt.name))
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
