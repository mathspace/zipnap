// Package gharunner provides a configuration structure for GitHub Actions
// Runner activator.
//
// This activator wakes up the host and keeps it awake while there are GihHub
// Actions jobs with matching runner labels that need a runner.
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

// doJobsNeedRepos checks if there are any GitHub Actions jobs that need runners
// in any of the repos.
func (g *GHARunner) doJobsNeedRepos(ctx context.Context) (bool, error) {
	for _, repoFull := range g.cfg.Repos {
		owner, repo, _ := splitOwnerRepo(repoFull)
		need, err := g.doJobsNeedRepo(ctx, owner, repo)
		if err != nil {
			return false, err
		}
		if need {
			return true, nil
		}
	}

	return false, nil
}

// doJobsNeedRepo checks if there are any GitHub Actions jobs that need runners
// in the specified repository. Jobs needs runnings while they wait for them and
// while they're running.
func (g *GHARunner) doJobsNeedRepo(ctx context.Context, owner, repo string) (bool, error) {

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
				// technically need the runners while they're running.
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
					// If all labels match, we have a job that needs a runner.
					return true, nil
				}
			}
			page += 1
		}
	}

	// If we reach here, no jobs were found that needs a runner with the
	// configured labels.
	return false, nil

}

func (g *GHARunner) Run(ctx context.Context) error {

	// Do one initial check to test the connection and configuration.

	if _, err := g.doJobsNeedRepos(ctx); err != nil {
		return err
	}

	var unlock func()
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	check := func() {
		need, err := g.doJobsNeedRepos(ctx)
		if err != nil {
			g.logger.Printf("Error checking for jobs: %v", err)
			return
		}
		if !need {
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
