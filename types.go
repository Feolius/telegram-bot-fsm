package fsm

import (
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type MessageConfig struct {
	Text           string
	ReplyMarkup    interface{}
	ParseMode      string
	ExtraTexts     []string
	RemoveKeyboard bool
}

type Transition struct {
	Target string
	MessageConfig
}

func TargetTransition(target string) Transition {
	return Transition{
		Target: target,
	}
}

func TextTransition(text string) Transition {
	return Transition{
		MessageConfig: MessageConfig{
			Text: text,
		},
	}
}

type TransitionFn[T any] func(ctx context.Context, update *tgbotapi.Update, data T) (Transition, T)
type MessageFn[T any] func(ctx context.Context, data T) MessageConfig

type StateConfig[T any] struct {
	TransitionFn        TransitionFn[T]
	MessageFn           MessageFn[T]
	RemoveKeyboardAfter bool
	CleanupData         bool
}

type LoadStateFn[T any] func(ctx context.Context, chatId int64) (name string, data T, err error)
type SaveStateFn[T any] func(ctx context.Context, chatId int64, name string, data T) error
