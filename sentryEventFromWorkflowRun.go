package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/google/go-github/v32/github"
)

const transactionType = "transaction"

func sentryEventFromWorkflowRun(ctx context.Context, event *CompleteWorkflowRunEvent, actions actionsService, randReader io.Reader) (*sentry.Event, error) {
	owner := event.GetRepo().GetOwner().GetLogin()
	repo := event.GetRepo().GetName()
	workflowName := event.Workflow.GetName()
	run := event.WorkflowRun
	runID := run.GetID()

	description := fmt.Sprintf("%s (%s)", workflowName, run.GetEvent())

	jobs, _, jobsError := actions.ListWorkflowJobs(ctx, owner, repo, runID, &github.ListWorkflowJobsOptions{
		Filter: "all",
	})
	if jobsError != nil {
		return nil, jobsError
	}

	// The intent of this region is to join multiple actions workflows launched in response to the same event together
	// TODO test
	var traceID string
	if run.GetCheckSuiteURL() != "" {
		traceID = generateTraceID(strings.NewReader(run.GetCheckSuiteURL()))
	} else {
		traceID = generateTraceID(strings.NewReader(run.GetNodeID()))
	}

	spanID := generateSpanID(randReader)

	runJson, jsonErr := json.MarshalIndent(run, "", "    ")
	if jsonErr != nil {
		return nil, jsonErr
	}

	sentryEvent := &sentry.Event{
		Type:           transactionType,
		StartTimestamp: run.GetCreatedAt().Time.UTC(),
		Timestamp:      run.GetUpdatedAt().Time.UTC(),
		Transaction:    description,
		Contexts: map[string]interface{}{
			"trace": sentry.TraceContext{
				TraceID:     traceID,
				SpanID:      spanID,
				Op:          "actions.workflow_run",
				Description: description,
				Status:      spanStatusFromConclusion(run.GetConclusion()),
			},
		},
		Tags: map[string]string{
			"workflow.event":      run.GetEvent(),
			"workflow.conclusion": run.GetConclusion(),
			"workflow.headBranch": run.GetHeadBranch(),
			"workflow.id":         fmt.Sprintf("%d", run.GetWorkflowID()),
			"workflow.name":       workflowName,
			"github.owner":        owner,
			"github.repository":   fmt.Sprintf("%s/%s", owner, repo),
		},
		Extra: map[string]interface{}{
			"workflow.run": string(runJson),
		},
		User: sentry.User{
			Username: event.GetSender().GetLogin(),
		},
		// TODO
		// Modules
	}

	// TODO extract
	if run.GetConclusion() == "failure" {
		sentryEvent.Exception = append(sentryEvent.Exception, sentry.Exception{
			Value:      fmt.Sprintf("%s failed", description),
			Type:       "error",
			Stacktrace: &sentry.Stacktrace{Frames: []sentry.Frame{}},
		})
		sentryEvent.Level = sentry.LevelError
	}

	for _, job := range jobs.Jobs {
		jobSpanID := generateSpanID(strings.NewReader(job.GetNodeID()))

		jobJson, jsonErr := json.MarshalIndent(job, "", "    ")
		if jsonErr != nil {
			return nil, jsonErr
		}

		sentryEvent.Spans = append(sentryEvent.Spans, &sentry.Span{
			TraceID:        traceID,
			SpanID:         jobSpanID,
			ParentSpanID:   spanID,
			Description:    job.GetName(),
			Op:             "actions.job",
			StartTimestamp: job.GetStartedAt().Time.UTC(),
			EndTimestamp:   job.GetCompletedAt().Time.UTC(),
			Status:         spanStatusFromConclusion(job.GetConclusion()),
			Data: map[string]interface{}{
				"job": string(jobJson),
			},
		})
		for _, step := range job.Steps {
			stepSpanID := generateSpanID(strings.NewReader(fmt.Sprintf("%s-%d", job.GetNodeID(), step.GetNumber())))

			stepJson, jsonErr := json.MarshalIndent(step, "", "    ")
			if jsonErr != nil {
				return nil, jsonErr
			}

			sentryEvent.Spans = append(sentryEvent.Spans, &sentry.Span{
				TraceID:        traceID,
				SpanID:         stepSpanID,
				ParentSpanID:   jobSpanID,
				Description:    step.GetName(),
				Op:             "actions.step",
				StartTimestamp: step.GetStartedAt().Time.UTC(),
				EndTimestamp:   step.GetCompletedAt().Time.UTC(),
				Status:         spanStatusFromConclusion(step.GetConclusion()),
				Data: map[string]interface{}{
					"job": string(stepJson),
				},
			})
		}
	}

	return sentryEvent, nil
}

func spanStatusFromConclusion(conclusion string) string {
	switch conclusion {
	// TODO const
	case "cancelled":
		return "cancelled"
	case "failure":
		return "internal_error"
	case "timed_out":
		return "deadline_exceeded"
	default:
		return "ok"
	}
}
