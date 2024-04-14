package fsm

import (
	"context"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"strings"
)

const CommandHandlerState = "command-handler"
const UndefinedState = "undefined"

type NoChatIdError struct {
	*tgbotapi.Update
}

func (e *NoChatIdError) Error() string {
	return fmt.Sprintf("no chat id in update: %+v", e.Update)
}

type LoadStateFn[T any] func(ctx context.Context, chatId int64) (name string, data T, err error)
type SaveStateFn[T any] func(ctx context.Context, chatId int64, name string, data T) error

type botFsmOpts[T any] struct {
	loadStateFn           LoadStateFn[T]
	saveStateFn           SaveStateFn[T]
	removeKeyboardTempMsg string
}

type BotFsmOptsFn[T any] func(options *botFsmOpts[T])

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
	if _, ok := configs[CommandHandlerState]; !ok {
		panic("command handler state configuration must be provided")
	}
	if _, ok := configs[UndefinedState]; !ok {
		panic("undefined state configuration must be provided")
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

	stateConfig, ok := b.configs[name]
	if !ok {
		panic(fmt.Sprintf("%s state configuration is not found", name))
	}

	transition := stateConfig.TransitionFn(ctx, update, &data)
	newName := transition.Target
	if newName == "" {
		newName = name
	}

	messageConfig := transition.MessageConfig
	newStateConfig, ok := b.configs[newName]
	if !ok {
		panic(fmt.Sprintf("%s state configuration is not found", newName))
	}
	if newStateConfig.CleanupData {
		data = b.getZeroData()
	}
	if messageConfig.Text == "" {
		messageConfig = newStateConfig.MessageFn(ctx, data)
	}

	if stateConfig.RemoveKeyboardAfter || messageConfig.RemoveKeyboard {
		b.removeKeyboard(chatId)
	}

	err = b.saveStateFn(ctx, chatId, newName, data)
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

func (b *BotFsm[T]) GoTo(ctx context.Context, chatId int64, transition Transition) error {
	name, data, err := b.loadStateFn(ctx, chatId)
	if err != nil {
		return err
	}

	newName := transition.Target
	if newName == "" {
		newName = name
	}

	newStateConfig, ok := b.configs[newName]
	if !ok {
		panic(fmt.Sprintf("%s state configuration is not found", newName))
	}
	if newStateConfig.CleanupData {
		data = b.getZeroData()
	}
	messageConfig := transition.MessageConfig
	if messageConfig.Text == "" {
		messageConfig = newStateConfig.MessageFn(ctx, data)
	}

	err = b.saveStateFn(ctx, chatId, newName, data)
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

func (b *BotFsm[T]) resumeState(ctx context.Context, update *tgbotapi.Update) (string, T, error) {
	chatId := getChatId(update)

	emptyData := b.getZeroData()
	if chatId == 0 {
		return "", emptyData, &NoChatIdError{update}
	}

	var text string
	if update.Message != nil {
		text = update.Message.Text
	}

	if strings.HasPrefix(text, "/") {
		return CommandHandlerState, emptyData, nil
	}

	name, data, err := b.loadStateFn(ctx, chatId)
	if err != nil {
		return "", emptyData, err
	}
	return name, data, nil

}

func (b *BotFsm[T]) getZeroData() T {
	var data T
	return data
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
		log.Printf("error in attempt to send hide-keyboard message: %s", err)
	}
	deleteMsg := tgbotapi.NewDeleteMessage(msgSent.Chat.ID, msgSent.MessageID)
	_, err = b.bot.Send(deleteMsg)
	if err != nil {
		log.Printf("error in attempt to delete hide-keyboard message: %s", err)
	}
}

func (b *BotFsm[T]) getStateMessageConfigs(chatId int64, payload MessageConfig) []tgbotapi.MessageConfig {
	msg := tgbotapi.NewMessage(chatId, payload.Text)
	msg.ParseMode = payload.ParseMode
	if payload.ReplyMarkup != nil {
		msg.ReplyMarkup = payload.ReplyMarkup
	}
	res := make([]tgbotapi.MessageConfig, 0, 1)
	res = append(res, msg)
	for _, extraText := range payload.ExtraTexts {
		extraMsg := tgbotapi.NewMessage(chatId, extraText)
		extraMsg.ParseMode = msg.ParseMode
		extraMsg.ReplyMarkup = msg.ReplyMarkup
		res = append(res, extraMsg)
	}
	return res
}
