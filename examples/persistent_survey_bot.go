package main

import (
	"context"
	"encoding/csv"
	"fmt"
	fsm "github.com/Feolius/telegram-bot-fsm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"net/http"
	"os"
	"strconv"
)

const NameState = "name"
const AgeState = "age"

const File = "persons.csv"

type Data struct {
	PersonName string
	PersonAge  int
}

type StartCommandHandler struct{}

func (h StartCommandHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	// "/start" command initiates survey with empty data. It resets existing data.
	return fsm.StateTransition(NameState), Data{}
}

type WhoamiCommandHandler struct {
	File string
}

func (h WhoamiCommandHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	// "/whoami" command returns information about yourself, if it exists and non-empty.
	file, err := os.Open(h.File)
	if err != nil {
		return fsm.TextTransition("Error in attempt to load data"), Data{}
	}
	r := csv.NewReader(file)
	records, err := r.ReadAll()
	if err != nil {
		return fsm.TextTransition("Error in attempt to load data"), Data{}
	}
	chatIdStr := strconv.FormatInt(update.Message.Chat.ID, 10)
	for _, record := range records {
		if record[0] == chatIdStr && record[2] != "" && record[3] != "0" {
			return fsm.TextTransition(fmt.Sprintf("I'm %s %s years old", record[2], record[3])), data
		}
	}
	return fsm.TextTransition("You have to complete survey about yourself first"), data
}

type NameStateHandler struct{}

func (h NameStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	return fsm.TextMessageConfig("What is your name?")
}

func (h NameStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	if update.Message == nil {
		return fsm.TextTransition("Something goes wrong. Please let me know your name."), data
	}
	// A new task name is populated with the user response.
	data.PersonName = update.Message.Text
	return fsm.StateTransition(AgeState), data
}

type AgeStateHandler struct{}

func (h AgeStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	return fsm.TextMessageConfig("How old are you?")
}

func (h AgeStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	if update.Message == nil {
		return fsm.TextTransition("Something goes wrong. Please provide your age."), data
	}
	age, err := strconv.Atoi(update.Message.Text)
	if err != nil {
		return fsm.TextTransition("Age should be a number"), data
	}
	data.PersonAge = age
	transition := fsm.Transition{}
	transition.State = fsm.UndefinedState
	transition.Text = "You finished survey. Now you can ask who you are using /whoami command"
	return transition, data
}

type UndefinedStateHandler struct{}

func (h UndefinedStateHandler) MessageFn(ctx context.Context, data Data) fsm.MessageConfig {
	// Can be used for some general information.
	return fsm.TextMessageConfig("Use /start command to fill info about yourself or /whoami command to information about yourself.")
}

func (h UndefinedStateHandler) TransitionFn(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
	return fsm.Transition{}, data
}

type CsvFilePersistenceHandler struct {
	File string
}

func (h CsvFilePersistenceHandler) LoadStateFn(ctx context.Context, chatId int64) (state fsm.State, data Data, err error) {
	file, err := os.Open(h.File)
	if err != nil {
		return "", Data{}, err
	}
	r := csv.NewReader(file)
	records, err := r.ReadAll()
	if err != nil {
		return "", Data{}, err
	}
	chatIdStr := strconv.FormatInt(chatId, 10)
	for _, record := range records {
		if record[0] == chatIdStr {
			age, err := strconv.Atoi(record[3])
			if err != nil {
				return "", Data{}, err
			}
			return record[1], Data{record[2], age}, nil
		}
	}
	return "", Data{}, nil
}

func (h CsvFilePersistenceHandler) SaveStateFn(ctx context.Context, chatId int64, state fsm.State, data Data) error {
	file, err := os.Create(h.File)
	if err != nil {
		return err
	}
	r := csv.NewReader(file)
	records, err := r.ReadAll()
	if err != nil {
		return err
	}
	chatIdStr := strconv.FormatInt(chatId, 10)
	rowIndex := -1
	for index, record := range records {
		if record[0] == chatIdStr {
			rowIndex = index
		}
	}
	row := []string{chatIdStr, state, data.PersonName, strconv.Itoa(data.PersonAge)}
	if rowIndex == -1 {
		rowIndex = len(records)
		records = append(records, row)
	} else {
		records[rowIndex] = row
	}
	w := csv.NewWriter(file)
	err = w.WriteAll(records)
	if err != nil {
		return err
	}
	return nil
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

	configs := make(map[string]fsm.StateHandler[Data])
	configs[NameState] = NameStateHandler{}
	configs[AgeState] = AgeStateHandler{}
	configs[fsm.UndefinedState] = UndefinedStateHandler{}

	commands := make(map[string]fsm.TransitionProvider[Data])
	commands["start"] = StartCommandHandler{}
	commands["whoami"] = WhoamiCommandHandler{File: File}

	botFsm := fsm.NewBotFsm(
		bot,
		configs,
		fsm.WithCommands[Data](commands),
		fsm.WithPersistenceHandler[Data](CsvFilePersistenceHandler{File: File}),
	)

	ctx := context.TODO()
	for update := range updates {
		err = botFsm.HandleUpdate(ctx, &update)
	}
}
