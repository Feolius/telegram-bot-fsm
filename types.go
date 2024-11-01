package fsm

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type State = string

// MessageConfig contains bot message information.
type MessageConfig struct {
	// ChatId field is ignored.
	tgbotapi.MessageConfig
	// These messages are sent right after the main one.
	// Note: params from embedded MessageConfig (e.g. ReplyMarkup or ParseMode) will be applied for all of them.
	ExtraTexts []string
	// If true, it will send and remove RemoveKeyboard message prior to main message sending.
	RemoveKeyboard bool
}

func TextMessageConfig(text string) MessageConfig {
	messageConfig := MessageConfig{}
	messageConfig.Text = text
	return messageConfig
}

func (m MessageConfig) Empty() bool {
	return m.Text == ""
}

// Transition describes state switching rule.
type Transition struct {
	// Next state name. If it is empty, bot stays in the same state (e.g. when need to send validation error).
	State
	// Defines transition bot message. If MessageConfig Text field is empty, the next state MessageFn
	// will be called to get MessageConfig.
	MessageConfig
}

// StateTransition simplifies Transition object creation for state switches.
func StateTransition(state State) Transition {
	return Transition{State: state}
}

// TextTransition simplifies Transition object creation, when no state switch needed.
func TextTransition(text string) Transition {
	transition := Transition{}
	transition.Text = text
	return transition
}

// MessageFn serves for MessageConfig definition.
type MessageFn[T any] func(ctx context.Context, data T) MessageConfig

type TransitionProvider[T any] interface {
	// TransitionFn defines state switching logic.
	TransitionFn(ctx context.Context, update *tgbotapi.Update, data T) (Transition, T)
}

type MessageConfigProvider[T any] interface {
	// MessageFn serves for MessageConfig definition.
	MessageFn(ctx context.Context, data T) MessageConfig
}

type StateHandler[T any] interface {
	MessageConfigProvider[T]
	TransitionProvider[T]
}

type RemoveKeyboardAfterMarker interface {
	// RemoveKeyboardAfter gives a signal to remove keyboard after bot left the state.
	RemoveKeyboardAfter() bool
}

type RemoveKeyboardBeforeMarker interface {
	// RemoveKeyboardBefore gives a signal to remove keyboard before bot enters the state.
	RemoveKeyboardBefore() bool
}

type PersistenceHandler[T any] interface {
	LoadStateFn(ctx context.Context, chatId int64) (state State, data T, err error)
	SaveStateFn(ctx context.Context, chatId int64, state State, data T) error
}
