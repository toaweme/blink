package exec

import (
	"context"
	"sync"
)

type Buffer struct {
	mu    sync.RWMutex
	lines []string

	// cond is used to broadcast that a new line was appended
	cond *sync.Cond
}

// NewBuffer creates an empty Buffer with a condition variable.
func NewBuffer() *Buffer {
	lb := &Buffer{
		lines: make([]string, 0),
	}
	// cond requires a Lock; we can reuse lb.mu for that
	lb.cond = sync.NewCond(&lb.mu)
	return lb
}

// Append adds a new line to the log buffer and notifies waiters.
func (b *Buffer) Append(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)

	// Broadcast to all goroutines that might be waiting in TailStream
	b.cond.Broadcast()
}

// Count returns the total number of lines stored so far.
func (b *Buffer) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.lines)
}

// Full returns a copy of all lines in the log buffer.
func (b *Buffer) Full() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// Tail returns up to the last n lines, as a copy.
func (b *Buffer) Tail(n int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total := len(b.lines)
	if n > total {
		n = total
	}
	start := total - n
	out := make([]string, n)
	copy(out, b.lines[start:total])
	return out
}

// Range returns lines in [start, end), clamping to valid bounds.
func (b *Buffer) Range(start, end int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total := len(b.lines)
	if start < 0 {
		start = 0
	}
	if end > total {
		end = total
	}
	if start > end {
		start = end
	}
	out := make([]string, end-start)
	copy(out, b.lines[start:end])
	return out
}

// TailStream returns a channel that streams lines as they arrive,
// starting from line index `fromLine`. It blocks until new lines come in.
//
// You can cancel the streaming by canceling the provided context.
//
// Example usage:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	ch := logBuffer.TailStream(ctx, 0)
//	for line := range ch {
//	    fmt.Println("New line:", line)
//	}
//	// call cancel() or break when you want to stop
func (b *Buffer) TailStream(ctx context.Context, fromLine int) <-chan string {
	outChan := make(chan string)

	go func() {
		defer close(outChan)

		b.mu.Lock()
		defer b.mu.Unlock()

		idx := fromLine
		for {
			// Emit any lines that have arrived since idx
			for idx < len(b.lines) {
				line := b.lines[idx]
				idx++
				// We must release the lock before sending to avoid blocking everything
				b.mu.Unlock()

				select {
				case outChan <- line:
					// Sent OK
				case <-ctx.Done():
					return
				}

				// Reacquire the lock to check lines again
				b.mu.Lock()
			}

			// If we reach here, we have no new lines to stream right now.
			// Wait for either new lines to arrive or context to be canceled.
			select {
			case <-ctx.Done():
				return
			default:
				// Wait until we get a cond.Broadcast() in Append
				b.cond.Wait()
			}
		}
	}()

	return outChan
}

// Pagination defines pagination metadata.
type Pagination struct {
	Page  int // current page (1-based)
	Total int // total items
	Limit int // items per page
}

// Paginate returns a slice of lines for the specified page/limit,
// and also returns a Pagination struct with metadata.
func (b *Buffer) Paginate(page, limit int) ([]string, Pagination) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Make sure page and limit are sensible
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 1
	}

	total := len(b.lines) // total lines available
	start := (page - 1) * limit
	if start > total {
		// If start is beyond total lines, return empty slice
		start = total
	}

	end := start + limit
	if end > total {
		end = total
	}

	// Copy out the slice so external code can safely manipulate it
	out := make([]string, end-start)
	copy(out, b.lines[start:end])

	// Build the pagination struct
	p := Pagination{
		Page:  page,
		Total: total,
		Limit: limit,
	}

	return out, p
}
