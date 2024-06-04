# Golang Telegram Bot Finite State Machine

It's a wrapper around [Telegram Bot API Bindings](https://github.com/go-telegram-bot-api/telegram-bot-api). This 
library provides useful scaffolds to build bots which functionality is based on direct messages (DM) communication. 
It is not suitable for telegram groups bots. 

DM bot communication resembles finite state machine (fsm): every user 
interaction may either transit bot into another state or leave state the same (transit to the same state). Every 
transition is accompanied by a certain message from the bot. Moreover, to make any state switching more useful 
and meaningful, each transition may change underlying payload data. This library attempts to express this conception.

## Getting started
An FSM requires at least one state defined. For Telegram bot FSM it must be UndefinedState. This state is used 
implicitly when user sends a message first time or if user sends not supported command. 
But it can be used explicitly in your code.

A simple (and pointless) echo-bot example that uses a single UndefinedState FSM:
```go
package main

import (
	"context"
	fsm "github.com/Feolius/telegram-bot-fsm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"net/http"
	"os"
)

type Data struct{}

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

	// Declare bot commands configuration. It is optional.
	commands := make(map[string]fsm.TransitionFn[Data])
	commands["start"] = func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
		// "/start" command simply transits bot into UndefinedState.
		return fsm.TargetTransition(fsm.UndefinedState), data
	}

	// Declares FSM state configuration. Map key is a state name.
	configs := make(map[string]fsm.StateConfig[Data])
	// UndefinedState is a specific state, and it is required to be provided. 
	// And this is the only state we need for this bot.
	configs[fsm.UndefinedState] = fsm.StateConfig[Data]{
		// Default state message configuration will be returned in this handler.
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			return fsm.MessageConfig{
				// This message will be shown when /start command is used.
				Text: "Type any message and it will be sent back to you",
			}
		},
		// This is where we declare the main logic.
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			if update.Message == nil || update.Message.Text == "" {
				return fsm.TextTransition("This message should never be sent"), data
			}
			return fsm.TextTransition(update.Message.Text), data
		},
	}

	botFsm := fsm.NewBotFsm(bot, configs, fsm.WithCommands[Data](commands))

	ctx := context.TODO()
	for update := range updates {
		err = botFsm.HandleUpdate(ctx, &update)
		// Some error handling here...
	}
}
```

It is a primitive bot that can do 2 things: respond with a description message on `/start` command and send any text
message back to user. Using this package for echo-bot seems to be overkill, but it shows the main concept. 

You can find more examples in the [examples](examples) folder.

## FSM configuration

FSM configuration is a generic and has the following `map[string]fsm.StateConfig[T]` type. The string key in this map 
is a name of state. `T` is a type of payload data you will operate during state transitions. 

`fsm.StateConfig` is defined as follows
```go
type MessageFn[T any] func(ctx context.Context, data T) MessageConfig
type TransitionFn[T any] func(ctx context.Context, update *tgbotapi.Update, data T) (Transition, T)

type StateConfig[T any] struct {
    MessageFn MessageFn[T]
    TransitionFn TransitionFn[T]
    RemoveKeyboardAfter bool
}
```

`MessageFn` provides a special `MessageConfig` definition used to create the telegram message that will be sent 
when switching to this state.

```go
type MessageConfig struct {
	// Message text. If it is empty, MessageConfig is considered to be empty as well.
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
```

`TransitionFn` is a core of FSM state logic. Here you will define all state switching rules and data changes. 
That's why `TransitionFn` returns `Transition` object and data. `Transition` object defines next state bot must 
switch to. And returned payload data will be passed as an argument for both next state 
`MessageFn` and `TransitionFn`.

```go
const AddTaskNameState = "add-task-name"
const AddTaskDescriptionState = "add-task-description"

type Task struct {
    name        string
    description string
}

type Data struct {
    newTask Task
}

configs[AddTaskNameState] = fsm.StateConfig[Data]{
	MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
		// This message will be shown, when moving to AddTaskNameState.
        return fsm.MessageConfig{
            Text: "Enter task name",
        }
    },
	TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
		if update.Message == nil {
			// User did smth wrong. We keep it in the same state, and sends corresponding message.
			return fsm.TextTransition("Something is wrong. Please specify task name"), data
		}
		// Update the task name here.
		data.newTask.name = update.Message.Text
        // And switch to the next state. AddTaskDescriptionState TransitionFn and MessageFn will get updated data.
		return fsm.TargetTransition(AddTaskDescriptionState), data
	},
}
```

`Transition` is the following structure

```go
type Transition struct {
	Target string
	MessageConfig
}
```

Here `Target` is a name of the state (that string key in the config map) you want to transit to. Besides target 
state you can also define bot transition message as `MessageConfig`. Both `Target` and `MessageConfig` are optional.
If `Target` is empty bot stays in the current state. If `MessageConfig` is empty, target `MessageFn` will be called 
to get message information. If `MessageConfig` is not empty, target `MessageFn` won't be called.

`fsm.TargetTransition(target string)` and `fsm.TextTransition(text string)` are 2 helper factories that create 
`Transition` with `Target` and `MessageConfig.Text` correspondingly. 

Thus, the simplified pipeline looks like this
![alt text](docs/pipeline.png "pipeline")

## Commands

Commands is a very popular way to interact with bots. You can define commands handlers using the following 
configuration `map[string]fsm.TransitionFn[T]`. Here the map key is a command _without_ "/" prefix. 
`T` type is payload data type. 

```go
commands := make(map[string]fsm.TransitionFn[Data])
commands["start"] = func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	// "/start" command simply transits bot into "menu".
	return fsm.TargetTransition("menu"), data
}
commands["faq"] = func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
    // "/faq" command returns some FAQ text.
    return fsm.TextTransition("Some faq text here"), data
}

// Pass commands via fsm.WithCommands wrapper.
botFsm := fsm.NewBotFsm(bot, configs, fsm.WithCommands[Data](commands))
```

## State persistence 

`TransitionFn` may change payload data passed, and the next state `TransitionFn` will receive updated copy of the
payload data. That means data must be kept somewhere between user requests. By default, it is stored in memory. 
That means, all the data will be lost after the bot re-run. It's okey when you during development or for 
very simple bots, but most of the time you want data to be kept. In order to do that you should use 
persistence handlers.

```go
type LoadStateFn[T any] func(ctx context.Context, chatId int64) (name string, data T, err error)
type SaveStateFn[T any] func(ctx context.Context, chatId int64, name string, data T) error
```

LoadStateFn is declared to restore state name and data from persistent storage. SaveStateFn is used to save state 
name and data into persistent storage. Together these handlers provide the ability to manage "session" data between
requests. `name` return param denotes state name. If load handler returns empty `name` (e.g. when user , it is treated as 
UndefinedState. 

Persistence handlers can be provided using `fsm.WithPersistenceHandlers` option function

```go
botFsm := fsm.NewBotFsm(bot, configs, fsm.WithPersistenceHandlers[Data](loadStateHandler, saveStatehandler))
```

