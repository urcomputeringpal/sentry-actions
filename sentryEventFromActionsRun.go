package main

import (
	"context"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/go-github/v32/github"
)

func sentryEventFromActionsRun(ctx context.Context, workflowName string, owner string, repo string, runID int64, actions *github.ActionsService) (*sentry.Event, error) {
	sentryEvent := sentry.NewEvent()
	sentryEvent.Level = "info"
	sentryEvent.Type = "transaction"

	// wait for conclusion
	conclusion := ""
	var run *github.WorkflowRun
	var runError error
	for conclusion == "" {
		run, _, runError = actions.GetWorkflowRunByID(ctx, owner, repo, runID)
		if runError != nil {
			return nil, runError
		}
		conclusion = run.GetConclusion()
		if conclusion == "" {
			time.Sleep(10000 * time.Millisecond)
		}
	}

	// decorate event with metadata
	if run.GetConclusion() == "failure" {
		sentryEvent.Level = "error"
	}
	sentryEvent.Tags["WorkflowRun.conclusion"] = run.GetConclusion()
	sentryEvent.StartTimestamp = run.GetCreatedAt().Time
	sentryEvent.StartTimestamp = run.GetUpdatedAt().Time
	sentryEvent.Message = fmt.Sprintf("%s/%s: %s (%s)", workflowName, run.GetEvent(), owner, repo)

	return sentryEvent, nil
}
