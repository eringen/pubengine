package markdown

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"fmt"
	"sync"
)

const maxInput = 2 << 20
const maxOutput = 16 << 20
const cacheBudget = 16 << 20

type renderEntry struct {
	key  [32]byte
	body []byte
}

var renderedCache = struct {
	sync.Mutex
	items map[[32]byte]*list.Element
	order *list.List
	bytes int
}{items: map[[32]byte]*list.Element{}, order: list.New()}

type boundedOutput struct {
	bytes.Buffer
	err error
}

func (b *boundedOutput) WriteString(s string) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	if len(s) > maxOutput-b.Len() {
		b.err = fmt.Errorf("markdown output exceeds 16 MB")
		return 0, b.err
	}
	return b.Buffer.WriteString(s)
}
func cachedMarkdown(content string) ([]byte, error) {
	if len(content) > maxInput {
		return nil, fmt.Errorf("markdown input exceeds 2 MB")
	}
	key := sha256.Sum256([]byte(content))
	renderedCache.Lock()
	if entry := renderedCache.items[key]; entry != nil {
		renderedCache.order.MoveToFront(entry)
		body := entry.Value.(renderEntry).body
		renderedCache.Unlock()
		return body, nil
	}
	renderedCache.Unlock()
	var output boundedOutput
	renderMarkdown(&output, content)
	if output.err != nil {
		return nil, output.err
	}
	body := output.Bytes()
	if len(body) > 1<<20 {
		return body, nil
	}
	renderedCache.Lock()
	defer renderedCache.Unlock()
	if existing := renderedCache.items[key]; existing != nil {
		return existing.Value.(renderEntry).body, nil
	}
	for renderedCache.bytes+len(body) > cacheBudget || len(renderedCache.items) >= 256 {
		old := renderedCache.order.Back()
		entry := old.Value.(renderEntry)
		delete(renderedCache.items, entry.key)
		renderedCache.bytes -= len(entry.body)
		renderedCache.order.Remove(old)
	}
	// Retain the output length, not the buffer's spare capacity.
	body = bytes.Clone(body)
	renderedCache.bytes += len(body)
	renderedCache.items[key] = renderedCache.order.PushFront(renderEntry{key, body})
	return body, nil
}
