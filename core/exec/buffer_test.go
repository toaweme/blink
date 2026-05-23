package exec

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Buffer_AppendAndCount(t *testing.T) {
	b := NewBuffer()
	assert.Equal(t, 0, b.Count())

	b.Append("a")
	b.Append("b")
	b.Append("c")

	assert.Equal(t, 3, b.Count())
	assert.Equal(t, []string{"a", "b", "c"}, b.Full())
}

func Test_Buffer_Tail(t *testing.T) {
	b := NewBuffer()
	for _, s := range []string{"1", "2", "3", "4", "5"} {
		b.Append(s)
	}

	assert.Equal(t, []string{"4", "5"}, b.Tail(2))
	assert.Equal(t, []string{"1", "2", "3", "4", "5"}, b.Tail(10))
	assert.Empty(t, b.Tail(0))
}

func Test_Buffer_RangeClamping(t *testing.T) {
	b := NewBuffer()
	for _, s := range []string{"a", "b", "c"} {
		b.Append(s)
	}

	assert.Equal(t, []string{"a", "b"}, b.Range(0, 2))
	assert.Equal(t, []string{"b", "c"}, b.Range(1, 99))
	assert.Empty(t, b.Range(5, 7))
	assert.Empty(t, b.Range(2, 1))
}

func Test_Buffer_FullReturnsCopy(t *testing.T) {
	b := NewBuffer()
	b.Append("a")
	full := b.Full()
	full[0] = "MUTATED"

	assert.Equal(t, "a", b.Full()[0])
}

func Test_Buffer_ConcurrentAppend(t *testing.T) {
	b := NewBuffer()
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				b.Append("x")
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 1000, b.Count())
}

func Test_Buffer_TailStreamReceivesAppends(t *testing.T) {
	b := NewBuffer()
	b.Append("seed")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := b.TailStream(ctx, 0)

	got := []string{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for line := range ch {
			got = append(got, line)
			if len(got) == 3 {
				return
			}
		}
	}()

	b.Append("a")
	b.Append("b")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for TailStream lines")
	}

	require.Len(t, got, 3)
	assert.Equal(t, []string{"seed", "a", "b"}, got)
}
