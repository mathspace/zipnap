// Package gharunner provides a configuration structure for GitHub Actions
// Runner activator.
//
// This activator wakes up the host and keeps it awake while there are GihHub
// Actions jobs with matching runner labels that are awaiting their runners.
package gharunner

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/google/go-github/v74/github"
	"github.com/mathspace/zipnap/activator"
	"golang.org/x/oauth2"
)

type GHARunner struct {
	cfg    *Config
	labels map[string]struct{}
	logger *log.Logger
	cb     activator.Callbacks
	client *github.Client
}

func New(cfg *Config, logger *log.Logger) (*GHARunner, error) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv(cfg.TokenEnvVar)},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)

	return &GHARunner{
		cfg:    cfg,
		labels: make(map[string]struct{}),
		logger: logger,
		client: client,
	}, nil
}

func (g *GHARunner) RegisterCallbacks(cb activator.Callbacks) {
	g.cb = cb
}

// areJobsWaiting checks if there are any GitHub Actions jobs waiting for a
// runner with the configured labels across all repositories.
func (g *GHARunner) areJobsWaiting(ctx context.Context) (bool, error) {
	for _, repoFull := range g.cfg.Repos {
		owner, repo, _ := splitOwnerRepo(repoFull)
		waiting, err := g.areJobsWaitingForRepo(ctx, owner, repo)
		if err != nil {
			return false, err
		}
		if waiting {
			return true, nil
		}
	}

	return false, nil
}

// areJobsWaitingForRepo checks if there are any GitHub Actions jobs waiting for
// a runner in the specified repository.
func (g *GHARunner) areJobsWaitingForRepo(ctx context.Context, owner, repo string) (bool, error) {

	// Get list of all workflows that are in progress.

	allWorkflowRuns := []*github.WorkflowRun{}
	page := 1
	for {
		workflows, _, err := g.client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &github.ListWorkflowRunsOptions{
			Status: "in_progress",
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: 100,
			},
		})
		if err != nil {
			return false, err
		}
		if len(workflows.WorkflowRuns) == 0 {
			break
		}
		allWorkflowRuns = append(allWorkflowRuns, workflows.WorkflowRuns...)
		page += 1
	}

	// For each workflow, check if it has any jobs that are waiting for a runner
	// (status of queued) and that runner has all the labels configured in the
	// activator.

	for _, workflowRun := range allWorkflowRuns {
		page := 1
		for {
			wfJobs, _, err := g.client.Actions.ListWorkflowJobs(ctx, owner, repo, workflowRun.GetID(), &github.ListWorkflowJobsOptions{
				ListOptions: github.ListOptions{
					Page:    page,
					PerPage: 100,
				},
			})
			if err != nil {
				return false, err
			}
			if len(wfJobs.Jobs) == 0 {
				break
			}
			for _, job := range wfJobs.Jobs {
				// We need to check for in_progress as well since they're
				// technically still using a runner and thus "waiting" for it to
				// continue to exist.
				if job.GetStatus() != "queued" && job.GetStatus() != "in_progress" {
					continue
				}
				matchCount := 0
				// Check if the job has any labels that match the configured runner labels.
				for _, label := range job.Labels {
					if _, ok := g.labels[label]; ok {
						matchCount++
					}
				}
				if matchCount == len(g.labels) {
					// If all labels match, we have a job waiting for a runner.
					return true, nil
				}
			}
			page += 1
		}
	}

	// If we reach here, no jobs were found that are waiting for a runner with
	// the configured labels.
	return false, nil

}

func (g *GHARunner) Run(ctx context.Context) error {

	// Do one initial check to test the connection and configuration.

	if _, err := g.areJobsWaiting(ctx); err != nil {
		return err
	}

	var unlock func()
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	check := func() {
		waiting, err := g.areJobsWaiting(ctx)
		if err != nil {
			g.logger.Printf("Error checking for waiting jobs: %v", err)
			return
		}
		if !waiting {
			if unlock != nil {
				unlock()
				unlock = nil
			}
			return
		}
		if unlock == nil {
			unlock, err = g.cb.WakeLock(ctx, true)
			if err != nil {
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					g.logger.Printf("Error acquiring wake lock: %v", err)
				}
			}
		}
	}

	for {
		check()

		t := time.NewTimer(3 * time.Second)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
