package fsm

import (
	"context"
	"sync"
)

type ChatState[T any] struct {
	chatId int64
	state  State
	data   T
}

type chatStatesStore[T any] struct {
	mx sync.RWMutex
	m  map[int64]*ChatState[T]
}

func (s *chatStatesStore[T]) put(key int64, state *ChatState[T]) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.m[key] = state
}

func (s *chatStatesStore[T]) get(key int64) (*ChatState[T], bool) {
	s.mx.RLock()
	defer s.mx.RUnlock()
	val, ok := s.m[key]
	return val, ok
}

func (s *chatStatesStore[T]) delete(key int64) { //nolint:unused // will be used later
	s.mx.Lock()
	defer s.mx.Unlock()
	delete(s.m, key)
}

type pseudoPersistenceHandler[T any] struct {
	*chatStatesStore[T]
}

func (h *pseudoPersistenceHandler[T]) LoadStateFn(ctx context.Context, chatId int64) (state State, data T, err error) {
	if chatState, ok := h.get(chatId); ok {
		state = chatState.state
		data = chatState.data
	}
	return state, data, nil
}

func (h *pseudoPersistenceHandler[T]) SaveStateFn(ctx context.Context, chatId int64, state State, data T) error {
	chatState := &ChatState[T]{
		chatId: chatId,
		state:  state,
		data:   data,
	}
	h.put(chatId, chatState)
	return nil
}

func getDefaultOpts[T any]() botFsmOpts[T] {
	store := &chatStatesStore[T]{
		m: make(map[int64]*ChatState[T]),
	}
	return botFsmOpts[T]{
		PersistenceHandler:     &pseudoPersistenceHandler[T]{store},
		removeKeyboardTempText: "Thinking...",
	}
}
