package operations

import (
	"log"
	"time"

	"poule/configuration"
	"poule/gh"
	"poule/utils"

	"github.com/google/go-github/github"
)

type Context struct {
	// Client is the GitHub API client instance.
	Client gh.Client

	// Username is the owner of the GitHub repository.
	Username string

	// Repository is the name of the GitHub repository.
	Repository string
}

// FilterResult describes the result of an operation filter.
type FilterResult int

const (
	// Accept means that the filter accepts the item.
	Accept FilterResult = iota

	// Reject means that the filter rejects the item
	Reject

	// Terminal means that the filter is rejected, and that no more items
	// should be sumbmitted to that filter. This is typically useful for
	// operations working on sorted sets of data, and for which the first
	// failure could also mean that no Accept may further occur.
	Terminal
)

// Configuration is an opaque data structure for operation-specific
// configuration.
type Configuration map[string]interface{}

// AcceptedType describes the combination of GitHub item types accepted by an
// operation.
type AcceptedType int

const (
	// Issues means that the operation can apply to issues.
	Issues AcceptedType = 1 << iota

	// PullRequests means that the operation can apply to pull requests.
	PullRequests

	// All means that the operation can apply to both issues and pull requests.
	All = Issues | PullRequests
)

// Operation is the central interface: it encapsulates an action over GitHub
// items (either issues, pull requests, or both).
type Operation interface {
	// Accepts returns the combination of GitHub item types that the operation
	// can be applied to.
	Accepts() AcceptedType

	// Apply applies the operation to the GitHub item.
	Apply(*Context, gh.Item, interface{}) error

	// Describe returns a human-readable description of calling Apply on the
	// specified item with the specified context.
	Describe(*Context, gh.Item, interface{}) string

	// Filter returns whether that operation should apply to the specified
	// item, and an operation specific user data that is guaranteed to be
	// passed on Apply and Describe invocation.
	Filter(*Context, gh.Item) (FilterResult, interface{})

	// IssueListOptions returns the global filtering options to apply when
	// listing issues for the specified context.
	IssueListOptions(*Context) *github.IssueListByRepoOptions

	// PullRequestListOptions returns the global filtering options to apply
	// when listing pull requests for the specified context.
	PullRequestListOptions(*Context) *github.PullRequestListOptions
}

func RunOnIssues(c *configuration.Config, op Operation) {
	context := Context{}
	context.Client = gh.MakeClient(c)
	context.Username, context.Repository = gh.GetRepository(c.Repository)

	for page := 1; page != 0; {
		// Retrieve the list options from the operation, and override the page
		// number with the current pointer.
		listOptions := op.IssueListOptions(&context)
		listOptions.ListOptions.Page = page

		// List all issues for that repository with the specific settings.
		issues, resp, err := context.Client.Issues().ListByRepo(context.Username, context.Repository, listOptions)
		if err != nil {
			log.Fatal(err)
		}

		// Handle each issue, filtering them using the operation first.
		for _, issue := range issues {
			item := gh.MakeItem(&issue)
			switch filterResult, userdata := op.Filter(&context, item); filterResult {
			case Accept:
				if s := op.Describe(&context, item, userdata); s != "" {
					log.Printf(s)
				}
				if !utils.IsDryRun(c) {
					if err := op.Apply(&context, item, userdata); err != nil {
						log.Printf("Error applying operation on issue %d: %v", *issue.Number, err)
					}
				}
				break
			case Terminal:
				return
			}
		}

		// Move on to the next page, and respect the specified delay to avoid
		// hammering the GitHub API.
		page = resp.NextPage
		if c.Delay > 0 {
			time.Sleep(c.Delay)
		}
	}
}

func RunOnPullRequests(c *configuration.Config, op Operation) {
	context := Context{}
	context.Client = gh.MakeClient(c)
	context.Username, context.Repository = gh.GetRepository(c.Repository)

	for page := 1; page != 0; {
		// Retrieve the list options from the operation, and override the page
		// number with the current pointer.
		listOptions := op.PullRequestListOptions(&context)
		listOptions.ListOptions.Page = page

		// List all issues for that repository with the specific settings.
		prs, resp, err := context.Client.PullRequests().List(context.Username, context.Repository, listOptions)
		if err != nil {
			log.Fatal(err)
		}

		// Handle each issue, filtering them using the operation first.
		for _, pr := range prs {
			item := gh.MakeItem(&pr)
			switch filterResult, userdata := op.Filter(&context, item); filterResult {
			case Accept:
				if s := op.Describe(&context, item, userdata); s != "" {
					log.Printf(s)
				}

				if !utils.IsDryRun(c) {
					if err := op.Apply(&context, item, userdata); err != nil {
						log.Printf("Error applying operation on pull request %d: %v", *pr.Number, err)
					}
				}
				break
			case Terminal:
				return
			}
		}

		// Move on to the next page, and respect the specified delay to avoid
		// hammering the GitHub API.
		page = resp.NextPage
		if c.Delay > 0 {
			time.Sleep(c.Delay)
		}
	}
}
