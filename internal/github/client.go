package github

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v62/github"
)

// Client wraps the GitHub API client
type Client struct {
	client *github.Client
	token  string
}

// PRResult contains the result of creating a PR
type PRResult struct {
	PRURL    string
	PRNumber int
	Branch   string
}

// New creates a new GitHub client
func New() (*Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}

	client := github.NewClient(nil).WithAuthToken(token)
	return &Client{client: client, token: token}, nil
}

// NewWithToken creates a client with a specific token
func NewWithToken(token string) *Client {
	client := github.NewClient(nil).WithAuthToken(token)
	return &Client{client: client, token: token}
}

// CreateRunnerRightSizePR creates a PR to change the runner label in a workflow file
func (c *Client) CreateRunnerRightSizePR(ctx context.Context, opts PROptions) (*PRResult, error) {
	// Parse owner/repo
	parts := strings.Split(opts.Repository, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository format: %s (expected owner/repo)", opts.Repository)
	}
	owner, repo := parts[0], parts[1]

	// Get the default branch
	repoInfo, _, err := c.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	defaultBranch := repoInfo.GetDefaultBranch()

	// Get the workflow file content
	workflowPath := opts.WorkflowFile
	if workflowPath == "" {
		workflowPath = ".github/workflows/" + opts.JobID + ".yml"
	}

	fileContent, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, workflowPath, &github.RepositoryContentGetOptions{
		Ref: defaultBranch,
	})
	if err != nil {
		// Try common workflow file names first
		commonPatterns := []string{
			".github/workflows/ci.yml",
			".github/workflows/ci-hosted.yml",
			".github/workflows/main.yml",
			".github/workflows/build.yml",
			".github/workflows/test.yml",
			".github/workflows/pipeline.yml",
			".github/workflows/release.yml",
		}
		for _, p := range commonPatterns {
			if p == workflowPath {
				continue // already tried
			}
			fileContent, _, _, err = c.client.Repositories.GetContents(ctx, owner, repo, p, &github.RepositoryContentGetOptions{
				Ref: defaultBranch,
			})
			if err == nil {
				workflowPath = p
				break
			}
		}
	}
	if err != nil {
		// Last resort: list all files under .github/workflows/ and pick the
		// first .yml that references the old runner label.
		_, dirContents, _, listErr := c.client.Repositories.GetContents(ctx, owner, repo, ".github/workflows", &github.RepositoryContentGetOptions{
			Ref: defaultBranch,
		})
		if listErr == nil {
			for _, f := range dirContents {
				if f.GetType() != "file" {
					continue
				}
				fc, _, _, ferr := c.client.Repositories.GetContents(ctx, owner, repo, f.GetPath(), &github.RepositoryContentGetOptions{
					Ref: defaultBranch,
				})
				if ferr != nil {
					continue
				}
				body, _ := fc.GetContent()
				if strings.Contains(body, opts.CurrentLabel) || strings.Contains(body, opts.JobID) {
					fileContent = fc
					workflowPath = f.GetPath()
					err = nil
					break
				}
			}
		}
	}
	if err != nil || fileContent == nil {
		return nil, fmt.Errorf("could not find a workflow file for job '%s' in %s", opts.JobID, opts.Repository)
	}

	// Decode the file content
	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("failed to decode workflow content: %w", err)
	}

	// Replace the runner label
	newContent := replaceRunnerLabel(content, opts.CurrentLabel, opts.NewLabel, opts.JobID)
	if newContent == content {
		// CurrentLabel not found literally — try a case-insensitive scan and
		// replace any runs-on line that differs from the new label, so we
		// always produce a meaningful diff when the detected machine name
		// doesn't exactly match the YAML value (e.g. catalog ID vs label).
		newContent = replaceAnyRunnerLabel(content, opts.NewLabel)
		if newContent == content {
			return nil, fmt.Errorf("no 'runs-on:' line found in workflow file %s", workflowPath)
		}
	}

	// Create a new branch
	branchName := fmt.Sprintf("runright/rightsize-%s-%s", sanitizeBranchName(opts.JobID), time.Now().Format("20060102-150405"))

	// Get the SHA of the default branch
	ref, _, err := c.client.Git.GetRef(ctx, owner, repo, "refs/heads/"+defaultBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get default branch ref: %w", err)
	}

	// Create the new branch
	newRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branchName),
		Object: &github.GitObject{SHA: ref.Object.SHA},
	}
	_, _, err = c.client.Git.CreateRef(ctx, owner, repo, newRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}

	// Update the file on the new branch
	commitMsg := fmt.Sprintf("ci: right-size %s runner from %s to %s\n\nRecommended by RunRight.\nEstimated savings: $%.2f/month (%.0f%% reduction)",
		opts.JobID, opts.CurrentLabel, opts.NewLabel, opts.MonthlySavings, opts.SavingsPercent)

	_, _, err = c.client.Repositories.UpdateFile(ctx, owner, repo, workflowPath, &github.RepositoryContentFileOptions{
		Message: github.String(commitMsg),
		Content: []byte(newContent),
		Branch:  github.String(branchName),
		SHA:     fileContent.SHA,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update workflow file: %w", err)
	}

	// Create the PR
	prTitle := fmt.Sprintf("ci: right-size %s runner (%.0f%% cost reduction)", opts.JobID, opts.SavingsPercent)
	prBody := generatePRBody(opts)

	pr, _, err := c.client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.String(prTitle),
		Body:  github.String(prBody),
		Head:  github.String(branchName),
		Base:  github.String(defaultBranch),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	return &PRResult{
		PRURL:    pr.GetHTMLURL(),
		PRNumber: pr.GetNumber(),
		Branch:   branchName,
	}, nil
}

// PROptions contains the options for creating a right-size PR
type PROptions struct {
	Repository        string
	JobID             string
	WorkflowFile      string
	CurrentLabel      string
	NewLabel          string
	CurrentVCPUs      int
	CurrentMemoryGiB  float64
	NewVCPUs          int
	NewMemoryGiB      float64
	P95CPU            float64
	P95Memory         float64
	RunCount          int
	MonthlySavings    float64
	SavingsPercent    float64
	ConsecutiveRuns   int
}

func replaceRunnerLabel(content, oldLabel, newLabel, jobID string) string {
	// Try to find and replace the specific job's runs-on
	// Pattern: job-name:\n  runs-on: old-label
	jobPattern := regexp.MustCompile(`(?m)(` + regexp.QuoteMeta(jobID) + `:\s*\n(?:.*\n)*?\s*runs-on:\s*)` + regexp.QuoteMeta(oldLabel))
	if jobPattern.MatchString(content) {
		return jobPattern.ReplaceAllString(content, "${1}"+newLabel)
	}

	// Fallback: replace any runs-on with the old label
	pattern := regexp.MustCompile(`(?m)(runs-on:\s*)` + regexp.QuoteMeta(oldLabel))
	return pattern.ReplaceAllString(content, "${1}"+newLabel)
}

// replaceAnyRunnerLabel replaces ALL runs-on: <anything> lines with newLabel.
// Used as a last resort when the stored label name doesn't match the YAML
// value exactly (e.g. catalog machine ID vs actual GitHub runner label).
func replaceAnyRunnerLabel(content, newLabel string) string {
	pattern := regexp.MustCompile(`(?m)(runs-on:\s*)\S+`)
	return pattern.ReplaceAllString(content, "${1}"+newLabel)
}

func sanitizeBranchName(s string) string {
	// Replace non-alphanumeric characters with dashes
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	return strings.Trim(re.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func generatePRBody(opts PROptions) string {
	return fmt.Sprintf(`## 🎯 Right-Size Runner Recommendation

**RunRight** detected that job **%s** is over-provisioned and recommends downsizing.

### 📊 Analysis Summary

| Metric | Value |
|--------|-------|
| **Job** | %s |
| **Repository** | %s |
| **Runs Analyzed** | %d |
| **Consecutive Under-utilized** | %d |

### 💻 Current Runner
- **Label:** `+"`%s`"+`
- **vCPUs:** %d
- **Memory:** %.0f GiB

### ✅ Recommended Runner
- **Label:** `+"`%s`"+`
- **vCPUs:** %d
- **Memory:** %.0f GiB

### 📈 Resource Usage (p95)
- **CPU:** %.1f%% of available
- **Memory:** %.1f%% of available

### 💰 Estimated Savings
- **Monthly:** $%.2f
- **Reduction:** %.0f%%

---

*Generated by [RunRight](https://runright.io) - Right-size your CI runners*
`,
		opts.JobID,
		opts.JobID,
		opts.Repository,
		opts.RunCount,
		opts.ConsecutiveRuns,
		opts.CurrentLabel,
		opts.CurrentVCPUs,
		opts.CurrentMemoryGiB,
		opts.NewLabel,
		opts.NewVCPUs,
		opts.NewMemoryGiB,
		opts.P95CPU,
		opts.P95Memory,
		opts.MonthlySavings,
		opts.SavingsPercent,
	)
}

// ValidateToken checks if the token has the required permissions
func (c *Client) ValidateToken(ctx context.Context) error {
	_, resp, err := c.client.Users.Get(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to validate token: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token validation failed with status: %d", resp.StatusCode)
	}
	return nil
}
