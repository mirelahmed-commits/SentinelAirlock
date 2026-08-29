package util

import "os"

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func DefaultIfEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
