//go:build !codeanalysis

package main

import (
	"context"
	"fmt"
	fsm "github.com/Feolius/telegram-bot-fsm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const MenuState = "menu"
const AddTaskNameState = "add-task-name"
const AddTaskDescriptionState = "add-task-description"
const AddTaskPriorityState = "add-task-priority"
const DeleteTaskChoiceState = "delete-task-choice"

const AddTaskKeyword = "Add task"
const ListTasksKeyword = "See my tasks"
const DeleteTaskKeyword = "Delete task"

type Priority int

const (
	LowPriority Priority = iota
	NormalPriority
	HighPriority
)

type Task struct {
	name        string
	description string
	priority    Priority
}

type Data struct {
	tasks   []Task
	newTask Task
}

type StartCommandHandler struct{}

func (s StartCommandHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	// "/start" command simply transits bot into MenuState.
	return fsm.StateTransition(MenuState), data
}

type MenuStateHandler struct{}

func (h MenuStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	// Show menu keyboard along with the message text.
	messageConfig := fsm.MessageConfig{}
	messageConfig.Text = "Choose what you want to do"
	messageConfig.ReplyMarkup = getMenuButtons()
	return messageConfig
}

func (h MenuStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	transition := fsm.Transition{}
	if update.Message == nil {
		// That's not expected type of interaction. Ask user to use keyboard again with a different message.
		transition.Text = "Please use one of menu buttons available"
		transition.ReplyMarkup = getMenuButtons()
		return transition, data
	}
	switch update.Message.Text {
	case AddTaskKeyword:
		// Start creating a new task.
		data.newTask = Task{}
		return fsm.StateTransition(AddTaskNameState), data
	case ListTasksKeyword:
		texts := tasksToTexts(data.tasks)
		if len(texts) == 0 {
			transition.State = fsm.UndefinedState
			transition.Text = "You don't have any tasks"
			return transition, data
		}
		// This is an example of sending several messages within a single response. Each task is sent as a
		// separate message here. We don't expect any further interaction, that's why UndefinedState  is a good
		// choice to send informational messages.
		transition.State = fsm.UndefinedState
		transition.Text = texts[0]
		transition.ExtraTexts = texts[1:]
		transition.ParseMode = "MarkdownV2"
		return transition, data
	case DeleteTaskKeyword:
		if len(data.tasks) == 0 {
			transition.State = fsm.UndefinedState
			transition.Text = "You don't have any tasks to delete"
			return transition, data
		}
		return fsm.StateTransition(DeleteTaskChoiceState), data
	default:
		// User didn't use any button. Ask user to use keyboard again with a different message.
		transition.Text = "Please use one of menu buttons available"
		transition.ReplyMarkup = getMenuButtons()
		return transition, data
	}
}

// RemoveKeyboardAfter implementation controls whether fsm should remove keyboard when leaving this state.
func (h MenuStateHandler) RemoveKeyboardAfter() bool {
	return true
}

type AddTaskNameStateHandler struct{}

func (h AddTaskNameStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	return fsm.TextMessageConfig("Enter task name")
}

func (h AddTaskNameStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	if update.Message == nil {
		return fsm.TextTransition("Please specify task name"), data
	}
	// A new task name is populated with the user response.
	data.newTask.name = update.Message.Text
	return fsm.StateTransition(AddTaskDescriptionState), data
}

type AddTaskDescriptionStateHandler struct{}

func (h AddTaskDescriptionStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	return fsm.TextMessageConfig("Enter task description")
}

func (h AddTaskDescriptionStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	if update.Message == nil {
		return fsm.TextTransition("Please specify task description"), data
	}
	data.newTask.description = update.Message.Text
	return fsm.StateTransition(AddTaskPriorityState), data
}

type AddTaskPriorityStateHandler struct{}

func (h AddTaskPriorityStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	// User must pick one of the following inline variants.
	keyboardRows := make([][]tgbotapi.InlineKeyboardButton, 3)
	keyboardRows[0] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Low", strconv.Itoa(int(LowPriority))))
	keyboardRows[1] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Normal", strconv.Itoa(int(NormalPriority))))
	keyboardRows[2] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("High", strconv.Itoa(int(HighPriority))))

	messageConfig := fsm.MessageConfig{}
	messageConfig.Text = "Choose priority level"
	messageConfig.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
	return messageConfig
}

func (h AddTaskPriorityStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	if update.CallbackQuery == nil {
		return fsm.TextTransition("Please pick task priority"), data
	}
	switch update.CallbackQuery.Data {
	case "0":
		data.newTask.priority = LowPriority
	case "1":
		data.newTask.priority = NormalPriority
	case "2":
		data.newTask.priority = HighPriority
	default:
		return fsm.TextTransition("Please pick task priority"), data
	}
	data.tasks = append(data.tasks, data.newTask)
	target := fsm.Transition{}
	target.State = fsm.UndefinedState
	target.Text = "Task added"
	return target, data
}

type DeleteTaskChoiceStateHandler struct{}

func (h DeleteTaskChoiceStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	// Another inline keyboard example.
	keyboardRows := make([][]tgbotapi.InlineKeyboardButton, len(data.tasks))
	for i, task := range data.tasks {
		keyboardRows[i] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s ❌", task.name),
			strconv.Itoa(i),
		))
	}

	messageConfig := fsm.MessageConfig{}
	messageConfig.Text = "Pick task to remove"
	messageConfig.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
	return messageConfig
}

func (h DeleteTaskChoiceStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	if update.CallbackQuery == nil {
		return fsm.TextTransition("Please pick task to remove"), data
	}
	transition := fsm.Transition{}
	transition.State = fsm.UndefinedState
	i, err := strconv.Atoi(update.CallbackQuery.Data)
	if err != nil || i >= len(data.tasks) || i < 0 {
		// Callback query was sent, but something is completely wrong. Probably it was unrelated callback query.
		// Cannot handle it within this state, so just show error message.
		log.Printf("cannot convert callback query to int: %s", err)
		transition.Text = "Something went wrong, please try again"
		return transition, data
	}
	data.tasks = append(data.tasks[:i], data.tasks[i+1:]...)
	transition.Text = "Task removed"
	return transition, data
}

type UndefinedStateHandler struct{}

func (h UndefinedStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	// It can be used for some general information.
	return fsm.TextMessageConfig("Use /start command to get into main menu")
}

func (h UndefinedStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	return fsm.Transition{}, data
}

type UnknownCommandMessageProvider struct{}

func (u UnknownCommandMessageProvider) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	// This message will be shown, if user enters command which is not handled by this bot.
	return fsm.TextMessageConfig("Unknown command")
}

func main() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	updates := bot.ListenForWebhook("/" + bot.Token)
	go func() {
		log.Printf("serving port " + os.Getenv("PORT"))
		err = http.ListenAndServe("0.0.0.0:"+os.Getenv("PORT"), nil)
		if err != nil {
			log.Fatalf("cannot start server: %s", err)
		}
	}()

	configs := make(map[fsm.State]fsm.StateHandler[Data])
	configs[MenuState] = MenuStateHandler{}
	configs[AddTaskNameState] = AddTaskNameStateHandler{}
	configs[AddTaskDescriptionState] = AddTaskDescriptionStateHandler{}
	configs[AddTaskPriorityState] = AddTaskPriorityStateHandler{}
	configs[DeleteTaskChoiceState] = DeleteTaskChoiceStateHandler{}
	configs[fsm.UndefinedState] = UndefinedStateHandler{}

	commands := make(map[string]fsm.TransitionProvider[Data])
	commands["start"] = StartCommandHandler{}

	botFsm := fsm.NewBotFsm(
		bot,
		configs,
		fsm.WithCommands[Data](commands),
		fsm.WithUnknownCommandMessageConfigProvider[Data](UnknownCommandMessageProvider{}),
	)

	ctx := context.TODO()
	for update := range updates {
		err = botFsm.HandleUpdate(ctx, &update)
		if err != nil {
			log.Println(err)
		}
	}
}

func getMenuButtons() tgbotapi.ReplyKeyboardMarkup {
	keyboardButtonRows := make([][]tgbotapi.KeyboardButton, 3)
	keyboardButtonRows[0] = tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(AddTaskKeyword))
	keyboardButtonRows[1] = tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(ListTasksKeyword))
	keyboardButtonRows[2] = tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(DeleteTaskKeyword))
	return tgbotapi.NewOneTimeReplyKeyboard(keyboardButtonRows...)
}

func tasksToTexts(tasks []Task) []string {
	result := make([]string, len(tasks))
	for i, task := range tasks {
		priority := ""
		switch task.priority {
		case LowPriority:
			priority = "Low"
		case NormalPriority:
			priority = "Normal"
		case HighPriority:
			priority = "High"
		}
		result[i] = fmt.Sprintf("*%s* \n\n%s \n\nPriority: _%s_", MarkdownV2Replacer.Replace(task.name), MarkdownV2Replacer.Replace(task.description), priority)
	}
	return result
}

var MarkdownV2Replacer = strings.NewReplacer(
	"_", "\\_",
	"*", "\\*",
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"~", "\\~",
	"`", "\\`",
	">", "\\>",
	"#", "\\#",
	"+", "\\+",
	"-", "\\-",
	"=", "\\=",
	"|", "\\|",
	"{", "\\{",
	"}", "\\}",
	".", "\\.",
	"!", "\\!",
)
