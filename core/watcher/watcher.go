// Package watcher recursively watches a service's source tree with fsnotify
// and reports debounced change events so the supervisor can restart on edits.
package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gobwas/glob"

	"github.com/toaweme/log"

	"github.com/toaweme/blink/core/config"
)

const debounceWindow = 200 * time.Millisecond

// EventKind distinguishes a content change from a delete-triggered restart.
type EventKind int

const (
	// EventChange is fired when a tracked file is created or written.
	EventChange EventKind = iota
	// EventDelete is fired when a file matching Reload.ReloadOnDelete is removed.
	EventDelete
)

// Event is the debounced restart signal emitted to the supervisor.
type Event struct {
	Service string
	Kind    EventKind
	// Paths is the deduped set of paths that contributed to this event.
	Paths []string
	// SetupTrigger is true when at least one contributing path is a
	// runtime-declared setup trigger (a manifest or lockfile), so the
	// supervisor re-runs Commands.Setup alongside the restart.
	SetupTrigger bool
}

// Watcher watches one service's filesystem footprint and emits debounced events.
type Watcher struct {
	service config.Service
	root    string
	roots   []string // absolute recursive roots

	extensions    map[string]struct{}
	setupTriggers map[string]struct{} // base filenames that re-run Commands.Setup
	includeGlobs  []glob.Glob
	excludeGlobs  []glob.Glob
	deleteGlobs   []glob.Glob

	// strictInclude is set when every Fs.Include entry resolves to a file, not a
	// directory. Then only paths matching an include glob fire restarts, and the
	// implicit DirRoot/Service.Dir recursive root is skipped.
	strictInclude bool

	fsw *fsnotify.Watcher
	out chan Event

	// fileCount and dirCount are populated by addRecursive and surfaced via
	// Stats(). fsnotify registers only directories, but users want the file
	// total. seen tracks every counted path so overlapping roots and re-walks on
	// directory-create events don't double-count.
	mu        sync.Mutex
	fileCount int
	dirCount  int
	seen      map[string]struct{}
}

// Stats returns the current count of files and directories under all
// recursive roots. Safe to call concurrently.
func (w *Watcher) Stats() (files, dirs int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fileCount, w.dirCount
}

// New constructs a Watcher for the given service. extraRoots are additional
// absolute paths (typically runtime-contributed, e.g. go.work modules) watched
// recursively alongside DirRoot/Service.Dir and Service.Fs.Include. The watcher
// must be started with Start(ctx) before events are produced.
func New(cfg config.Config, svc config.Service, extraRoots ...string) (*Watcher, error) {
	if !svc.Reload.Reload && len(svc.Reload.ReloadOnDelete) == 0 {
		return nil, fmt.Errorf("watcher: service %q has no reload triggers configured", svc.Name)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	w := &Watcher{
		service:       svc,
		root:          cfg.DirRoot,
		extensions:    extSet(svc.Fs.Extensions),
		setupTriggers: map[string]struct{}{},
		fsw:           fsw,
		out:           make(chan Event, 8),
		seen:          map[string]struct{}{},
	}

	for _, pat := range svc.Fs.Include {
		g, err := compileGlob(pat)
		if err != nil {
			return nil, fmt.Errorf("failed to compile include pattern %q: %w", pat, err)
		}
		w.includeGlobs = append(w.includeGlobs, g)
	}
	for _, pat := range append(config.DefaultExcludes(), svc.Fs.Exclude...) {
		g, err := compileGlob(pat)
		if err != nil {
			return nil, fmt.Errorf("failed to compile exclude pattern %q: %w", pat, err)
		}
		w.excludeGlobs = append(w.excludeGlobs, g)
	}
	for _, pat := range svc.Reload.ReloadOnDelete {
		g, err := compileGlob(pat)
		if err != nil {
			return nil, fmt.Errorf("failed to compile reload_on_delete pattern %q: %w", pat, err)
		}
		w.deleteGlobs = append(w.deleteGlobs, g)
	}

	w.roots = w.resolveRoots(extraRoots)
	return w, nil
}

// SetSetupTriggers registers base filenames whose change re-runs Commands.Setup
// (in addition to the normal restart). Call before Start. Names are matched by
// base name regardless of Fs.Extensions, so lockfiles are observed even when
// their extension is not otherwise watched.
func (w *Watcher) SetSetupTriggers(names []string) {
	for _, n := range names {
		w.setupTriggers[filepath.Base(n)] = struct{}{}
	}
}

// isSetupTrigger reports whether path's base name is a registered setup trigger.
func (w *Watcher) isSetupTrigger(path string) bool {
	_, ok := w.setupTriggers[filepath.Base(path)]
	return ok
}

// anySetupTrigger reports whether any path is a registered setup trigger.
func (w *Watcher) anySetupTrigger(paths []string) bool {
	for _, p := range paths {
		if w.isSetupTrigger(p) {
			return true
		}
	}
	return false
}

// Events returns the debounced event channel. It is closed when Start's
// context is canceled.
func (w *Watcher) Events() <-chan Event { return w.out }

// Start begins watching. It returns after the initial roots are registered;
// the actual watch loop runs in a goroutine until ctx is done.
func (w *Watcher) Start(ctx context.Context) error {
	for _, root := range w.roots {
		if err := w.addRecursive(root); err != nil {
			log.Warn("watcher: failed to add root", "service", w.service.Name, "root", root, "error", err)
		}
	}
	log.Debug("watcher started", "service", w.service.Name, "roots", w.roots)
	go w.loop(ctx)
	return nil
}

// resolveRoots picks the recursive watch roots for this service:
//   - every directory listed in Fs.Include
//   - every runtime-contributed extra root
//   - the implicit DirRoot/Service.Dir, unless Include is entirely file globs (then each file's parent dir is watched so fsnotify can deliver events, and matchesChange enforces strict glob matching).
func (w *Watcher) resolveRoots(extra []string) []string {
	roots := make([]string, 0, 1+len(w.service.Fs.Include)+len(extra))

	hasInclude := len(w.service.Fs.Include) > 0
	hasDirInclude := false
	for _, inc := range w.service.Fs.Include {
		path := inc
		if !filepath.IsAbs(path) {
			path = filepath.Join(w.root, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			// path doesn't exist yet; the parent dir watch picks it up on create.
			roots = append(roots, filepath.Dir(path))
			continue
		}
		if info.IsDir() {
			hasDirInclude = true
			roots = append(roots, path)
		} else {
			roots = append(roots, filepath.Dir(path))
		}
	}

	w.strictInclude = hasInclude && !hasDirInclude

	if !w.strictInclude {
		roots = append(roots, filepath.Join(w.root, w.service.Dir))
	}

	for _, p := range extra {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			roots = append(roots, p)
		}
	}
	return dedupe(roots)
}

func (w *Watcher) addRecursive(root string) error {
	var files, dirs int
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// best-effort walk: skip entries we cannot stat (permission denied,
			// races with deletes) and keep registering the rest.
			return nil //nolint:nilerr // intentionally tolerate per-entry walk errors and continue.
		}
		if info.IsDir() {
			if w.isExcluded(path) {
				return filepath.SkipDir
			}
			// always (re-)register the dir watch (fsnotify.Add is idempotent), but
			// count it only the first time we see this path.
			if err := w.fsw.Add(path); err != nil {
				log.Debug("watcher: failed to add dir", "service", w.service.Name, "path", path, "error", err)
			}
			if w.markSeen(path) {
				dirs++
			}
			return nil
		}
		if w.isExcluded(path) {
			return nil
		}
		if !w.extOK(path) {
			return nil
		}
		if w.markSeen(path) {
			files++
		}
		return nil
	})
	w.mu.Lock()
	w.fileCount += files
	w.dirCount += dirs
	w.mu.Unlock()
	return err
}

// markSeen records path as counted and reports whether it was newly added.
// A path that was already counted (overlapping roots, a re-walk triggered by
// a directory-create event) returns false so it isn't tallied twice.
func (w *Watcher) markSeen(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.seen[path]; ok {
		return false
	}
	w.seen[path] = struct{}{}
	return true
}

func (w *Watcher) loop(ctx context.Context) {
	defer close(w.out)
	defer w.fsw.Close()

	var (
		pendingChange = map[string]struct{}{}
		pendingDelete = map[string]struct{}{}
		timer         *time.Timer
		timerC        <-chan time.Time
	)

	armTimer := func() {
		if timer == nil {
			timer = time.NewTimer(debounceWindow)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounceWindow)
		}
		timerC = timer.C
	}

	flush := func() {
		if len(pendingChange) > 0 {
			paths := keys(pendingChange)
			w.emit(Event{Service: w.service.Name, Kind: EventChange, Paths: paths, SetupTrigger: w.anySetupTrigger(paths)})
			pendingChange = map[string]struct{}{}
		}
		if len(pendingDelete) > 0 {
			w.emit(Event{Service: w.service.Name, Kind: EventDelete, Paths: keys(pendingDelete)})
			pendingDelete = map[string]struct{}{}
		}
		timerC = nil
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handle(ev, pendingChange, pendingDelete)
			if len(pendingChange) > 0 || len(pendingDelete) > 0 {
				armTimer()
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Debug("watcher error", "service", w.service.Name, "error", err)
		case <-timerC:
			flush()
		}
	}
}

func (w *Watcher) handle(ev fsnotify.Event, change, deletes map[string]struct{}) {
	// auto-add new directories so subtree changes get tracked.
	if ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if !w.isExcluded(ev.Name) {
				_ = w.addRecursive(ev.Name)
			}
			return
		}
	}

	if ev.Op&fsnotify.Remove != 0 || ev.Op&fsnotify.Rename != 0 {
		if w.matchesDelete(ev.Name) {
			deletes[ev.Name] = struct{}{}
		}
		return
	}

	if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return
	}
	if w.matchesChange(ev.Name) {
		change[ev.Name] = struct{}{}
	}
}

func (w *Watcher) matchesChange(path string) bool {
	if w.isExcluded(path) {
		return false
	}
	// a manifest/lockfile change always reloads (and re-runs setup), even when
	// its extension is not in the watched set.
	if w.isSetupTrigger(path) {
		return true
	}
	if !w.extOK(path) {
		return false
	}
	// strict include mode: when Fs.Include is a set of file globs, only those
	// files trigger restarts, so skip the under-root fallback.
	if w.strictInclude {
		return anyMatch(w.includeGlobs, path)
	}
	if len(w.includeGlobs) > 0 && !anyMatch(w.includeGlobs, path) {
		// files inside recursive roots are implicitly included; includeGlobs
		// only narrows when set with file globs, so paths under those roots
		// pass through.
		if !w.underRoot(path) {
			return false
		}
	}
	return true
}

func (w *Watcher) matchesDelete(path string) bool {
	if len(w.deleteGlobs) == 0 {
		return false
	}
	return anyMatch(w.deleteGlobs, path)
}

func (w *Watcher) isExcluded(path string) bool {
	return anyMatch(w.excludeGlobs, path)
}

func (w *Watcher) extOK(path string) bool {
	if len(w.extensions) == 0 {
		return true
	}
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	_, ok := w.extensions[ext]
	return ok
}

func (w *Watcher) underRoot(path string) bool {
	for _, root := range w.roots {
		if strings.HasPrefix(path, root+string(os.PathSeparator)) || path == root {
			return true
		}
	}
	return false
}

func (w *Watcher) emit(ev Event) {
	select {
	case w.out <- ev:
	default:
		// drop on a slow consumer to avoid blocking the watch loop; restarts
		// coalesce upstream so a lost event is harmless.
		log.Debug("watcher: dropped event (slow consumer)", "service", w.service.Name)
	}
}

func compileGlob(pat string) (glob.Glob, error) {
	return glob.Compile(pat, '/')
}

func anyMatch(globs []glob.Glob, path string) bool {
	for _, g := range globs {
		if g.Match(path) {
			return true
		}
	}
	return false
}

func extSet(exts []string) map[string]struct{} {
	if len(exts) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		out[strings.TrimPrefix(strings.ToLower(e), ".")] = struct{}{}
	}
	return out
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
