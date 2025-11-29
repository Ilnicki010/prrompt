package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type testRepo struct {
	Dir        string
	BranchName string
}

func setupTestRepo(t *testing.T) testRepo {
	tmpDir := t.TempDir()
	branchName := "feature-branch"
	
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "config", "core.hooksPath", "/dev/null")
	cmd.Dir = tmpDir
	cmd.Run()

	runGitInDir(tmpDir, "commit", "--allow-empty", "-m", "Initial commit")

	runGitInDir(tmpDir, "checkout", "-b", branchName)

	return testRepo{Dir: tmpDir, BranchName: branchName}
}

func runGitInDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func runPrrompt(t *testing.T, repoDir string, commitSHA string) error {
	// Change to repo directory before calling processCommit
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(oldDir)
	
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("Failed to change to repo directory: %v", err)
	}
	
	return processCommit(commitSHA)
}

func Test_PromptOnlyCommit(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo.Dir)

	// Create a commit with only prompt files
	promptFile := filepath.Join(repo.Dir, ".claude/skills/test.md")
	os.MkdirAll(filepath.Dir(promptFile), 0755)
	os.WriteFile(promptFile, []byte("# Test prompt"), 0644)

	runGitInDir(repo.Dir, "add", promptFile)
	runGitInDir(repo.Dir, "commit", "-m", "Add prompt file")

	// Get the commit SHA
	commitSHA, _ := runGitInDir(repo.Dir, "rev-parse", "HEAD")
	shortSHA := commitSHA[:7]

	// Run prrompt
	if err := runPrrompt(t, repo.Dir, commitSHA); err != nil {
		t.Fatalf("prrompt failed: %v", err)
	}

	// Verify we're back on the feature branch (original branch)
	currentBranch, _ := runGitInDir(repo.Dir, "rev-parse", "--abbrev-ref", "HEAD")
	if currentBranch != repo.BranchName {
		t.Errorf("Expected to be back on %s, but on %s", repo.BranchName, currentBranch)
	}

	// Verify the prompt branch has the commit with correct prefix
	expectedBranch := defaultBranchPrefix + "/" + shortSHA
	runGitInDir(repo.Dir, "checkout", expectedBranch)
	commitMsg, _ := runGitInDir(repo.Dir, "log", "--format=%B", "-n", "1", "HEAD")
	if !strings.Contains(commitMsg, "["+defaultCommitPrefix+"]") {
		t.Errorf("Expected commit message to contain [%s], got: %s", defaultCommitPrefix, commitMsg)
	}
	if !strings.Contains(commitMsg, "Add prompt file") {
		t.Errorf("Expected commit message to contain original message, got: %s", commitMsg)
	}

	// Verify the prompt file exists in the branch
	if _, err := os.Stat(promptFile); os.IsNotExist(err) {
		t.Error("Prompt file should exist in the branch")
	}
}

func Test_MixedCommit(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo.Dir)

	// Create a mixed commit with both prompt and other files
	promptFile := filepath.Join(repo.Dir, "prompts/test.md")
	os.MkdirAll(filepath.Dir(promptFile), 0755)
	os.WriteFile(promptFile, []byte("# Test prompt"), 0644)

	otherFile := filepath.Join(repo.Dir, "src/main.go")
	os.MkdirAll(filepath.Dir(otherFile), 0755)
	os.WriteFile(otherFile, []byte("package main"), 0644)

	runGitInDir(repo.Dir, "add", promptFile, otherFile)
	runGitInDir(repo.Dir, "commit", "-m", "Add prompt and code")

	// Get the commit SHA
	commitSHA, _ := runGitInDir(repo.Dir, "rev-parse", "HEAD")
	shortSHA := commitSHA[:7]

	// Run prrompt
	if err := runPrrompt(t, repo.Dir, commitSHA); err != nil {
		t.Fatalf("prrompt failed: %v", err)
	}

	// Verify branch was created
	expectedBranch := defaultBranchPrefix + "/" + shortSHA
	branches, _ := runGitInDir(repo.Dir, "branch", "--list", expectedBranch)
	if !strings.Contains(branches, expectedBranch) {
		t.Errorf("Expected branch %s not found. Branches: %s", expectedBranch, branches)
	}

	// Verify we're back on the feature branch (original branch)
	currentBranch, _ := runGitInDir(repo.Dir, "rev-parse", "--abbrev-ref", "HEAD")
	if currentBranch != repo.BranchName {
		t.Errorf("Expected to be back on %s, but on %s", repo.BranchName, currentBranch)
	}

	if _, err := os.Stat(otherFile); os.IsNotExist(err) {
		t.Error("Other file should still exist in the working directory - prrompt should not remove files")
	}

	// Verify the prompt branch only has the prompt file in the commit
	runGitInDir(repo.Dir, "checkout", expectedBranch)
	
	// Check what files are in the commit
	filesInCommit, _ := runGitInDir(repo.Dir, "ls-tree", "-r", "--name-only", "HEAD")
	
	// Prompt file should exist in the commit
	if !strings.Contains(filesInCommit, ".claude/skills/test.md") && !strings.Contains(filesInCommit, "prompts/test.md") {
		t.Errorf("Prompt file should exist in the branch. Files in commit: %s", filesInCommit)
	}

	// Other file should NOT exist in the commit
	if strings.Contains(filesInCommit, "src/main.go") {
		t.Errorf("Other file should NOT exist in the prompt branch commit. Files in commit: %s", filesInCommit)
	}

	// Verify commit message includes extraction info
	commitMsg, _ := runGitInDir(repo.Dir, "log", "--format=%B", "-n", "1", "HEAD")
	if !strings.Contains(commitMsg, "Extracted from") {
		t.Errorf("Expected commit message to contain extraction info, got: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, shortSHA) {
		t.Errorf("Expected commit message to contain commit SHA, got: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, repo.BranchName) {
		t.Errorf("Expected commit message to contain source branch %s, got: %s", repo.BranchName, commitMsg)
	}
}

func Test_NoPromptFiles(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo.Dir)

	// Create a commit with only non-prompt files
	codeFile := filepath.Join(repo.Dir, "src/main.go")
	os.MkdirAll(filepath.Dir(codeFile), 0755)
	os.WriteFile(codeFile, []byte("package main"), 0644)

	runGitInDir(repo.Dir, "add", codeFile)
	runGitInDir(repo.Dir, "commit", "-m", "Add code file")

	// Get the commit SHA
	commitSHA, _ := runGitInDir(repo.Dir, "rev-parse", "HEAD")

	// Run prrompt - should exit silently without creating branch
	if err := runPrrompt(t, repo.Dir, commitSHA); err != nil {
		t.Fatalf("prrompt should not fail for commits without prompt files: %v", err)
	}

	// Verify no branch was created
	branches, _ := runGitInDir(repo.Dir, "branch", "--list")
	if strings.Contains(branches, defaultBranchPrefix) {
		t.Errorf("No branch should be created for commits without prompt files. Branches: %s", branches)
	}
}

func Test_SkipOnPromptBranch(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo.Dir)

	// Switch to a prompt branch (we're currently on feature-branch from setup)
	runGitInDir(repo.Dir, "checkout", "main")
	
	// Create a prompt branch
	promptBranch := defaultBranchPrefix + "/test123"
	runGitInDir(repo.Dir, "checkout", "-b", promptBranch)

	// Create a commit with prompt files
	promptFile := filepath.Join(repo.Dir, ".claude/skills/test.md")
	os.MkdirAll(filepath.Dir(promptFile), 0755)
	os.WriteFile(promptFile, []byte("# Test prompt"), 0644)

	runGitInDir(repo.Dir, "add", promptFile)
	runGitInDir(repo.Dir, "commit", "-m", "Add prompt file")

	// Get the commit SHA
	commitSHA, _ := runGitInDir(repo.Dir, "rev-parse", "HEAD")

	// Run prrompt - should skip because we're on a prompt branch
	if err := runPrrompt(t, repo.Dir, commitSHA); err != nil {
		t.Fatalf("prrompt should not fail when on prompt branch: %v", err)
	}

	// Verify no new branch was created (we're already on a prompt branch)
	branches, _ := runGitInDir(repo.Dir, "branch", "--list")
	branchCount := strings.Count(branches, defaultBranchPrefix)
	if branchCount != 1 {
		t.Errorf("Should only have one prompt branch, found %d. Branches: %s", branchCount, branches)
	}
}

func Test_CustomConfig(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo.Dir)

	// Set custom config values
	runGitInDir(repo.Dir, "config", "prrompt.commitPrefix", "custom-prefix")
	runGitInDir(repo.Dir, "config", "prrompt.branchPrefix", "custom-branch")
	runGitInDir(repo.Dir, "config", "prrompt.baseBranch", "main")
	runGitInDir(repo.Dir, "config", "prrompt.promptPatterns", "custom/prompts/")

	// Create a commit with prompt file matching custom pattern
	promptFile := filepath.Join(repo.Dir, "custom/prompts/test.md")
	os.MkdirAll(filepath.Dir(promptFile), 0755)
	os.WriteFile(promptFile, []byte("# Test prompt"), 0644)

	runGitInDir(repo.Dir, "add", promptFile)
	runGitInDir(repo.Dir, "commit", "-m", "Add prompt file")

	// Get the commit SHA
	commitSHA, _ := runGitInDir(repo.Dir, "rev-parse", "HEAD")
	shortSHA := commitSHA[:7]

	// Run prrompt
	if err := runPrrompt(t, repo.Dir, commitSHA); err != nil {
		t.Fatalf("prrompt failed: %v", err)
	}

	// Verify branch was created with custom prefix
	expectedBranch := "custom-branch/" + shortSHA
	branches, _ := runGitInDir(repo.Dir, "branch", "--list", expectedBranch)
	if !strings.Contains(branches, expectedBranch) {
		t.Errorf("Expected branch %s not found. Branches: %s", expectedBranch, branches)
	}

	// Verify commit message uses custom prefix
	runGitInDir(repo.Dir, "checkout", expectedBranch)
	commitMsg, _ := runGitInDir(repo.Dir, "log", "--format=%B", "-n", "1", "HEAD")
	if !strings.Contains(commitMsg, "[custom-prefix]") {
		t.Errorf("Expected commit message to contain [custom-prefix], got: %s", commitMsg)
	}
}

func Test_InvalidCommitSHA(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo.Dir)

	// Run prrompt with invalid SHA - should fail
	err := runPrrompt(t, repo.Dir, "invalid-sha-12345")
	if err == nil {
		t.Error("prrompt should fail with invalid commit SHA")
	}

	if err != nil && !strings.Contains(err.Error(), "error analyzing commit") {
		t.Errorf("Expected error message about analyzing commit, got: %v", err)
	}
}

func Test_MultiplePromptPatterns(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo.Dir)

	// Set custom patterns
	runGitInDir(repo.Dir, "config", "prrompt.promptPatterns", "prompts/,skills/,docs/prompts/")

	// Create a commit with files matching different patterns
	promptFile1 := filepath.Join(repo.Dir, "prompts/test1.md")
	promptFile2 := filepath.Join(repo.Dir, "skills/test2.md")
	promptFile3 := filepath.Join(repo.Dir, "docs/prompts/test3.md")
	
	os.MkdirAll(filepath.Dir(promptFile1), 0755)
	os.MkdirAll(filepath.Dir(promptFile2), 0755)
	os.MkdirAll(filepath.Dir(promptFile3), 0755)
	
	os.WriteFile(promptFile1, []byte("# Prompt 1"), 0644)
	os.WriteFile(promptFile2, []byte("# Prompt 2"), 0644)
	os.WriteFile(promptFile3, []byte("# Prompt 3"), 0644)

	runGitInDir(repo.Dir, "add", promptFile1, promptFile2, promptFile3)
	runGitInDir(repo.Dir, "commit", "-m", "Add multiple prompt files")

	// Get the commit SHA
	commitSHA, _ := runGitInDir(repo.Dir, "rev-parse", "HEAD")
	shortSHA := commitSHA[:7]

	// Run prrompt
	if err := runPrrompt(t, repo.Dir, commitSHA); err != nil {
		t.Fatalf("prrompt failed: %v", err)
	}

	// Verify branch was created
	expectedBranch := defaultBranchPrefix + "/" + shortSHA
	branches, _ := runGitInDir(repo.Dir, "branch", "--list", expectedBranch)
	if !strings.Contains(branches, expectedBranch) {
		t.Errorf("Expected branch %s not found. Branches: %s", expectedBranch, branches)
	}

	// Verify all prompt files exist in the branch
	runGitInDir(repo.Dir, "checkout", expectedBranch)
	
	for _, file := range []string{promptFile1, promptFile2, promptFile3} {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("Prompt file %s should exist in the branch", file)
		}
	}
}

func Test_newPromptFileAdded(t *testing.T) {
	repo := setupTestRepo(t)
	defer os.RemoveAll(repo.Dir)

	// Create a commit with a new prompt file
	promptFile := filepath.Join(repo.Dir, ".claude/skills/test.md")
	os.MkdirAll(filepath.Dir(promptFile), 0755)
	os.WriteFile(promptFile, []byte("# Test prompt"), 0644)

	runGitInDir(repo.Dir, "add", promptFile)
	runGitInDir(repo.Dir, "commit", "-m", "Add prompt file")
	
	// Get the commit SHA
	commitSHA, _ := runGitInDir(repo.Dir, "rev-parse", "HEAD")
	shortSHA := commitSHA[:7]

	if err := runPrrompt(t, repo.Dir, commitSHA); err != nil {
		t.Fatalf("prrompt failed: %v", err)
	}

	// Verify branch was created
	expectedBranch := defaultBranchPrefix + "/" + shortSHA
	branches, _ := runGitInDir(repo.Dir, "branch", "--list", expectedBranch)
	if !strings.Contains(branches, expectedBranch) {
		t.Errorf("Expected branch %s not found. Branches: %s", expectedBranch, branches)
	}
}