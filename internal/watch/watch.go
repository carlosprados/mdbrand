// Package watch reruns a build whenever one of the files it read changes.
//
// It watches directories, never files. Editors save by writing a temporary and
// renaming it over the original (Neovim, VS Code, most others), which removes
// the inode an inotify watch on the file was attached to: the first :w would
// be the last change ever seen. A watch on the directory survives that, and
// events are filtered down to the paths the last build reported as inputs.
//
// The output PDF is never an input, so writing it cannot trigger a build and
// there is no exclusion list to keep in step with --out and --work.
package watch

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Build runs once and returns the files it read. It must return them even when
// the build failed, since the fix arrives as a change to one of them.
type Build func() []string

// Debounce is how long the tree must stay quiet before a build starts. One save
// is a burst of events (create, write, chmod, rename) and one build is wanted.
const Debounce = 300 * time.Millisecond

// Run builds once, then again after every change to an input, until ctx ends.
// A change that lands while a build runs queues exactly one more build.
func Run(ctx context.Context, build Build, logf func(string, ...any)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	inputs := map[string]bool{}
	dirs := map[string]bool{}
	rebuild := func() {
		inputs, dirs = track(w, build(), dirs, logf)
	}
	rebuild()

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			// Chmod alone is metadata: Dropbox and backup tools touch it
			// without changing a byte.
			if ev.Op == fsnotify.Chmod || !inputs[filepath.Clean(ev.Name)] {
				continue
			}
			timer.Reset(Debounce)
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			logf("watch: %v", err)
		case <-timer.C:
			rebuild()
		}
	}
}

// track points the watcher at the directories holding paths and returns the
// set to filter events by. Each path is resolved through symlinks, because
// inotify reports the directory that really changed: a bundle linked into the
// brands directory changes under its real path, not under the link.
func track(w *fsnotify.Watcher, paths []string, old map[string]bool, logf func(string, ...any)) (inputs, dirs map[string]bool) {
	inputs, dirs = map[string]bool{}, map[string]bool{}
	for _, p := range paths {
		for _, q := range resolve(p) {
			inputs[q] = true
			dirs[filepath.Dir(q)] = true
		}
	}
	for d := range dirs {
		if old[d] {
			continue
		}
		if err := w.Add(d); err != nil {
			// A bibliography in a directory that does not exist yet, say. The
			// build already failed naming it; there is nothing here to watch.
			logf("watch: cannot watch %s: %v", d, err)
			delete(dirs, d)
		}
	}
	for d := range old {
		if !dirs[d] {
			_ = w.Remove(d)
		}
	}
	return inputs, dirs
}

// resolve returns p as given (absolute and clean) and, when it differs, the
// file it links to. A missing file cannot be resolved and is kept as written,
// so creating it is still seen.
func resolve(p string) []string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	out := []string{abs}
	if real, err := filepath.EvalSymlinks(abs); err == nil && real != abs {
		out = append(out, real)
	} else if err != nil {
		// The file is missing but its directory may be a link.
		if dir, derr := filepath.EvalSymlinks(filepath.Dir(abs)); derr == nil && dir != filepath.Dir(abs) {
			out = append(out, filepath.Join(dir, filepath.Base(abs)))
		}
	}
	return out
}
