package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func writeRegularFile(abs, combRoot string, content []byte) error {
	abs, err := filepath.Abs(abs)
	if err != nil {
		return err
	}
	combRoot, err = filepath.Abs(combRoot)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(combRoot); err == nil {
		combRoot = resolved
	}
	parent := filepath.Dir(abs)
	if info, err := os.Lstat(parent); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifacts: parent of %q is a symlink", abs)
		}
		resolvedParent, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return err
		}
		if pathEscapes(combRoot, resolvedParent) {
			return fmt.Errorf("artifacts: write escapes comb")
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if pathEscapes(combRoot, parent) {
		return fmt.Errorf("artifacts: write escapes comb")
	}

	if info, err := os.Lstat(abs); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifacts: ref is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}
