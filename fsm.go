package fsm

import (
	"context"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"strings"
)

const UndefinedState = "undefined"

// NoChatIdError Returned when FSM was not able to get chat id either from Message or CallbackQuery.
type NoChatIdError struct {
	*tgbotapi.Update
}

func (e *NoChatIdError) Error() string {
	return fmt.Sprintf("no chat id in update: %+v", e.Update)
}

// DeleteKeyboardError Returned when some error happened during "remove keyboard" temp message sending attempt.
type DeleteKeyboardError struct {
	Err error
}

func (e *DeleteKeyboardError) Error() string {
	return fmt.Sprintf("failed to delete keyboard: %s", e.Err)
}

func (e *DeleteKeyboardError) Unwrap() error {
	return e.Err
}

// LoadStateError Error wrapper for load state handler error.
type LoadStateError struct {
	Err error
}

func (e *LoadStateError) Error() string {
	return fmt.Sprintf("loading state error: %s", e.Err)
}

func (e *LoadStateError) Unwrap() error {
	return e.Err
}

// SaveStateError Error wrapper for save state handler error.
type SaveStateError struct {
	Err error
}

func (e *SaveStateError) Error() string {
	return fmt.Sprintf("saving state error: %s", e.Err)
}

func (e *SaveStateError) Unwrap() error {
	return e.Err
}

// CurrentStateConfigNotFoundError Returned when load state handler returned state name that doesn't exist in current state configuration.
type CurrentStateConfigNotFoundError struct {
	Name string
}

func (e *CurrentStateConfigNotFoundError) Error() string {
	return fmt.Sprintf("current state %s config not found", e.Name)
}

// NextStateConfigNotFoundError Returned on attempt to perform transition to non-existing state.
type NextStateConfigNotFoundError struct {
	Name string
}

func (e *NextStateConfigNotFoundError) Error() string {
	return fmt.Sprintf("next state %s config not found", e.Name)
}

// Additional FSM options.
type botFsmOpts[T any] struct {
	// Map key is a command without "/" prefix.
	commands map[string]TransitionFn[T]
	// Determine bot reaction on non-existing command.
	undefinedCommandMessageFn MessageFn[T]
	// Load handler restores current state name and data by chat id from persistent storage.
	loadStateFn LoadStateFn[T]
	// Save handler puts current state name and data for given chat id into persistent storage.
	saveStateFn SaveStateFn[T]
	// This message will be sent along with RemoveKeyboard request. It will be removed right after that. But user
	// might see this message for a second.
	removeKeyboardTempText string
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

func WithRemoveKeyboardTempText[T any](text string) BotFsmOptsFn[T] {
	return func(opts *botFsmOpts[T]) {
		opts.removeKeyboardTempText = text
	}
}

type BotFsm[T any] struct {
	bot *tgbotapi.BotAPI
	// States configuration. Key is a state name. It is used as a transition target.
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

// HandleUpdate processes tgbotapi Update and handle it according to given FSM config.
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
		return &CurrentStateConfigNotFoundError{name}
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
		return &NextStateConfigNotFoundError{newName}
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
		err = b.removeKeyboard(chatId)
		if err != nil {
			return err
		}
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

// GoTo forces chat transition to a specific state. This function is useful when you need to trigger some notifications,
// or start a new scenario.
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

	if messageConfig.RemoveKeyboard {
		err = b.removeKeyboard(chatId)
		if err != nil {
			return err
		}
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

func (b *BotFsm[T]) removeKeyboard(chatId int64) error {
	msg := tgbotapi.NewMessage(chatId, b.removeKeyboardTempText)
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
	msgSent, err := b.bot.Send(msg)
	if err != nil {
		return &DeleteKeyboardError{err}
	}
	deleteMsg := tgbotapi.NewDeleteMessage(msgSent.Chat.ID, msgSent.MessageID)
	// This method always returns an error because of invalid bool to Message conversion.
	b.bot.Send(deleteMsg)
	return nil
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
