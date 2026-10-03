// Package repopath provides shared repository-root path resolution for
// evidence checks and changed-file ratchets.
package repopath

import (
	"os"
	"path/filepath"
)

// Root finds the nearest Git worktree root containing dir. If dir is outside
// a Git worktree, its absolute path is the root, making callers compare paths
// relative to the scan directory.
func Root(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	cur := absDir
	for {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	// Forgejo Actions can check out source as an archive without a .git
	// directory. Use the outermost Go module containing the scan directory
	// so repo-root-relative changed-file lists still resolve, even when the
	// caller runs from a nested module or a different working directory.
	if moduleRoot := outermostModuleRootFrom(absDir); moduleRoot != "" {
		return moduleRoot
	}
	return absDir
}

func outermostModuleRootFrom(dir string) string {
	var moduleRoot string
	for cur := dir; ; cur = filepath.Dir(cur) {
		if info, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil && !info.IsDir() {
			moduleRoot = cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return moduleRoot
		}
	}
}

// Relative returns path relative to root, normalized for changed-file lists.
func Relative(root, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	return filepath.ToSlash(rel), err
}
