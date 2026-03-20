package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

var reviewName string

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().StringVar(&reviewName, "name", "", "alias for the worktree (default: pr-<number>)")
}

var reviewCmd = &cobra.Command{
	Use:   "review [pr-number]",
	Short: "Check out a pull request into a new worktree",
	Long: `Create a new worktree for a GitHub pull request.

Without arguments, lists open pull requests.
With a PR number, creates a worktree for that PR.

Grove will:
  - Fetch the PR branch from GitHub (via gh CLI)
  - Create a worktree for the PR
  - Copy all .env* files and create symlinks as usual

Requires the GitHub CLI (gh) to be installed and authenticated.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runReview,
}

type prInfo struct {
	HeadRefName       string `json:"headRefName"`
	Number            int    `json:"number"`
	Title             string `json:"title"`
	IsCrossRepository bool   `json:"isCrossRepository"`
}

type prListItem struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	HeadRefName string `json:"headRefName"`
}

func requireGH() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is required — install from https://cli.github.com")
	}
	return nil
}

func fetchPRInfo(number string) (prInfo, error) {
	cmd := exec.Command("gh", "pr", "view", number,
		"--json", "headRefName,number,title,isCrossRepository")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return prInfo{}, fmt.Errorf("gh pr view: %s", strings.TrimSpace(string(out)))
	}

	var info prInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return prInfo{}, fmt.Errorf("failed to parse PR info: %w", err)
	}
	return info, nil
}

func fetchPRList() ([]prListItem, error) {
	cmd := exec.Command("gh", "pr", "list",
		"--json", "number,title,author,headRefName",
		"--limit", "20")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %s", strings.TrimSpace(string(out)))
	}

	var items []prListItem
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("failed to parse PR list: %w", err)
	}
	return items, nil
}

func runReview(cmd *cobra.Command, args []string) error {
	if err := requireGH(); err != nil {
		return err
	}

	if len(args) == 0 {
		return runReviewList()
	}
	return runReviewCheckout(args[0])
}

func runReviewList() error {
	fmt.Println("Fetching open pull requests...")

	items, err := fetchPRList()
	if err != nil {
		return err
	}

	if len(items) == 0 {
		fmt.Println("No open pull requests.")
		return nil
	}

	fmt.Println(renderPRTable(items))
	fmt.Println("Run: grove review <number>")

	return nil
}

func renderPRTable(items []prListItem) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241"))
	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	titleStyle := lipgloss.NewStyle().Bold(true)
	authorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	numW := len("#")
	titleW := len("TITLE")
	authorW := len("AUTHOR")

	for _, item := range items {
		w := len(fmt.Sprintf("%d", item.Number))
		if w > numW {
			numW = w
		}
		if len(item.Title) > titleW {
			titleW = len(item.Title)
		}
		if len(item.Author.Login) > authorW {
			authorW = len(item.Author.Login)
		}
	}

	const maxTitle = 60
	if titleW > maxTitle {
		titleW = maxTitle
	}

	pad := func(s string, w int) string {
		if len(s) > w {
			return s[:w-1] + "…" + "  "
		}
		return s + strings.Repeat(" ", w-len(s)+2)
	}

	var sb strings.Builder

	sb.WriteString(
		header.Render(pad("#", numW)) +
			header.Render(pad("TITLE", titleW)) +
			header.Render(pad("AUTHOR", authorW)) +
			header.Render("BRANCH") + "\n",
	)

	for _, item := range items {
		sb.WriteString(
			numStyle.Render(pad(fmt.Sprintf("%d", item.Number), numW)) +
				titleStyle.Render(pad(item.Title, titleW)) +
				authorStyle.Render(pad(item.Author.Login, authorW)) +
				item.HeadRefName + "\n",
		)
	}

	return sb.String()
}

func runReviewCheckout(number string) error {
	if _, err := strconv.Atoi(number); err != nil {
		return fmt.Errorf("expected a PR number, got %q", number)
	}

	fmt.Printf("Fetching PR #%s...\n", number)

	pr, err := fetchPRInfo(number)
	if err != nil {
		return err
	}
	fmt.Printf("  #%d: %s\n", pr.Number, pr.Title)

	from := ""
	if pr.IsCrossRepository {
		refspec := fmt.Sprintf("pull/%d/head:%s", pr.Number, pr.HeadRefName)
		if err := git.Fetch("origin", refspec); err != nil {
			return fmt.Errorf("fetching fork PR: %w", err)
		}
	} else {
		if err := git.Fetch("origin", pr.HeadRefName); err != nil {
			return fmt.Errorf("fetching branch: %w", err)
		}
		from = "origin/" + pr.HeadRefName
	}
	fmt.Println("  ✓ branch fetched")

	alias := reviewName
	if alias == "" {
		alias = fmt.Sprintf("pr-%s", number)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	s, err := state.Load(root)
	if err != nil {
		return err
	}

	if err := doCreate(root, cfg, &s, pr.HeadRefName, alias, from); err != nil {
		return err
	}

	if pr.IsCrossRepository {
		fmt.Println("  Note: fork PR — push requires adding the fork as a remote")
	}

	return nil
}
