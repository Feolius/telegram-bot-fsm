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

const FileName = "persons.csv"

type Data struct {
	PersonName string
	PersonAge  int
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
		// "/start" command initiates survey with empty data. It resets existing data.
		return fsm.StateTransition(NameState), Data{}
	}
	commands["whoami"] = func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
		// "/whoami" command returns information about yourself, if it exists and non-empty.
		file, err := os.Open(FileName)
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

	botFsm := fsm.NewBotFsm(
		bot,
		configs,
		fsm.WithCommands[Data](commands),
		fsm.WithPersistenceHandlers[Data](getPersistentHandlers()),
	)

	ctx := context.TODO()
	for update := range updates {
		err = botFsm.HandleUpdate(ctx, &update)
	}
}

func getFsmConfigs() map[string]fsm.StateConfig[Data] {
	configs := make(map[string]fsm.StateConfig[Data])
	configs[NameState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			return fsm.MessageConfig{
				Text: "What is your name?",
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			if update.Message == nil {
				return fsm.TextTransition("Something goes wrong. Please let me know your name."), data
			}
			// A new task name is populated with the user response.
			data.PersonName = update.Message.Text
			return fsm.StateTransition(AgeState), data
		},
	}
	configs[AgeState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			return fsm.MessageConfig{
				Text: "How old are you?",
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			if update.Message == nil {
				return fsm.TextTransition("Something goes wrong. Please provide your age."), data
			}
			age, err := strconv.Atoi(update.Message.Text)
			if err != nil {
				return fsm.TextTransition("Age should be a number"), data
			}
			data.PersonAge = age
			return fsm.Transition{
				Target: fsm.UndefinedState,
				MessageConfig: fsm.MessageConfig{
					Text: "You finished survey. Now you can ask who you are using /whoami command",
				},
			}, data
		},
	}
	// UndefinedState must be provided.
	configs[fsm.UndefinedState] = fsm.StateConfig[Data]{
		MessageFn: func(ctx context.Context, data Data) fsm.MessageConfig {
			// It can be used for some general information.
			return fsm.MessageConfig{
				Text: "Use /start command to fill info about yourself or /whoami command to information about yourself.",
			}
		},
		TransitionFn: func(ctx context.Context, update *tgbotapi.Update, data Data) (fsm.Transition, Data) {
			return fsm.Transition{}, data
		},
	}
	return configs
}

func getPersistentHandlers() (fsm.LoadStateFn[Data], fsm.SaveStateFn[Data]) {
	return func(ctx context.Context, chatId int64) (name string, data Data, err error) {
			file, err := os.Open(FileName)
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
		}, func(ctx context.Context, chatId int64, name string, data Data) error {
			file, err := os.Create(FileName)
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
			row := []string{chatIdStr, name, data.PersonName, strconv.Itoa(data.PersonAge)}
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
}
