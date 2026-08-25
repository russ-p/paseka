package protocol

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
)

// Reserved branch ref names that isolated worktrees must not target.
var reservedBranchNames = map[string]struct{}{
	"head":     {},
	"main":     {},
	"master":   {},
	"detached": {},
}

// ValidateBranchRef checks a git branch name for worktree.branch emit and ensure paths.
// colonyRoot is optional; when set, the colony checkout's current branch is also rejected.
func ValidateBranchRef(branch, colonyRoot string) []ValidationDetail {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return []ValidationDetail{{Path: "payload.branch", Message: "required"}}
	}
	if len(branch) > MaxWorktreeBranchLen {
		return []ValidationDetail{{Path: "payload.branch", Message: fmt.Sprintf("must be at most %d characters", MaxWorktreeBranchLen)}}
	}
	if strings.HasPrefix(branch, "-") {
		return []ValidationDetail{{Path: "payload.branch", Message: "must not start with '-'"}}
	}
	if strings.HasPrefix(branch, "refs/") {
		return []ValidationDetail{{Path: "payload.branch", Message: "refs/ prefix not allowed"}}
	}
	if strings.Contains(branch, ":") || strings.Contains(branch, "\\") || strings.Contains(branch, "@{") {
		return []ValidationDetail{{Path: "payload.branch", Message: "invalid ref characters"}}
	}
	if looksLikeRemoteTracking(branch) {
		return []ValidationDetail{{Path: "payload.branch", Message: "remote-tracking ref not allowed"}}
	}
	if _, ok := reservedBranchNames[strings.ToLower(branch)]; ok {
		return []ValidationDetail{{Path: "payload.branch", Message: "reserved branch name"}}
	}
	if err := gitroot.CheckRefFormat(branch); err != nil {
		return []ValidationDetail{{Path: "payload.branch", Message: err.Error()}}
	}
	if colonyRoot != "" {
		defaultBranch, err := gitroot.DefaultBranch(colonyRoot)
		if err == nil {
			defaultBranch = strings.TrimSpace(defaultBranch)
			if defaultBranch != "" && defaultBranch != "HEAD" && strings.EqualFold(branch, defaultBranch) {
				return []ValidationDetail{{Path: "payload.branch", Message: "cannot target the colony default branch"}}
			}
		}
	}
	return nil
}

func looksLikeRemoteTracking(branch string) bool {
	lower := strings.ToLower(branch)
	first, _, _ := strings.Cut(lower, "/")
	switch first {
	case "origin", "remotes", "heads", "refs":
		return true
	default:
		return false
	}
}
