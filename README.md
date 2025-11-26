# prrompt

**pr**rompt is a lightweight git post-commit hook that extracts prompts from commits and creates a new branch for just the prompts.

## Problem

When you're working on a project, you often need to update the prompts for your AI agents and share them with your team as quick as possible to help them work efficiently. This is a pain because you have to:
- Keep track of prompt changes separately from the rest of the actual code changes
- Manually cherry-pick the prompt changes into a new branch
- Create a PR for the prompt changes
- Merge them into the main branch

What is saw is people deffering merging prompt changes into the main branch until they finish working on a feature. In the reality AI agents prompts are usualy (however not always) logiclly separate from the rest of the code and should be merged as soon as possible to keep improving the AI workflow on a team.

## Solution

**pr**rompt does a simple thing. It checks if the current commit contains any prompt changes. If it does, it creates a new branch for just the prompts and pushes it to the remote repository.

## Installation

### Using GoReleaser Releases

1. Download the release archive for your platform from the [releases page](https://github.com/Ilnicki010/prrompt/releases):
   - **macOS**: `prrompt_Darwin_x86_64.tar.gz` (Intel) or `prrompt_Darwin_arm64.tar.gz` (Apple Silicon)
   - **Linux**: `prrompt_Linux_x86_64.tar.gz`, `prrompt_Linux_i386.tar.gz`, or `prrompt_Linux_arm64.tar.gz`

2. Extract the archive and move the binary to a directory in your `PATH`:
   ```bash
   # macOS/Linux example
   tar -xzf prrompt_Darwin_x86_64.tar.gz
   sudo mv prrompt /usr/local/bin/
   
   # Or to a local bin directory
   mkdir -p ~/.local/bin
   mv prrompt ~/.local/bin/
   export PATH="$HOME/.local/bin:$PATH"  # Add to ~/.zshrc or ~/.bashrc
   ```

3. Run `prrompt install` in your project directory to install the git hook:
   ```bash
   cd your-project
   prrompt install
   ```

## Configuration

**pr**rompt is configured using a `.gitconfig` file. You can configure the following settings:

- `prrompt.commitPrefix`: The prefix to use for the commit message (default: `prompt`)
- `prrompt.branchPrefix`: The prefix to use for the branch name (default: `prompt-update`)
- `prrompt.baseBranch`: The base branch to create the prompt branch from (default: `main`)
- `prrompt.promptPatterns`: The patterns to use for the prompt files (default: `prompts/,.claude/skills/`)
- `prrompt.verbosity`: The verbosity level (default: `low`)

## Usage

**pr**rompt is a git post-commit hook. It will automatically run when you commit your changes.