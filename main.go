package main

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	"github.com/hashicorp/go-multierror"
	"github.com/sethvargo/go-githubactions"
	"golang.org/x/oauth2"

	"github.com/getsentry/sentry-go"
)

type config struct {
	githubRunID       int64
	githubToken       string
	sentryDSN         string
	sentryEnvironment string
	sentryRelease     string
	sentryDebug       bool
	owner             string
	repo              string
	workflowName      string
}

func main() {
	if githubactions.GetInput("GITHUB_RUN_ID") == os.Getenv("GITHUB_RUN_ID") {
		githubactions.Fatalf("%+v", errors.New("cannot report on a running action. see usage documentation"))
	}

	c := &config{
		githubToken:       githubactions.GetInput("GITHUB_TOKEN"),
		sentryDSN:         githubactions.GetInput("SENTRY_DSN"),
		sentryEnvironment: githubactions.GetInput("SENTRY_ENVIRONMENT"),
		sentryRelease:     githubactions.GetInput("SENTRY_RELEASE"),
		sentryDebug:       githubactions.GetInput("SENTRY_DEBUG") == "true",
		workflowName:      os.Getenv("GITHUB_WORKFLOW"),
	}
	githubRunID, err := strconv.ParseInt(githubactions.GetInput("GITHUB_RUN_ID"), 10, 64)
	if err != nil {
		githubactions.Fatalf("failed to validate input: %+v", err)
	}
	c.githubRunID = githubRunID

	repoOwner := strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")
	c.owner = repoOwner[0]
	c.repo = repoOwner[1]

	validateErr := c.Validate()
	if validateErr != nil {
		githubactions.Fatalf("failed to validate input: %+v", validateErr)
	}

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

	ctx := context.Background()
	defer sentry.RecoverWithContext(ctx)
	client := c.githubClient(ctx)

	sentryEvent, eventError := sentryEventFromActionsRun(ctx, c.workflowName, c.owner, c.repo, c.githubRunID, client.Actions)
	if eventError != nil {
		githubactions.Fatalf("failed creating event from actions run: %+v", eventError)
	}

	id := sentry.CaptureEvent(sentryEvent)
	if id == nil {
		githubactions.Fatalf("failed reporting event %+v", sentryEvent)
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
