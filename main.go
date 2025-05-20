package main

import (
	"io"
	"log"
	"os"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"gopkg.in/yaml.v3"

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

func handleWebhook(bot *tgbotapi.BotAPI, webhookSecret string, chatID int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		eventType := r.Header.Get("X-GitHub-Event")
        if eventType != "workflow_run" {
            http.Error(w, "Unknown event", http.StatusBadRequest)
            return
        }

        body, err := io.ReadAll(r.Body)
        if err != nil {
            http.Error(w, "Error reading body", http.StatusInternalServerError)
            return
        }
        r.Body = io.NopCloser(bytes.NewBuffer(body))

        sig := r.Header.Get("X-Hub-Signature-256")
        if sig == "" {
            http.Error(w, "Missing signature", http.StatusUnauthorized)
            return
        }

        mac := hmac.New(sha256.New, []byte(webhookSecret))
        mac.Write(body)
        expectedMAC := hex.EncodeToString(mac.Sum(nil))
        expectedSig := "sha256=" + expectedMAC

        if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
            http.Error(w, "Invalid signature", http.StatusUnauthorized)
            return
        }

        var payload struct {
            Action      string `json:"action"`
            WorkflowRun struct {
                Status     string `json:"status"`
                Conclusion string `json:"conclusion"`
                HTMLURL    string `json:"html_url"`
            } `json:"workflow_run"`
        }

        if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
            http.Error(w, "Error decoding JSON", http.StatusBadRequest)
            return
        }

        if payload.Action != "completed" {
            w.WriteHeader(http.StatusOK)
            return
        }

        var msgText string
        if payload.WorkflowRun.Conclusion == "success" {
            msgText = fmt.Sprintf("✅ Workflow успешно завершен!\nСсылка: %s", payload.WorkflowRun.HTMLURL)
        } else {
            msgText = fmt.Sprintf("❌ Workflow завершился с ошибкой!\nСтатус: %s\nСсылка: %s", payload.WorkflowRun.Conclusion, payload.WorkflowRun.HTMLURL)
        }

        msg := tgbotapi.NewMessage(chatID, msgText)
        if _, err := bot.Send(msg); err != nil {
            log.Printf("Ошибка отправки сообщения: %v", err)
        }

        w.WriteHeader(http.StatusOK)
	}
}

type Config struct {
	GitHub struct {
		Owner		string `yaml:"owner"`
		Repo		string `yaml:"repo"`
		TokenFile	string `yaml:"token_file"`
	} `yaml:"github"`

	Telegram struct {
		TokenFile string `yaml:"token_file"`
		ChatIDFile string `yaml:"chat_id_file"`
	} `yaml:"telegram"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func main() {
	config, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Ошибка загрузки конфига %v", err)
	}
	tokenBytes, err := os.ReadFile("tg_token")
	if err != nil {
		log.Fatal("Ошибка чтения телеграм токена %v", err)
	}
	telegramToken := strings.TrimSpace(string(tokenBytes))
	bot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Panic(err)
	}

	gh_tokenBytes, err := os.ReadFile("gh_token")
	if err != nil {
		log.Fatalf("Ошибка чтения гитхаб токена %v", err)
	}
	githubToken := strings.TrimSpace(string(gh_tokenBytes))

	chatIDBytes, err := os.ReadFile("chatID")
	if err != nil {
		log.Fatalf("Ошибка чтения чата ID %v", err)
	}
	chatIDStr := strings.TrimSpace(string(chatIDBytes))
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		log.Fatalf("Ошибка парсинга chat_id %v", err)
	}
	
	

	webhookSecretBytes, err := os.ReadFile("webhook")
	if err != nil {
		log.Fatalf("Ошибка чтения webhook %v", err)
	}
	webhookSecret := strings.TrimSpace(string(webhookSecretBytes))

	go func() {
		http.HandleFunc("/webhook", handleWebhook(bot, webhookSecret, chatID))
		log.Println("Сервер вебхука запущен на :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatalf("Ошибка запуска сервера вебхука %v", err)
		}
	}()

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
			msg.Text = "Привет! Я бот для просмотра статусом workflow. Доступные команды:\n/status - проверить сервер\n/cicd - узнать последний статус workflow"
		case "/status":
			msg.Text = "Артееемка"
		case "/cicd":
			status, err := getGitHubActionsStatus(config.GitHub.Owner, config.GitHub.Repo, githubToken)
			if err != nil {
				msg.Text = "Ошибка: " + err.Error()
			} else {
				msg.Text = status
				msg.ParseMode = "Markdown"
			}
		case "/setrepo":
			args := strings.Split(update.Message.Text, " ")
			if len(args) != 3 {
				msg.Text = "Формат: /setrepo <owner> <repo>"
				break
			}
			config.GitHub.Owner = args[1]
			config.GitHub.Repo = args[2]
			msg.Text = fmt.Sprintf("Теперь отслеживаю: %s/%s", args[1], args[2])
		default:
			msg.Text = "Неизвестная команда."
		}

		if _, err := bot.Send(msg); err != nil {
			log.Println(err)
		}
	}
}
