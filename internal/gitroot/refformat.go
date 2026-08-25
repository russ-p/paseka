package gitroot

import (
	"fmt"
	"os/exec"
	"strings"
)

// IsolatedWorktreeBranch returns the anonymous default branch for a trace: paseka/<safeTraceId>.
func IsolatedWorktreeBranch(traceID string) string {
	safe := strings.NewReplacer("/", "-", " ", "-").Replace(traceID)
	return "paseka/" + safe
}

// CheckRefFormat validates a single branch ref name using git check-ref-format.
func CheckRefFormat(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("empty ref")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("must not start with '-'")
	}
	if strings.HasPrefix(ref, "refs/") {
		return fmt.Errorf("refs/ prefix not allowed")
	}
	cmd := exec.Command("git", "check-ref-format", "--allow-onelevel", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// LocalBranchExists reports whether refs/heads/<branch> exists in the repo.
func LocalBranchExists(repoRoot, branch string) bool {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false
	}
	cmd := exec.Command("git", "-C", repoRoot, "show-ref", "--verify", "--quiet", "--", "refs/heads/"+branch)
	return cmd.Run() == nil
}
