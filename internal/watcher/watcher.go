package watcher

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches a file for changes
type Watcher struct {
	path     string
	onChange func()
	debounce time.Duration
}

// New creates a new file watcher
func New(path string, onChange func()) *Watcher {
	return &Watcher{
		path:     path,
		onChange: onChange,
		debounce: 500 * time.Millisecond,
	}
}

// WithDebounce sets the debounce duration
func (w *Watcher) WithDebounce(d time.Duration) *Watcher {
	w.debounce = d
	return w
}

// Watch starts watching the file for changes
// It blocks until the context is cancelled or an error occurs
func (w *Watcher) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Watch the directory containing the file
	// This handles cases where the file is replaced (e.g., by editors)
	dir := filepath.Dir(w.path)
	filename := filepath.Base(w.path)

	if err := watcher.Add(dir); err != nil {
		return err
	}

	log.Printf("Watching %s for changes", w.path)

	var debounceTimer *time.Timer
	var lastEventTime time.Time

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Check if this event is for our file
			if filepath.Base(event.Name) != filename {
				continue
			}

			// Handle write or create events
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				now := time.Now()

				// Debounce rapid changes
				if debounceTimer != nil {
					debounceTimer.Stop()
				}

				debounceTimer = time.AfterFunc(w.debounce, func() {
					if time.Since(lastEventTime) >= w.debounce {
						log.Printf("File changed: %s", w.path)
						w.onChange()
					}
				})

				lastEventTime = now
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error: %v", err)

		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return ctx.Err()
		}
	}
}

// WatchMultiple watches multiple files and calls onChange when any of them change
func WatchMultiple(ctx context.Context, paths []string, onChange func(path string)) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Track which directories and files we're watching
	watchedDirs := make(map[string]bool)
	fileSet := make(map[string]bool)

	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}

		dir := filepath.Dir(absPath)
		if !watchedDirs[dir] {
			if err := watcher.Add(dir); err != nil {
				log.Printf("Failed to watch directory %s: %v", dir, err)
				continue
			}
			watchedDirs[dir] = true
		}

		fileSet[absPath] = true
		log.Printf("Watching %s for changes", absPath)
	}

	debounceTimers := make(map[string]*time.Timer)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			absPath, err := filepath.Abs(event.Name)
			if err != nil {
				continue
			}

			if !fileSet[absPath] {
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if timer, exists := debounceTimers[absPath]; exists {
					timer.Stop()
				}

				debounceTimers[absPath] = time.AfterFunc(500*time.Millisecond, func() {
					log.Printf("File changed: %s", absPath)
					onChange(absPath)
				})
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error: %v", err)

		case <-ctx.Done():
			for _, timer := range debounceTimers {
				timer.Stop()
			}
			return ctx.Err()
		}
	}
}
