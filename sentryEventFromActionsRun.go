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
	ListWorkflowJobs(ctx context.Context, owner, repo string, runID int64, opts *github.ListWorkflowJobsOptions) (*github.Jobs, *github.Response, error)
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

	jobs, _, jobsError := actions.ListWorkflowJobs(ctx, owner, repo, runID, &github.ListWorkflowJobsOptions{
		Filter: "all",
	})
	if jobsError != nil {
		return nil, jobsError
	}

	var spans []*sentry.Span
	for _, job := range jobs.Jobs {
		jobSpanID := fmt.Sprintf("%x", generateSpanID(strings.NewReader(job.GetNodeID())))

		spans = append(spans, &sentry.Span{
			TraceID:        traceID,
			SpanID:         jobSpanID,
			ParentSpanID:   spanID,
			Description:    job.GetName(),
			Op:             "actions.job",
			StartTimestamp: job.GetStartedAt().Time.UTC(),
			EndTimestamp:   job.GetCompletedAt().Time.UTC(),
			// TODO map statuses
			Status: "ok",
		})
		for _, step := range job.Steps {
			stepSpanID := fmt.Sprintf("%x", generateSpanID(strings.NewReader(fmt.Sprintf("%s-%d", job.GetNodeID(), step.GetNumber()))))
			spans = append(spans, &sentry.Span{
				TraceID:        traceID,
				SpanID:         stepSpanID,
				ParentSpanID:   jobSpanID,
				Description:    step.GetName(),
				Op:             "actions.step",
				StartTimestamp: step.GetStartedAt().Time.UTC(),
				EndTimestamp:   step.GetCompletedAt().Time.UTC(),
				// TODO map statuses
				Status: "ok",
			})
		}
	}

	sentryEvent := &sentry.Event{
		Type:           transactionType,
		StartTimestamp: run.GetCreatedAt().Time.UTC(),
		Timestamp:      run.GetUpdatedAt().Time.UTC(),
		Transaction:    description,
		Spans:          spans,
		Contexts: map[string]interface{}{
			"trace": sentry.TraceContext{
				TraceID:     traceID,
				SpanID:      spanID,
				Op:          run.GetEvent(),
				Description: description,
				Status:      status,
			},
		},
		Tags: map[string]string{
			"workflow.event":      run.GetEvent(),
			"workflow.conclusion": run.GetConclusion(),
			"workflow.headBranch": run.GetHeadBranch(),
			"workflow.id":         fmt.Sprintf("%d", run.GetWorkflowID()),
		},
		Extra: map[string]interface{}{
			"workflow.head_sha":   run.GetHeadSHA(),
			"workflow.run_number": run.GetRunNumber(),
			"workflow.html_url":   run.GetHTMLURL(),
		},
		User: sentry.User{
			Username: run.GetCheckSuiteURL(),
		},
	}

	return sentryEvent, nil
}
