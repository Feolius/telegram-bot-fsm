package fsm

import (
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MessageConfig contains bot message information.
type MessageConfig struct {
	// Message text.
	Text string
	// It is used as tgbotapi BaseChat ReplyMarkup field value.
	ReplyMarkup interface{}
	// It is used as tgbotapi MessageConfig ParseMode field value, e.g. "MarkdownV2" or "HTML".
	ParseMode string
	// These messages are sent right after the main one. Note: ReplyMarkup and ParseMode are applied for all of them.
	ExtraTexts []string
	// If true, it will send and remove RemoveKeyboard message prior to main message sending.
	RemoveKeyboard bool
}

// Transition describes state switching rule.
type Transition struct {
	// Next state name. If it is empty, bot stays in the same state (e.g. when need to send validation error).
	Target string
	// Defines transition bot message. If MessageConfig Text field is empty, corresponding Target state MessageFn
	// will be called to get MessageConfig.
	MessageConfig
}

// TargetTransition simplifies Transition object creation for state switches.
func TargetTransition(target string) Transition {
	return Transition{
		Target: target,
	}
}

// TextTransition simplifies Transition object creation, when no state switch needed.
func TextTransition(text string) Transition {
	return Transition{
		MessageConfig: MessageConfig{
			Text: text,
		},
	}
}

// TransitionFn defines state switching logic.
type TransitionFn[T any] func(ctx context.Context, update *tgbotapi.Update, data T) (Transition, T)

// MessageFn serves for MessageConfig definition.
type MessageFn[T any] func(ctx context.Context, data T) MessageConfig

// StateConfig is a single state configuration.
type StateConfig[T any] struct {
	// If bot sits in the current state, this function will be called to determine Transition to the next state.
	TransitionFn TransitionFn[T]
	// If TransitionFn returns Transition with empty message Text (i.e. MessageConfig is empty), Target MessageFn will
	// be called to get MessageConfig.
	MessageFn MessageFn[T]
	// If set to true, RemoveMessage will be sent and removed before next state transition.
	RemoveKeyboardAfter bool
}

// LoadStateFn is declared to restore state name and data from persistent storage. External dependencies can be passed using closure.
type LoadStateFn[T any] func(ctx context.Context, chatId int64) (name string, data T, err error)

// SaveStateFn is declared to save state name and data into persistent storage. External dependencies can be passed using closure.
type SaveStateFn[T any] func(ctx context.Context, chatId int64, name string, data T) error
