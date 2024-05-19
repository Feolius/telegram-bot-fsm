package fsm

import (
	"context"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"strings"
)

const UndefinedState = "undefined"

type NoChatIdError struct {
	*tgbotapi.Update
}

func (e *NoChatIdError) Error() string {
	return fmt.Sprintf("no chat id in update: %+v", e.Update)
}

type LoadStateError struct {
	Err error
}

func (e *LoadStateError) Error() string {
	return fmt.Sprintf("loading state error: %s", e.Err)
}

func (e *LoadStateError) Unwrap() error {
	return e.Err
}

type SaveStateError struct {
	Err error
}

func (e *SaveStateError) Error() string {
	return fmt.Sprintf("saving state error: %s", e.Err)
}

func (e *SaveStateError) Unwrap() error {
	return e.Err
}

type botFsmOpts[T any] struct {
	commands                  map[string]TransitionFn[T]
	undefinedCommandMessageFn MessageFn[T]
	loadStateFn               LoadStateFn[T]
	saveStateFn               SaveStateFn[T]
	removeKeyboardTempMsg     string
}

type BotFsmOptsFn[T any] func(options *botFsmOpts[T])

func WithCommands[T any](commands map[string]TransitionFn[T]) BotFsmOptsFn[T] {
	return func(opts *botFsmOpts[T]) {
		opts.commands = commands
	}
}

func WithUnknownCommandMessageFn[T any](messageFn MessageFn[T]) BotFsmOptsFn[T] {
	return func(opts *botFsmOpts[T]) {
		opts.undefinedCommandMessageFn = messageFn
	}
}

func WithPersistenceHandlers[T any](loadStateFn LoadStateFn[T], saveStateFn SaveStateFn[T]) BotFsmOptsFn[T] {
	return func(opts *botFsmOpts[T]) {
		opts.loadStateFn = loadStateFn
		opts.saveStateFn = saveStateFn
	}
}

func WithRemoveKeyboardTempMsg[T any](removeKeyboardTempMsg string) BotFsmOptsFn[T] {
	return func(opts *botFsmOpts[T]) {
		opts.removeKeyboardTempMsg = removeKeyboardTempMsg
	}
}

type BotFsm[T any] struct {
	bot     *tgbotapi.BotAPI
	configs map[string]StateConfig[T]
	botFsmOpts[T]
}

func NewBotFsm[T any](bot *tgbotapi.BotAPI, configs map[string]StateConfig[T], optFns ...BotFsmOptsFn[T]) *BotFsm[T] {
	if _, ok := configs[UndefinedState]; !ok {
		panic("undefined state configuration must be provided")
	}

	for name, config := range configs {
		if config.TransitionFn == nil {
			panic(fmt.Sprintf("transition function is not provided for %s state", name))
		}
		if config.MessageFn == nil {
			panic(fmt.Sprintf("message function is not provided for %s state", name))
		}
	}

	opts := getDefaultOpts[T]()
	for _, optFn := range optFns {
		optFn(&opts)
	}

	return &BotFsm[T]{
		bot:        bot,
		configs:    configs,
		botFsmOpts: opts,
	}
}

func (b *BotFsm[T]) HandleUpdate(ctx context.Context, update *tgbotapi.Update) error {
	chatId := getChatId(update)
	if chatId == 0 {
		return &NoChatIdError{update}
	}

	name, data, err := b.resumeState(ctx, update)
	if err != nil {
		return err
	}

	command := extractCommand(update)
	if command != "" {
		name = UndefinedState
	}

	stateConfig, ok := b.configs[name]
	if !ok {
		// @TODO Replace with error
		panic(fmt.Sprintf("%s state configuration is not found", name))
	}

	var transition Transition
	newData := data
	if command != "" {
		commandTransitionFn, ok := b.commands[command]
		if ok {
			transition, newData = commandTransitionFn(ctx, update, data)
		} else {
			transition = Transition{}
		}
	} else {
		transition, newData = stateConfig.TransitionFn(ctx, update, data)
	}

	newName := transition.Target
	if newName == "" {
		newName = name
	}

	messageConfig := transition.MessageConfig
	newStateConfig, ok := b.configs[newName]
	if !ok {
		// @TODO Replace with error
		panic(fmt.Sprintf("%s state configuration is not found", newName))
	}
	if isEmptyMessageConfig(messageConfig) {
		messageFn := newStateConfig.MessageFn
		if command != "" && transition.Target == "" && b.undefinedCommandMessageFn != nil {
			// Command doesn't exist
			messageFn = b.undefinedCommandMessageFn
		}
		messageConfig = messageFn(ctx, newData)
	}

	if stateConfig.RemoveKeyboardAfter || messageConfig.RemoveKeyboard {
		b.removeKeyboard(chatId)
	}

	err = b.saveStateFn(ctx, chatId, newName, newData)
	if err != nil {
		return fmt.Errorf("error in attempt to save a new state: %w", err)
	}

	for _, msgConfig := range b.getStateMessageConfigs(chatId, messageConfig) {
		_, err = b.bot.Send(msgConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *BotFsm[T]) GoTo(ctx context.Context, chatId int64, transition Transition, data T) error {
	if transition.Target == "" {
		panic("transition target is required")
	}

	newStateConfig, ok := b.configs[transition.Target]
	if !ok {
		panic(fmt.Sprintf("%s state configuration is not found", transition.Target))
	}
	messageConfig := transition.MessageConfig
	if isEmptyMessageConfig(messageConfig) {
		messageConfig = newStateConfig.MessageFn(ctx, data)
	}

	err := b.saveStateFn(ctx, chatId, transition.Target, data)
	if err != nil {
		return &SaveStateError{err}
	}

	for _, msgConfig := range b.getStateMessageConfigs(chatId, messageConfig) {
		_, err = b.bot.Send(msgConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *BotFsm[T]) resumeState(ctx context.Context, update *tgbotapi.Update) (string, T, error) {
	chatId := getChatId(update)

	emptyData := b.getZeroData()
	if chatId == 0 {
		return "", emptyData, &NoChatIdError{update}
	}

	name, data, err := b.loadStateFn(ctx, chatId)
	if err != nil {
		return "", emptyData, &LoadStateError{err}
	}
	if name == "" {
		name = UndefinedState
	}

	return name, data, nil
}

func (b *BotFsm[T]) getZeroData() T {
	var data T
	return data
}

func isEmptyMessageConfig(config MessageConfig) bool {
	return config.Text == ""
}

func getChatId(update *tgbotapi.Update) int64 {
	var chatId int64
	if update.Message != nil && update.Message.Chat != nil {
		chatId = update.Message.Chat.ID
	}

	if chatId == 0 && update.CallbackQuery != nil && update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Chat != nil {
		chatId = update.CallbackQuery.Message.Chat.ID
	}
	return chatId
}

func (b *BotFsm[T]) removeKeyboard(chatId int64) {
	msg := tgbotapi.NewMessage(chatId, b.removeKeyboardTempMsg)
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
	msgSent, err := b.bot.Send(msg)
	if err != nil {
		// @TODO remove log
		log.Printf("error in attempt to send hide-keyboard message: %s", err)
	}
	deleteMsg := tgbotapi.NewDeleteMessage(msgSent.Chat.ID, msgSent.MessageID)
	_, err = b.bot.Send(deleteMsg)
	if err != nil {
		// @TODO remove log
		log.Printf("error in attempt to delete hide-keyboard message: %s", err)
	}
}

func (b *BotFsm[T]) getStateMessageConfigs(chatId int64, messageConfig MessageConfig) []tgbotapi.MessageConfig {
	msg := tgbotapi.NewMessage(chatId, messageConfig.Text)
	msg.ParseMode = messageConfig.ParseMode
	if messageConfig.ReplyMarkup != nil {
		msg.ReplyMarkup = messageConfig.ReplyMarkup
	}
	res := make([]tgbotapi.MessageConfig, 0, 1)
	res = append(res, msg)
	for _, extraText := range messageConfig.ExtraTexts {
		extraMsg := tgbotapi.NewMessage(chatId, extraText)
		extraMsg.ParseMode = msg.ParseMode
		extraMsg.ReplyMarkup = msg.ReplyMarkup
		res = append(res, extraMsg)
	}
	return res
}

func extractCommand(update *tgbotapi.Update) string {
	if update.Message != nil && strings.HasPrefix(update.Message.Text, "/") &&
		len(strings.Split(update.Message.Text, " ")) == 1 {
		return strings.TrimPrefix(update.Message.Text, "/")
	}
	return ""
}
