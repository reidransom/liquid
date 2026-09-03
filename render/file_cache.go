package render

import (
	"errors"
	"io/fs"
	"sync"

	"github.com/reidransom/liquid/parser"
)

type fileCacheKey struct {
	filename string
	location parser.SourceLoc
}

type fileCacheEntry struct {
	done chan struct{}
	root Node
	err  error
}

type fileCache struct {
	mu      sync.Mutex
	entries map[fileCacheKey]*fileCacheEntry
}

func newFileCache() *fileCache {
	return &fileCache{entries: make(map[fileCacheKey]*fileCacheEntry)}
}

func (c *fileCache) load(key fileCacheKey, load func() (Node, error)) (Node, error) {
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		c.mu.Unlock()
		<-entry.done
		return entry.root, entry.err
	}

	entry := &fileCacheEntry{done: make(chan struct{})}
	c.entries[key] = entry
	c.mu.Unlock()

	entry.root, entry.err = load()
	if entry.err != nil && !errors.Is(entry.err, fs.ErrNotExist) {
		c.mu.Lock()
		if c.entries[key] == entry {
			delete(c.entries, key)
		}
		c.mu.Unlock()
	}
	close(entry.done)

	return entry.root, entry.err
}
