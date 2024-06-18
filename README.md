# Golang Telegram Bot Finite State Machine

[![Go Reference](https://pkg.go.dev/badge/github.com/Feolius/telegram-bot-fsm.svg)](https://pkg.go.dev/github.com/Feolius/telegram-bot-fsm)

It's a wrapper around [Telegram Bot API Bindings](https://github.com/go-telegram-bot-api/telegram-bot-api). 
This library provides useful scaffolds to build bots whose functionality is based on direct message (DM) 
communication. It is not suitable for telegram group bots. 

DM bot communication resembles a finite state machine (fsm): every user interaction may either transit the bot 
into another state or leave the state the same (transit to the same state). Every transition is accompanied by 
a certain message from the bot. Moreover, to make any state switching more useful and meaningful, 
each transition may change underlying payload data. This library attempts to express this conception.

## Getting started
An FSM requires at least one state defined. For Telegram bot FSM it must be UndefinedState. This state is used 
implicitly when a user sends a message for the first time or if a user sends a not supported command. 
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

// Data is empty, because we don't need to pass any data between states for this bot.
type Data struct{}

type StartCommandHandler struct{}

func (h StartCommandHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	// "/start" command simply transits bot into UndefinedState.
	return fsm.StateTransition(fsm.UndefinedState), data
}

type UndefinedStateHandler struct{}

// MessageFn returns default state message configuration.
func (h UndefinedStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	// This message will be shown when /start command is used, because /start command transits bot into UndefinedState.
	return fsm.TextMessageConfig("Type any message and it will be sent back to you")
}

// TransitionFn describes how to switch to another states. In this case, we don't need to change state, because we have the only state.
func (h UndefinedStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	if update.Message == nil || update.Message.Text == "" {
		return fsm.TextTransition("This message should never be sent"), data
	}
	return fsm.TextTransition(update.Message.Text), data
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

	// Declare bot commands configuration. It is optional.
	commands := make(map[string]fsm.TransitionProvider[Data])
	commands["start"] = StartCommandHandler{}

	// Declare FSM state configuration. Map key is a state name.
	configs := make(map[string]fsm.StateHandler[Data])
	// UndefinedState is a specific state, and it is required to be provided.
	// And this is the only state we need for this bot.
	configs[fsm.UndefinedState] = UndefinedStateHandler{}

	botFsm := fsm.NewBotFsm(bot, configs, fsm.WithCommands[Data](commands))

	ctx := context.TODO()
	for update := range updates {
		err = botFsm.HandleUpdate(ctx, &update)
		// Some error handling here...
	}
}

```

It is a primitive bot that can do 2 things: respond with a description message on `/start` command and send any 
text message back to the user. Using this package for echo-bot seems to be overkill, but it shows the main concept.

You can find more examples in the [examples](examples) folder.

## FSM configuration

FSM configuration is a generic and has the following `map[string]fsm.StateHandler[T]` type. The string key in 
this map is the name of the state. `T` is a type of payload data you will operate during state transitions.

`fsm.StateHandler` is defined as follows
```go
type StateHandler[T any] interface {
    MessageConfigProvider[T]
    TransitionProvider[T]
}

type MessageConfigProvider[T any] interface {
    MessageFn(ctx context.Context, data T) MessageConfig
}

type TransitionProvider[T any] interface {
    TransitionFn(ctx context.Context, update *tgbotapi.Update, data T) (Transition, T)
}
```

`MessageConfigProvider` returns a special `MessageConfig` definition used to create the telegram message that 
will be sent when switching to this state.

```go
type MessageConfig struct {
    // ChatId field is ignored.
    tgbotapi.MessageConfig
    // These messages are sent right after the main one.
    // Note: params from embedded MessageConfig (e.g. ReplyMarkup or ParseMode) will be applied for all of them.
    ExtraTexts []string
    // If true, it will send and remove RemoveKeyboard message prior to main message sending.
    RemoveKeyboard bool
}
```

FSM MessageConfig extends tgbotapi MessageConfig with some extra information, that allows to send several messages in 
a row and remove current keyboard.

`TransitionProvider` implementation is a core of FSM state logic. Here you will define all state switching rules and 
data changes. 
That's why `TransitionFn` returns `Transition` object and data. `Transition` object defines the next state 
bot must switch to. And returned payload data will be passed as an argument for both the next state 
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

type AddTaskNameStateHandler struct{}

func (h AddTaskNameStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
    // This message will be shown, when moving to AddTaskNameState.
    return fsm.TextMessageConfig("Enter task name")
}

func (h AddTaskNameStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
    if update.Message == nil {
        return fsm.TextTransition("Please specify task name"), data
    }
    // A new task name is populated with the user response.
    data.newTask.name = update.Message.Text
    // And switch to the next state. AddTaskDescriptionState TransitionFn and MessageFn will get updated data.
    return fsm.StateTransition(AddTaskDescriptionState), data
}
// ...
configs := make(map[string]fsm.StateHandler[Data])
configs[AddTaskNameState] = AddTaskNameStateHandler{}
```

`Transition` is the following structure

```go
type Transition struct {
    State string
    MessageConfig
}
```

Here `State` is the name of the state (that string key in the config map) you want to transit to. Besides 
target state, you can also define the bot transition message as `MessageConfig`. Both `State` and `MessageConfig` 
are optional. If `State` is empty, the bot stays in the current state. If `MessageConfig` is empty, 
target `MessageFn` will be called to get message information. If `MessageConfig` is not empty, target `MessageFn` 
won't be called. Thus, transition `MessageConfig` _overrides_ `MessageProvider` behaviour.

`fsm.StateTransition(state string)` and `fsm.TextTransition(text string)` are 2 helper factories that create 
`Transition` with `State` and `MessageConfig.Text` correspondingly. 

Thus, the simplified pipeline looks like this
![alt text](docs/pipeline.png "pipeline")

## External state switch

Sometimes you need to change current user state and thus send a message by some external trigger. 
The typical example is sending notifications. With FSM you can do it using `GoTo` method

```go
err := botFsm.GoTo(context, chatId, transition, data)
```

## Commands

Commands are a very popular way to interact with bots. You can define command handlers using the following 
configuration `map[string]fsm.TransitionProvider[T]`. Here the map key is a command _without_ "/" prefix. 

```go
type StartCommandHandler struct{}

func (h StartCommandHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
    // "/start" command simply transits bot into "menu".
    return fsm.StateTransition("menu"), data
}

type FaqCommandHandler struct{}

func (h FaqCommandHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
    // "/faq" command returns some FAQ text.
    return fsm.TextTransition("Some faq text here"), data
}
// ...
commands := make(map[string]fsm.TransitionProvider[Data])
commands["start"] = StartCommandHandler{}
commands["faq"] = FaqCommandHandler{}
// Pass commands via fsm.WithCommands wrapper.
botFsm := fsm.NewBotFsm(bot, configs, fsm.WithCommands[Data](commands))
```

Note: When the incoming message is recognized as a command, the current user state is switched to `UndefinedState` 
before handler call. So, the command resets the user state. That makes `TextTransition` usage safe.

## State persistence 

`TransitionProvider` implementation may change payload data passed, and the next state `TransitionProvider` will 
receive an updated copy of  
the payload data. That means data must be kept somewhere between user requests. By default, it is stored in memory. 
That means, all the data will be lost after the bot re-run. It's okay during development or for very simple bots, 
but most of the time you want data to be kept. In order to do that you should implement `PersistenceHandler`.

```go
type PersistenceHandler[T any] interface {
    LoadStateFn(ctx context.Context, chatId int64) (state string, data T, err error)
    SaveStateFn(ctx context.Context, chatId int64, state string, data T) error
}
```

LoadStateFn is declared to restore the state name and data from persistent storage. SaveStateFn is used to save 
the state name and data into persistent storage. Together, these methods provide the ability to manage "session" 
data between requests. If load handler returns an empty `state` 
(e.g. when a user sends a message for the first time), it is treated as UndefinedState.

Persistence handlers can be provided using `fsm.WithPersistenceHandler` option function

```go
type CsvFilePersistenceHandler struct {
    File string
}

// PersistenceHandler implementation here.

botFsm := fsm.NewBotFsm(bot, configs, fsm.WithPersistenceHandler[Data](CsvFilePersistenceHandler{File: File}))
```

## Removing keyboard

There is a known 
[issue](https://stackoverflow.com/questions/45995052/how-to-remove-not-to-hide-replykeyboardmarkup-in-telegram-bot-using-c)
with custom [keyboard](https://core.telegram.org/bots/api#replykeyboardmarkup) removal. Sometimes you want to 
be sure that this keyboard no longer exists (even though the keyboard can be hidden). To achieve that, the 
bot must send a message with [ReplyKeyboardRemove](https://core.telegram.org/bots/api#replykeyboardremove) markup. 
But like any other telegram bot messages, this message must bear some text. Obviously, this message is a 
real garbage. Luckily,  telegram bot API allows removing any messages, and this message can be wiped out as well.

FSM supports this hack. There are 2 useful interfaces that any `StateHandler` may implement.
```go
type RemoveKeyboardAfterMarker interface {
	// RemoveKeyboardAfter gives a signal to remove keyboard after bot left the state. 
	RemoveKeyboardAfter() bool
}

type RemoveKeyboardBeforeMarker interface {
	// RemoveKeyboardBefore gives a signal to remove keyboard before bot enters the state.
	RemoveKeyboardBefore() bool
}
```

If it `RemoveKeyboardAfterMarker` is implemented and `RemoveKeyboardAfter` returns true, ReplyKeyboardRemove 
message will be sent and removed right before the current state is switched to another. `RemoveKeyboardBeforeMarker`
works similarly, but removes keyboard before state is switched to this one. 

`RemoveKeyboard` option also exists as a part of `MessageConfig`. Thus, keyboard removal can be controlled within 
`TransitionFn`.

ReplyKeyboardRemove message deletion request may take some time due to Telegram API network delays. That's why the
user may see this message text for a moment. In order to make it looks not so weird, it's better to provide some
reasonable text. By default, it is "Thinking...". But you can change that using `WithRemoveKeyboardTempText` option
function.

```go
botFsm := fsm.NewBotFsm(bot, configs, fsm.WithRemoveKeyboardTempText[Data]("Some text"))
```
