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

	configs := getFsmConfigs()

	commands := make(map[string]fsm.TransitionFn[Data])
	commands["start"] = func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
		// "/start" command simply transits bot into MenuState.
		return fsm.TargetTransition(MenuState), data
	}

	botFsm := fsm.NewBotFsm(bot, configs, fsm.WithCommands[Data](commands))

	ctx := context.TODO()
	for update := range updates {
		err = botFsm.HandleUpdate(ctx, &update)
	}
}

func getFsmConfigs() map[string]fsm.StateConfig[Data] {
	configs := make(map[string]fsm.StateConfig[Data])
	configs[MenuState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			// Show menu keyboard along with the message text.
			return fsm.MessageConfig{
				Text:        "Choose what you want to do",
				ReplyMarkup: getMenuButtons(),
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			if update.Message == nil {
				// That's not expected type of interaction. Ask user to use keyboard again with a different message.
				return fsm.Transition{
					MessageConfig: fsm.MessageConfig{
						Text:        "Please use one of menu buttons available",
						ReplyMarkup: getMenuButtons(),
					},
				}, data
			}
			switch update.Message.Text {
			case AddTaskKeyword:
				// Start creating a new task.
				data.newTask = Task{}
				return fsm.TargetTransition(AddTaskNameState), data
			case ListTasksKeyword:
				texts := tasksToTexts(data.tasks)
				if len(texts) == 0 {
					return fsm.Transition{
						Target: fsm.UndefinedState,
						MessageConfig: fsm.MessageConfig{
							Text: "You don't have any tasks",
						},
					}, data
				}
				// This is an example of sending several messages within a single response. Each task is sent as a
				// separate message here. We don't expect any further interaction, that's why UndefinedState  is a good
				// choice to send informational messages.
				return fsm.Transition{
					Target: fsm.UndefinedState,
					MessageConfig: fsm.MessageConfig{
						Text:       texts[0],
						ExtraTexts: texts[1:],
						ParseMode:  "MarkdownV2",
					},
				}, data
			case DeleteTaskKeyword:
				if len(data.tasks) == 0 {
					return fsm.Transition{
						Target: fsm.UndefinedState,
						MessageConfig: fsm.MessageConfig{
							Text: "You don't have any tasks",
						},
					}, data
				}
				return fsm.TargetTransition(DeleteTaskChoiceState), data
			default:
				return fsm.Transition{
					// User didn't use any button. Ask user to use keyboard again with a different message.
					MessageConfig: fsm.MessageConfig{
						Text:        "Please use one of menu buttons available",
						ReplyMarkup: getMenuButtons(),
					},
				}, data
			}
		},
		RemoveKeyboardAfter: true,
	}
	configs[AddTaskNameState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			return fsm.MessageConfig{
				Text: "Enter task name",
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			if update.Message == nil {
				return fsm.TextTransition("Please specify task name"), data
			}
			// A new task name is populated with the user response.
			data.newTask.name = update.Message.Text
			return fsm.TargetTransition(AddTaskDescriptionState), data
		},
	}
	configs[AddTaskDescriptionState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			return fsm.MessageConfig{
				Text: "Enter task description",
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			if update.Message == nil {
				return fsm.TextTransition("Please specify task description"), data
			}
			data.newTask.description = update.Message.Text
			return fsm.TargetTransition(AddTaskPriorityState), data
		},
	}
	configs[AddTaskPriorityState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			// User must pick one of the following inline variants.
			keyboardRows := make([][]tgbotapi.InlineKeyboardButton, 3)
			keyboardRows[0] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Low", strconv.Itoa(int(LowPriority))))
			keyboardRows[1] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Normal", strconv.Itoa(int(NormalPriority))))
			keyboardRows[2] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("High", strconv.Itoa(int(HighPriority))))
			return fsm.MessageConfig{
				Text:        "Choose priority level",
				ReplyMarkup: tgbotapi.NewInlineKeyboardMarkup(keyboardRows...),
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
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
			return fsm.Transition{
				Target: fsm.UndefinedState,
				MessageConfig: fsm.MessageConfig{
					Text: "New task added",
				},
			}, data
		},
	}
	configs[DeleteTaskChoiceState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			// Another inline keyboard example.
			keyboardRows := make([][]tgbotapi.InlineKeyboardButton, len(data.tasks))
			for i, task := range data.tasks {
				keyboardRows[i] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf("%s ❌", task.name),
					strconv.Itoa(i),
				))
			}
			return fsm.MessageConfig{
				Text:        "Delete task",
				ReplyMarkup: tgbotapi.NewInlineKeyboardMarkup(keyboardRows...),
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			if update.CallbackQuery == nil {
				return fsm.TextTransition("Please pick task to remove"), data
			}
			i, err := strconv.Atoi(update.CallbackQuery.Data)
			if err != nil || i >= len(data.tasks) || i < 0 {
				// Callback query was sent, but something is completely wrong. Probably it was unrelated callback query.
				// Cannot handle it within this state, so just show error message.
				log.Printf("cannot convert callback query to int: %s", err)
				return fsm.Transition{
					Target: fsm.UndefinedState,
					MessageConfig: fsm.MessageConfig{
						Text: "Something went wrong, please try again",
					},
				}, data
			}
			data.tasks = append(data.tasks[:i], data.tasks[i+1:]...)
			return fsm.Transition{
				Target: fsm.UndefinedState,
				MessageConfig: fsm.MessageConfig{
					Text: "Task removed",
				},
			}, data
		},
	}
	// UndefinedState must be provided.
	configs[fsm.UndefinedState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			// It can be used for some general information.
			return fsm.MessageConfig{
				Text: "Use /start command to get into main menu",
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			return fsm.Transition{}, data
		},
	}
	return configs
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
