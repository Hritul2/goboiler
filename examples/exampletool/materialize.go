package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// materialize copies the project at src into dst and lays the overlay on top.
// The project is "git ls-files -co --exclude-standard": tracked files plus
// untracked files that are not ignored, so a scratch copy of a working tree
// matches what would be committed. examples/ itself is left out, so the copy
// looks like a project after `make init` (no example targets, no recursion).
func materialize(src, overlay, dst string, clean bool) (int, error) {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return 0, err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return 0, err
	}
	if err := checkDst(srcAbs, dstAbs, clean); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dstAbs, 0o755); err != nil {
		return 0, err
	}

	cmd := exec.Command("git", "ls-files", "-z", "-c", "-o", "--exclude-standard")
	cmd.Dir = srcAbs
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	list, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git ls-files in %s: %v: %s", srcAbs, err, strings.TrimSpace(stderr.String()))
	}

	n := 0
	for _, rel := range strings.Split(string(list), "\x00") {
		if rel == "" || rel == "examples" || strings.HasPrefix(rel, "examples/") {
			continue
		}
		// A tracked file deleted in the working tree is still listed.
		ok, err := copyFile(filepath.Join(srcAbs, filepath.FromSlash(rel)), filepath.Join(dstAbs, filepath.FromSlash(rel)))
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}

	ov := filepath.Join(srcAbs, filepath.FromSlash(overlay))
	if filepath.IsAbs(overlay) {
		ov = overlay
	}
	info, err := os.Stat(ov)
	if err != nil || !info.IsDir() {
		return n, fmt.Errorf("overlay directory %s not found", ov)
	}
	err = filepath.WalkDir(ov, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(ov, p)
		if err != nil {
			return err
		}
		ok, err := copyFile(p, filepath.Join(dstAbs, rel))
		if ok {
			n++
		}
		return err
	})
	return n, err
}

// checkDst refuses a destination that would destroy the source or hold other
// work: it must be empty (or absent) unless clean is set, and it must never
// contain or equal the source tree.
func checkDst(src, dst string, clean bool) error {
	if dst == src || strings.HasPrefix(src+string(filepath.Separator), dst+string(filepath.Separator)) {
		return fmt.Errorf("-dst %s must not be the source tree or one of its parents", dst)
	}
	entries, err := os.ReadDir(dst)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if !clean {
		return fmt.Errorf("-dst %s is not empty; pass -clean to empty it first", dst)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a regular file, keeping its permission bits. Anything else
// (a deleted file, a directory, a symlink) is skipped and reported as false.
func copyFile(from, to string) (bool, error) {
	info, err := os.Lstat(from)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return false, err
	}
	in, err := os.Open(from)
	if err != nil {
		return false, err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return false, err
	}
	return true, out.Close()
}
