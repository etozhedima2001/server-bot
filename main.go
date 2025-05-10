package main

import (
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true // Включить логирование

	log.Printf("Бот запущен: %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

		switch update.Message.Text {
		case "/start":
			msg.Text = "Привет! Я бот для управления сервером. Доступные команды:\n/status - проверить сервер\n/restart - перезагрузить службу"
		case "/status":
			msg.Text = "Сервер работает!"
		default:
			msg.Text = "Неизвестная команда."
		}

		if _, err := bot.Send(msg); err != nil {
			log.Println(err)
		}
	}
}
