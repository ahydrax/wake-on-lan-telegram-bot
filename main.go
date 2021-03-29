package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/mdlayher/wol"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const (
	name        = "Wake on Lan Telegram bot"
	description = "Helps you with booting your PCs"
)

type AppConfig struct {
	Machines []struct {
		Name    string
		Mac     string
		Address string
	}
	Bot struct {
		BotKey  string
		OwnerId int64
	}
}

var configPath string
var config AppConfig

func init() {
	flag.StringVar(&configPath, "config", "/etc/wol-tg-bot.json", "-config /etc/wol-tg-bot.json")
	flag.Parse()

	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatal(err)
		panic(err)
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
}

func main() {

	osInterrupt := make(chan os.Signal, 1)
	signal.Notify(osInterrupt, os.Interrupt, os.Kill, syscall.SIGABRT, syscall.SIGTERM)

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot, err := tgbotapi.NewBotAPI(config.Bot.BotKey)
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
	defer bot.StopReceivingUpdates()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatal(err)
		panic(err)
	}

	wolClient, err := wol.NewClient()
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
	defer wolClient.Close()

	go processUpdates(appCtx, updates, bot, wolClient)

	sig := <-osInterrupt
	log.Println("Got signal: ", sig)
}

func processUpdates(ctx context.Context, updates tgbotapi.UpdatesChannel, bot *tgbotapi.BotAPI, client *wol.Client) {

	ownerId := config.Bot.OwnerId

	kbButtons := make([]tgbotapi.KeyboardButton, len(config.Machines))
	for i := range kbButtons {
		machine := config.Machines[i]
		kbButtons[i] = tgbotapi.NewKeyboardButton(machine.Name)
	}

	kbMarkup := tgbotapi.NewReplyKeyboard(kbButtons)
	kbMarkup.OneTimeKeyboard = false

	for {
		select {
		case <-ctx.Done():
			return
		case update := <-updates:
			if update.Message != nil && update.Message.Chat.ID == ownerId {

				text := update.Message.Text

				log.Println("Got update: '", text, "' from ", update.Message.Chat.ID)

				if text == "/start" {

					msg := tgbotapi.NewMessage(ownerId, "Select machine to start:")
					msg.ReplyMarkup = kbMarkup
					bot.Send(msg)
					continue
				}

				machineIndex := findMachineIndex(text)
				if machineIndex != -1 {
					machine := config.Machines[machineIndex]
					mac, err := net.ParseMAC(machine.Mac)
					if err != nil {
						log.Println(err)
						continue
					}
					client.Wake(machine.Address, mac)
				}
			}
		}
	}
}

func findMachineIndex(machineName string) int {
	for i := range config.Machines {
		if machineName == config.Machines[i].Name {
			return i
		}
	}
	return -1
}