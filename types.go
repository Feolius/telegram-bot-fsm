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

type StateConfig[T any] struct {
	TransitionFn        func(ctx context.Context, update *tgbotapi.Update, data *T) Transition
	MessageFn           func(ctx context.Context, data T) MessageConfig
	RemoveKeyboardAfter bool
	CleanupData         bool
}
