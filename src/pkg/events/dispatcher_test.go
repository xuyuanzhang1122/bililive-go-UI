package events

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAddAndRemoveEventListener(t *testing.T) {
	d := NewDispatcher(context.Background()).(*dispatcher)
	l := NewEventListener(func(event *Event) {})
	d.AddEventListener("test", l)
	d.AddEventListener("test2", NewEventListener(func(event *Event) {}))
	ls, ok := d.saver["test"]
	assert.True(t, ok)
	assert.Equal(t, l, ls.Front().Value)
	d.RemoveEventListener("test", l)
	_, ok = d.saver["test"]
	assert.False(t, ok)
	d.RemoveAllEventListener("test2")
	assert.Empty(t, d.saver)
}

func TestDispatchEvent(t *testing.T) {
	received := make(chan int, 4)
	d := NewDispatcher(context.Background()).(*dispatcher)
	d.AddEventListener("test", NewEventListener(func(event *Event) {
		received <- 0
	}))
	d.AddEventListener("test", NewEventListener(func(event *Event) {
		received <- 1
	}))
	d.AddEventListener("test", NewEventListener(func(event *Event) {
		received <- 2
	}))
	d.AddEventListener("test", NewEventListener(func(event *Event) {
		received <- 3
	}))
	d.DispatchEvent(NewEvent("test", nil))
	l := make([]int, 0, 4)
	for len(l) < 4 {
		select {
		case value := <-received:
			l = append(l, value)
		case <-time.After(time.Second):
			t.Fatal("事件回调未完成")
		}
	}
	assert.Equal(t, []int{0, 1, 2, 3}, l)
}
