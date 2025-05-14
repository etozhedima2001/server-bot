package main

import (
	"io"
	"log"
	"os"
	"encoding/json"
	"fmt"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type GitHubWorkflowRun struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

func getGitHubActionsStatus(repoOwner, repoName, githubToken string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs?per_page=1", repoOwner, repoName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка HTTP-запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API вернул %d, %s", resp.StatusCode, string(body))
	}

	var result struct {
		WorkflowRuns []GitHubWorkflowRun `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ошибка парсинга JSON: %v", err)
	}

	if len(result.WorkflowRuns) == 0 {
		return "Нет данных о workflow", nil
	}

	run := result.WorkflowRuns[0]
	return fmt.Sprintf(
		"**Статус CI/CD (GitHub Actions)**\n"+
			"🔹 **Статус:** `%s`\n"+
			"🔹 **Результат:** `%s`\n"+
			"🔹 **Ссылка:** [Открыть Workflow](%s)",
		run.Status,
		run.Conclusion,
		run.HTMLURL,
	), nil
}


func main() {
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if telegramToken == "" {
        log.Fatal("❌ TELEGRAM_TOKEN не установлен! Проверьте переменные окружения.")
    }
	bot, err := tgbotapi.NewBotAPI(telegramToken)
	githubToken := os.Getenv("GITHUB_TOKEN")
        repoOwner := "etozhedima2001"
        repoName := "server-bot"
	if err != nil {
		log.Panic(err)
	}

	if githubToken == "" {
		log.Panic("github токен не установлен")
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
			msg.Text = "Привет! Я бот для управления сервером. Доступные команды:\n/status - проверить сервер\n/cicd - узнать статус ci/cd"
		case "/status":
			msg.Text = "Сервер работает!testing deploy"
		case "/cicd":
			status, err := getGitHubActionsStatus(repoOwner, repoName, githubToken)
			if err != nil {
				msg.Text = "Ошибка: " + err.Error()
			} else {
				msg.Text = status
				msg.ParseMode = "Markdown"
			}
		default:
			msg.Text = "Неизвестная команда."
		}

		if _, err := bot.Send(msg); err != nil {
			log.Println(err)
		}
	}
}
