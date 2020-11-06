package main

import (
	"context"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/go-github/v32/github"
)

const transactionType = "transaction"

func sentryEventFromActionsRun(ctx context.Context, workflowName string, owner string, repo string, runID int64, actions *github.ActionsService) (*sentry.Event, error) {
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
	status := "ok"
	if run.GetConclusion() == "failure" {
		status = "internal_error"
	}
	if run.GetConclusion() == "cancelled" {
		status = "cancelled"
	}

	description := fmt.Sprintf("%s/%s: %s (%s)", workflowName, run.GetEvent(), owner, repo)

	var traceID string
	if run.GetCheckSuiteURL() != "" {
		traceID = run.GetCheckSuiteURL()
	} else {
		traceID = run.GetNodeID()
	}

	// TODO for each step
	testSpan := &sentry.Span{
		TraceID: traceID,
		// step node id
		SpanID: "1cc4b26ab9094ef0",
		// ParentSpanID: run.GetNodeID(),
		Description: `SELECT * FROM user WHERE "user"."id" = {id}`,
		Op:          "db.sql",
		Tags: map[string]string{
			"function_name":  "get_users",
			"status_message": "MYSQL OK",
		},
		StartTimestamp: run.GetCreatedAt().Time.UTC(),
		EndTimestamp:   run.GetUpdatedAt().Time.UTC(),
		Status:         "ok",
		Data: map[string]interface{}{
			"related_ids":  []uint{12312342, 76572, 4123485},
			"aws_instance": "ca-central-1",
		},
	}

	sentryEvent := &sentry.Event{
		Type:           transactionType,
		Spans:          []*sentry.Span{testSpan},
		StartTimestamp: run.GetCreatedAt().Time.UTC(),
		Timestamp:      run.GetUpdatedAt().Time.UTC(),
		Transaction:    description,
		Contexts: map[string]interface{}{
			"trace": sentry.TraceContext{
				TraceID:     traceID,
				SpanID:      run.GetNodeID(),
				Op:          run.GetEvent(),
				Description: description,
				Status:      status,
			},
		},
		Tags: map[string]string{
			"WorkflowRun.conclusion": run.GetConclusion(),
		},
	}

	return sentryEvent, nil
}
