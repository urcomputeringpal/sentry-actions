package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/google/go-github/v32/github"
	"github.com/hashicorp/go-multierror"
	"github.com/sethvargo/go-githubactions"
	"golang.org/x/oauth2"

	"github.com/getsentry/sentry-go"
)

type config struct {
	githubToken       string
	sentryDSN         string
	sentryEnvironment string
	sentryRelease     string
	sentryDebug       bool
	event             *CompleteWorkflowRunEvent
}

type actionsService interface {
	ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64, opts *github.ListWorkflowJobsOptions) (*github.Jobs, *github.Response, error)
}
type CompleteWorkflowRunEvent struct {
	github.WorkflowRunEvent
	Action *string `json:"action,omitempty"`

	WorkflowRun *github.WorkflowRun `json:"workflow_run,omitempty"`
	Workflow    *github.Workflow    `json:"workflow,omitempty"`
}

func main() {
	c := &config{
		githubToken:       githubactions.GetInput("GITHUB_TOKEN"),
		sentryDSN:         githubactions.GetInput("SENTRY_DSN"),
		sentryEnvironment: githubactions.GetInput("SENTRY_ENVIRONMENT"),
		sentryRelease:     githubactions.GetInput("SENTRY_RELEASE"),
		sentryDebug:       githubactions.GetInput("SENTRY_DEBUG") == "true",
	}

	eventString, err := ioutil.ReadFile(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		githubactions.Fatalf("Couldn't read event: %+v", err)
	}

	jsonErr := json.Unmarshal(eventString, &c.event)
	if jsonErr != nil {
		githubactions.Fatalf("failed to validate input: %+v", err)
	}

	validateErr := c.Validate()
	if validateErr != nil {
		githubactions.Fatalf("failed to validate input: %+v", validateErr)
	}

	ctx := context.Background()
	defer sentry.RecoverWithContext(ctx)
	client := c.githubClient(ctx)

	sentryEvent, eventError := sentryEventFromWorkflowRun(ctx, c.event, client.Actions, rand.Reader)
	if eventError != nil {
		githubactions.Fatalf("failed creating event from actions run: %+v", eventError)
	}
	log.Printf("%#v", sentryEvent)

	sentrySyncTransport := sentry.NewHTTPSyncTransport()
	sentrySyncTransport.Timeout = time.Second * 30

	sentryErr := sentry.Init(sentry.ClientOptions{
		Dsn:              c.sentryDSN,
		Environment:      c.sentryEnvironment,
		Release:          c.sentryRelease,
		Debug:            c.sentryDebug,
		Transport:        sentrySyncTransport,
		AttachStacktrace: false,
	})
	if sentryErr != nil {
		githubactions.Fatalf("sentry.Init: %s", sentryErr)
	}

	id := sentry.CaptureEvent(sentryEvent)
	if id == nil {
		githubactions.Fatalf("failed reporting event %+v", sentryEvent)
	}

	if sentryEvent.Level == sentry.LevelError {
		sentryEvent.Type = ""
		sentryEvent.Transaction = ""
		sentryEvent.Spans = []*sentry.Span{}
		id := sentry.CaptureEvent(sentryEvent)
		if id == nil {
			githubactions.Fatalf("failed reporting event %+v", sentryEvent)
		}
	}
}

func (c *config) Validate() error {
	var resultErr *multierror.Error
	if c.githubToken == "" {
		resultErr = multierror.Append(resultErr, errors.New("input 'GITHUB_TOKEN' missing"))
	}
	if c.sentryDSN == "" {
		resultErr = multierror.Append(resultErr, errors.New("input 'SENTRY_DSN' missing"))
	}
	if c.sentryEnvironment == "" {
		resultErr = multierror.Append(resultErr, errors.New("input 'SENTRY_ENVIRONMENT' missing"))
	}
	if c.sentryRelease == "" {
		resultErr = multierror.Append(resultErr, errors.New("input 'SENTRY_RELEASE' missing"))
	}
	return resultErr.ErrorOrNil()
}

func (c *config) githubClient(ctx context.Context) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: c.githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}
