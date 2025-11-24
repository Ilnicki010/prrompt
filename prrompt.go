package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const toolName = "prrompt"

const (
	defaultCommitPrefix = "prompt"
	defaultBranchPrefix = "skill-update"
	defaultBaseBranch   = "main"
)

var defaultPromptPatterns = []string{
	".claude/skills/",
	"prompts/",
}

func getCommitPrefix() string {
	value, err := runGit("config", "--get", "prrompt.commitPrefix")
	if err != nil {
		return defaultCommitPrefix
	}
	if value == "" {
		return defaultCommitPrefix
	}
	return value
}

func getBranchPrefix() string {
	value, err := runGit("config", "--get", "prrompt.branchPrefix")
	if err != nil {
		return defaultBranchPrefix
	}
	if value == "" {
		return defaultBranchPrefix
	}
	return value
}

func getBaseBranch() string {
	value, err := runGit("config", "--get", "prrompt.baseBranch")
	if err != nil {
		return defaultBaseBranch
	}
	if value == "" {
		return defaultBaseBranch
	}
	return value
}

func getPromptPatterns() []string {
	value, err := runGit("config", "--get", "prrompt.promptPatterns")
	if err != nil || value == "" {
		return defaultPromptPatterns
	}
	// Split by comma and trim whitespace
	patterns := strings.Split(value, ",")
	result := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" {
			result = append(result, pattern)
		}
	}
	if len(result) == 0 {
		return defaultPromptPatterns
	}
	return result
}

type CommitInfo struct {
	SHA         string
	Message     string
	PromptFiles  []string
	OtherFiles  []string
	IsMixed     bool
	SourceBranch string
}

func processCommit(commitSHA string) error {
	// Check if we're on a prompt branch - if so, skip to avoid recursion
	currentBranch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err == nil && strings.HasPrefix(currentBranch, getBranchPrefix()+"/") {
		// We're on a prompt branch, don't process
		return nil
	}
	
	commitInfo, err := analyzeCommit(commitSHA)
	if err != nil {
		return fmt.Errorf("error analyzing commit: %w", err)
	}

	if len(commitInfo.PromptFiles) == 0 {
		return nil
	}

	if err := extractPrompts(commitInfo); err != nil {
		return fmt.Errorf("error extracting prompts: %w", err)
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <commit-sha>\n", toolName)
		fmt.Println("This tool is meant to be run as a git post-commit hook")
		os.Exit(1)
	}

	commitSHA := os.Args[1]
	
	if err := processCommit(commitSHA); err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func isPromptFile(path string) bool {
	for _, pattern := range getPromptPatterns() {
		if strings.HasPrefix(path, pattern) {
			return true
		}
	}
	return false
}

func analyzeCommit(sha string) (*CommitInfo, error) {
	info := &CommitInfo{SHA: sha}

	commitMessage, err := runGit("log", "--format=%B", "-n", "1", sha)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit message: %w", err)
	}
	info.Message = commitMessage

	currentBranch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	info.SourceBranch = currentBranch

	changedFiles, err := runGit("diff-tree", "--no-commit-id", "--name-only", "-r", sha)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	for _, file := range strings.Split(changedFiles, "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if isPromptFile(file) {
			info.PromptFiles = append(info.PromptFiles, file)
		} else {
			info.OtherFiles = append(info.OtherFiles, file)
		}
	}

	info.IsMixed = len(info.OtherFiles) > 0

	return info, nil
}

func extractPrompts(info *CommitInfo) error {
	shortSHA := info.SHA[:7]
	promptBranch := fmt.Sprintf("%s/%s", getBranchPrefix(), shortSHA)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("Processing commit: %s\n", shortSHA)
	fmt.Printf("Message: %s\n", truncate(info.Message, 60))
	fmt.Printf("Prompt files: %d\n", len(info.PromptFiles))
	fmt.Printf("Other files: %d\n", len(info.OtherFiles))
	fmt.Println(strings.Repeat("=", 60))

	// Create and checkout new branch from base
	fmt.Printf("\nCreating branch: %s\n", promptBranch)
	if _, err := runGit("checkout", "-b", promptBranch, getBaseBranch()); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Cherry-pick without committing
	fmt.Printf("Cherry-picking commit %s...\n", shortSHA)
	if _, err := runGit("cherry-pick", info.SHA, "--no-commit"); err != nil {
		cleanup(info.SourceBranch, promptBranch)
		return fmt.Errorf("failed to cherry-pick: %w", err)
	}

	// For mixed commits, unstage non-prompt files (but keep them in working directory)
	if info.IsMixed {
		for _, file := range info.OtherFiles {
			runGit("restore", "--staged", file)
		}
	}

	// Create commit message
	commitMsg := fmt.Sprintf("[%s] %s", getCommitPrefix(), info.Message)
	if info.IsMixed {
		commitMsg += fmt.Sprintf("\n\nExtracted from %s (%s)", info.SourceBranch, shortSHA)
	}

	// Commit
	if _, err := runGit("commit", "-m", commitMsg); err != nil {
		cleanup(info.SourceBranch, promptBranch)
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("✓ Created skill branch: %s\n", promptBranch)

	// Push to remote
	if _, err := runGit("push", "origin", promptBranch, "-u"); err != nil {
		fmt.Printf("Warning: failed to push (you may need to push manually): %v\n", err)
	} else {
		fmt.Printf("✓ Pushed to origin/%s\n", promptBranch)
	}

	// Return to original branch (force to handle any uncommitted changes)
	if _, err := runGit("checkout", "-f", info.SourceBranch); err != nil {
		return fmt.Errorf("failed to return to original branch: %w", err)
	}

	// Generate PR URL
	prURL := generatePRURL(promptBranch)
	fmt.Printf("\n✓ Skill extraction complete!\n")
	fmt.Printf("\nCreate PR: %s\n\n", prURL)

	return nil
}

func cleanup(originalBranch, skillBranch string) {
	runGit("cherry-pick", "--abort")
	runGit("checkout", originalBranch)
	runGit("branch", "-D", skillBranch)
}

func generatePRURL(branch string) string {
	// Get remote URL
	remoteURL, err := runGit("config", "--get", "remote.origin.url")
	if err != nil {
		return fmt.Sprintf("Create PR manually for branch: %s", branch)
	}

	// Parse GitHub repo
	var repoPath string
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		// SSH: git@github.com:user/repo.git
		repoPath = strings.TrimPrefix(remoteURL, "git@github.com:")
		repoPath = strings.TrimSuffix(repoPath, ".git")
	} else if strings.Contains(remoteURL, "github.com/") {
		// HTTPS: https://github.com/user/repo.git
		parts := strings.Split(remoteURL, "github.com/")
		if len(parts) == 2 {
			repoPath = strings.TrimSuffix(parts[1], ".git")
		}
	}

	if repoPath == "" {
		return fmt.Sprintf("Create PR manually for branch: %s", branch)
	}

	return fmt.Sprintf("https://github.com/%s/compare/%s...%s?expand=1", repoPath, getBaseBranch(), branch)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func installHook() error {
	// Get git hooks directory
	gitDir, err := runGit("rev-parse", "--git-dir")
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	hookPath := filepath.Join(hooksDir, "post-commit")

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	hookContent := fmt.Sprintf(`#!/bin/sh
# Skill Extractor Post-Commit Hook

COMMIT_SHA=$(git rev-parse HEAD)
%s "$COMMIT_SHA"
`, exePath)

	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}

	fmt.Printf("✓ Installed post-commit hook at %s\n", hookPath)
	return nil
}

func init() {
	if len(os.Args) > 1 && os.Args[1] == "install" {
		if err := installHook(); err != nil {
			fmt.Printf("Error installing hook: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}