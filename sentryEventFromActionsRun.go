package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/go-github/v32/github"
)

const transactionType = "transaction"

type actionsService interface {
	GetWorkflowRunByID(ctx context.Context, owner string, repo string, runID int64) (*github.WorkflowRun, *github.Response, error)
}

func sentryEventFromActionsRun(ctx context.Context, workflowName string, owner string, repo string, runID int64, actions actionsService) (*sentry.Event, error) {
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

	description := fmt.Sprintf("%s/%s: %s (%s)", owner, repo, workflowName, run.GetEvent())

	var traceSeed string
	if run.GetCheckSuiteURL() != "" {
		traceSeed = run.GetCheckSuiteURL()
	} else {
		traceSeed = run.GetNodeID()
	}
	traceID := fmt.Sprintf("%x", generateTraceID(strings.NewReader(traceSeed)))
	spanID := fmt.Sprintf("%x", generateSpanID(strings.NewReader(traceSeed)))
	// TODO for each step
	testSpan := &sentry.Span{
		TraceID: traceID,
		SpanID:  spanID,
		// Check Suite if present
		// ParentSpanID: run.GetNodeID(),
		Description:    description,
		Op:             "actions.workflow",
		StartTimestamp: run.GetCreatedAt().Time.UTC(),
		EndTimestamp:   run.GetUpdatedAt().Time.UTC(),
		Status:         "ok",
		Tags: map[string]string{
			"actions.event":      run.GetEvent(),
			"actions.conclusion": run.GetConclusion(),
		},
		Data: map[string]interface{}{},
	}

	sentryEvent := &sentry.Event{
		Type:           transactionType,
		StartTimestamp: run.GetCreatedAt().Time.UTC(),
		Timestamp:      run.GetUpdatedAt().Time.UTC(),
		Transaction:    description,
		Spans:          []*sentry.Span{testSpan},
		Contexts: map[string]interface{}{
			"trace": sentry.TraceContext{
				TraceID:     traceID,
				SpanID:      spanID,
				Op:          run.GetEvent(),
				Description: description,
				Status:      status,
			},
		},
		Tags: map[string]string{},
	}

	return sentryEvent, nil
}
