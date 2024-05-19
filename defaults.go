package fsm

import (
	"context"
	"sync"
)

type ChatState[T any] struct {
	chatId int64
	name   string
	data   T
}

type ChatStatesStore[T any] struct {
	mx sync.RWMutex
	m  map[int64]*ChatState[T]
}

func (s *ChatStatesStore[T]) put(key int64, state *ChatState[T]) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.m[key] = state
}

func (s *ChatStatesStore[T]) get(key int64) (*ChatState[T], bool) {
	s.mx.RLock()
	defer s.mx.RUnlock()
	val, ok := s.m[key]
	return val, ok
}

func (s *ChatStatesStore[T]) delete(key int64) {
	s.mx.Lock()
	defer s.mx.Unlock()
	delete(s.m, key)
}

func defaultPersistenceHandlers[T any]() (LoadStateFn[T], SaveStateFn[T]) {
	store := &ChatStatesStore[T]{
		m: make(map[int64]*ChatState[T]),
	}
	return func(ctx context.Context, chatId int64) (name string, data T, err error) {
			chatState, ok := store.get(chatId)
			if ok {
				name = chatState.name
				data = chatState.data
			}
			return
		}, func(ctx context.Context, chatId int64, name string, data T) error {
			chatState := &ChatState[T]{
				chatId: chatId,
				name:   name,
				data:   data,
			}
			store.put(chatId, chatState)
			return nil
		}
}

func getDefaultOpts[T any]() botFsmOpts[T] {
	loadStateFn, saveStateFn := defaultPersistenceHandlers[T]()
	return botFsmOpts[T]{
		loadStateFn:           loadStateFn,
		saveStateFn:           saveStateFn,
		removeKeyboardTempMsg: "Thinking...",
	}
}
