package workspace

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CopyRepo copies a repository/workdir into dst while applying simple ignore rules.
func CopyRepo(src, dst string, ignore []string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dstAbs, 0o755); err != nil {
		return err
	}

	return filepath.WalkDir(srcAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcAbs, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if shouldIgnore(rel, d.IsDir(), ignore) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		outPath := filepath.Join(dstAbs, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		info, err := d.Info()
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}

		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}

		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func shouldIgnore(rel string, isDir bool, ignore []string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	_ = isDir
	for _, g := range ignore {
		g = filepath.ToSlash(strings.TrimSpace(g))
		if g == "" {
			continue
		}
		if strings.HasSuffix(g, "/**") {
			p := strings.TrimSuffix(g, "/**")
			if rel == p || strings.HasPrefix(rel, p+"/") {
				return true
			}
			continue
		}
		if strings.HasPrefix(g, "**/") {
			s := strings.TrimPrefix(g, "**/")
			if rel == s || strings.HasSuffix(rel, "/"+s) {
				return true
			}
			continue
		}
		if rel == g {
			return true
		}
	}
	return false
}
