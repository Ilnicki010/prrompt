package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	branchPrefix = "skill-update"
	baseBranch   = "main"
)

// Skill file patterns - files matching these will be extracted
var skillPatterns = []string{
	".claude/skills/",
	"prompts/",
}

type CommitInfo struct {
	SHA         string
	Message     string
	SkillFiles  []string
	OtherFiles  []string
	IsMixed     bool
	SourceBranch string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: skill-extractor <commit-sha>")
		fmt.Println("This tool is meant to be run as a git post-commit hook")
		os.Exit(1)
	}

	commitSHA := os.Args[1]
	
	// Check if we're on a skill branch - if so, skip to avoid recursion
	currentBranch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err == nil && strings.HasPrefix(currentBranch, branchPrefix+"/") {
		// We're on a skill branch, don't process
		return
	}
	
	// Get commit info
	info, err := analyzeCommit(commitSHA)
	if err != nil {
		fmt.Printf("Error analyzing commit: %v\n", err)
		os.Exit(1)
	}

	// Skip if no skill files
	if len(info.SkillFiles) == 0 {
		return
	}

	// Extract skills
	if err := extractSkills(info); err != nil {
		fmt.Printf("Error extracting skills: %v\n", err)
		os.Exit(1)
	}
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func isSkillFile(path string) bool {
	for _, pattern := range skillPatterns {
		if strings.HasPrefix(path, pattern) {
			return true
		}
	}
	return false
}

func analyzeCommit(sha string) (*CommitInfo, error) {
	info := &CommitInfo{SHA: sha}

	// Get commit message
	msg, err := runGit("log", "--format=%B", "-n", "1", sha)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit message: %w", err)
	}
	info.Message = msg

	// Get current branch
	branch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	info.SourceBranch = branch

	// Get changed files
	files, err := runGit("diff-tree", "--no-commit-id", "--name-only", "-r", sha)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	for _, file := range strings.Split(files, "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if isSkillFile(file) {
			info.SkillFiles = append(info.SkillFiles, file)
		} else {
			info.OtherFiles = append(info.OtherFiles, file)
		}
	}

	info.IsMixed = len(info.OtherFiles) > 0

	return info, nil
}

func extractSkills(info *CommitInfo) error {
	shortSHA := info.SHA[:7]
	skillBranch := fmt.Sprintf("%s/%s", branchPrefix, shortSHA)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("Processing commit: %s\n", shortSHA)
	fmt.Printf("Message: %s\n", truncate(info.Message, 60))
	fmt.Printf("Skill files: %d\n", len(info.SkillFiles))
	fmt.Printf("Other files: %d\n", len(info.OtherFiles))
	if info.IsMixed {
		fmt.Println("Type: MIXED")
	} else {
		fmt.Println("Type: SKILL-ONLY")
	}
	fmt.Println(strings.Repeat("=", 60))

	// Create and checkout new branch from base
	fmt.Printf("\nCreating branch: %s\n", skillBranch)
	if _, err := runGit("checkout", "-b", skillBranch, baseBranch); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Cherry-pick without committing
	fmt.Printf("Cherry-picking commit %s...\n", shortSHA)
	if _, err := runGit("cherry-pick", info.SHA, "--no-commit"); err != nil {
		cleanup(info.SourceBranch, skillBranch)
		return fmt.Errorf("failed to cherry-pick: %w", err)
	}

	// For mixed commits, remove non-skill files
	if info.IsMixed {
		fmt.Println("Mixed commit detected - extracting only skill files...")
		for _, file := range info.OtherFiles {
			fmt.Printf("  Removing: %s\n", file)
			runGit("restore", "--staged", file)
			runGit("restore", file)
		}
	}

	// Create commit message
	commitMsg := fmt.Sprintf("skill: %s", info.Message)
	if info.IsMixed {
		commitMsg += fmt.Sprintf("\n\nExtracted from %s (%s)", info.SourceBranch, shortSHA)
	}

	// Commit
	if _, err := runGit("commit", "-m", commitMsg); err != nil {
		cleanup(info.SourceBranch, skillBranch)
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("✓ Created skill branch: %s\n", skillBranch)

	// Push to remote
	fmt.Println("Pushing to origin...")
	if _, err := runGit("push", "origin", skillBranch, "-u"); err != nil {
		fmt.Printf("Warning: failed to push (you may need to push manually): %v\n", err)
	} else {
		fmt.Printf("✓ Pushed to origin/%s\n", skillBranch)
	}

	// Return to original branch
	if _, err := runGit("checkout", info.SourceBranch); err != nil {
		return fmt.Errorf("failed to return to original branch: %w", err)
	}

	// Generate PR URL
	prURL := generatePRURL(skillBranch)
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

	return fmt.Sprintf("https://github.com/%s/compare/%s...%s?expand=1", repoPath, baseBranch, branch)
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
	// If run with "install" argument, install the hook
	if len(os.Args) > 1 && os.Args[1] == "install" {
		if err := installHook(); err != nil {
			fmt.Printf("Error installing hook: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}