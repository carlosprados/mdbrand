package build

import (
	"os"
	"sync"
)

// Work directories are registered while they live, so that an interrupt can
// remove them: Ctrl-C ends the process without running a single defer, and
// every interrupted build left an mdbrand-* directory in the temp dir.
var live = struct {
	sync.Mutex
	dirs map[string]bool
}{dirs: map[string]bool{}}

// TempDir is os.MkdirTemp for a directory RemoveTempDirs can find.
func TempDir(pattern string) (string, error) {
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", err
	}
	live.Lock()
	live.dirs[dir] = true
	live.Unlock()
	return dir, nil
}

// RemoveTempDir removes one directory TempDir made.
func RemoveTempDir(dir string) {
	live.Lock()
	delete(live.dirs, dir)
	live.Unlock()
	_ = os.RemoveAll(dir) // a temp dir that will not go is not the build's failure
}

// RemoveTempDirs removes every directory TempDir made and nothing removed
// yet: what an interrupt calls before it exits.
func RemoveTempDirs() {
	live.Lock()
	defer live.Unlock()
	for dir := range live.dirs {
		_ = os.RemoveAll(dir)
		delete(live.dirs, dir)
	}
}
