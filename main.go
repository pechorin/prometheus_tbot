package main // import "github.com/pechorin/prometheus_tbot"

import (
	"bytes"
	"html/template"
	"time"

	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/telegram-bot-api.v4"

	textTemplate "text/template"

	"github.com/pechorin/prometheus_tbot/pkg/appconfig"
	"github.com/pechorin/prometheus_tbot/pkg/measureconv"
)

type Alerts struct {
	Alerts            []Alert                `json:"alerts"`
	CommonAnnotations map[string]interface{} `json:"commonAnnotations"`
	CommonLabels      map[string]interface{} `json:"commonLabels"`
	ExternalURL       string                 `json:"externalURL"`
	GroupKey          string                 `json:"groupKey"`
	GroupLabels       map[string]interface{} `json:"groupLabels"`
	Receiver          string                 `json:"receiver"`
	Status            string                 `json:"status"`
	Version           string                 `json:"version"`
}

type Alert struct {
	Annotations  map[string]interface{} `json:"annotations"`
	EndsAt       string                 `json:"endsAt"`
	GeneratorURL string                 `json:"generatorURL"`
	Labels       map[string]interface{} `json:"labels"`
	StartsAt     string                 `json:"startsAt"`
}

type Application struct {
	config           *appconfig.Config
	bot              *tgbotapi.BotAPI
	measureConverter *measureconv.Converter
}

func NewApplication() *Application {
	app := new(Application)
	app.config = appconfig.Setup()
	app.measureConverter = &measureconv.Converter{Config: app.config}

	return app
}


func main() {
	app := NewApplication()

	botTmp, err := tgbotapi.NewBotAPI(app.config.TelegramToken)

	if err != nil {
		fmt.Println("cant start bot with token: " + app.config.TelegramToken)
		log.Fatal(err)
	}

	app.bot = botTmp

	if app.config.Debug {
		app.bot.Debug = true
	}

	if !(app.config.Debug) {
		gin.SetMode(gin.ReleaseMode)
	}

	log.Printf("Authorised on account %s", app.bot.Self.UserName)
	go app.telegramBot(app.bot)

	router := gin.Default()

	router.POST("/alert/*chatids", app.HTTPAlertHandler)
	router.Run(app.config.ListenAddr)
}

func (app *Application) telegramBot(bot *tgbotapi.BotAPI) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatal(err)
	}

	sendChatId := func(update tgbotapi.Update) {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Chat id is '%d'", update.Message.Chat.ID))
		if _, err := app.bot.Send(msg); err != nil {
			log.Println("error while sending sendChatId", err)
		}
	}

	for update := range updates {
		if update.Message != nil {
			switch update.Message.Text {
			case "/chatID", "/chatid", "/chatId":
				sendChatId(update)
			default:
				continue
			}
		}
	}
}

func (app *Application) parseMultiParam(s string, c *gin.Context) []int64 {
	chats := strings.Split(s, "/")
	chatIds := []int64{}

	for _, chat := range chats {
		if chat == "" {
			continue
		}

		if chatId, err := strconv.ParseInt(chat, 10, 64); err != nil {
			log.Printf("Cat't parse chat id: %q", chat)
			return make([]int64, 0)
		} else {
			chatIds = append(chatIds, chatId)
		}
	}

	if app.config.Debug == true {
		log.Println("Chat ids for send", chatIds)
	}

	return chatIds
}

func (app *Application) HTTPAlertHandler(c *gin.Context) {
	chatIds := app.parseMultiParam(c.Param("chatids"), c)

	if len(chatIds) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"desc": "no chats provided",
		})

		return
	}

	alerts := new(Alerts)

	if err := c.ShouldBindJSON(alerts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err":    err,
			"info":   "alerts data invalid",
			"alerts": alerts,
			"errstr": err.Error(),
		})

		return
	}

	bufferCh := make(chan int64, len(chatIds))

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Println("Panic handled while sending message:", err)
			}
		}()

		for chatID := range bufferCh {
			if chatID == 0 {
				log.Println("Skip for 0 chatID")
				continue
			}

			defaultMessages := app.NewTemplateMessage(alerts, app.config.Templates["default"])

			chatIDStr := strconv.FormatInt(chatID, 10)

			renderBuffersPtr := &defaultMessages

			if chatTemplateName, ok := app.config.ChatsTemplates[chatIDStr]; ok == true {
				if chatTemplate, ok := app.config.Templates[chatTemplateName]; ok == true {
					chatMessages := app.NewTemplateMessage(alerts, chatTemplate)
					renderBuffersPtr = &chatMessages
				}
			}

			for idx, buffer := range *renderBuffersPtr {
				if buffer.Len() > 0 {
					if idx > 0 {
						time.Sleep(3)
					}

					msg := tgbotapi.NewMessage(chatID, buffer.String())
					msg.ParseMode = "HTML"

					if _, err := app.bot.Send(msg); err != nil {
						log.Println("Error while sending message:", chatID ,err)
					}
				}
			}

		}
	}()

	for _, chatID := range chatIds {
		log.Println("Sending chat-id", chatID)
		bufferCh <- chatID
	}

	close(bufferCh)

	c.String(http.StatusOK, "OK, delivered for", len(chatIds), "chats")
}

// Templating staff

func hasKey(dict map[string]interface{}, key_search string) bool {
	if _, ok := dict[key_search]; ok {
		return true
	}
	return false
}

// TODO: move to formaters with other template loader functions?
func (app *Application) TemplateFuncMap() (tm template.FuncMap) {
	tm = template.FuncMap{
		"FormatDate":        app.measureConverter.FormatDate,
		"ToUpper":           strings.ToUpper,
		"ToLower":           strings.ToLower,
		"Title":             strings.Title,
		"FormatFloat":       app.measureConverter.FormatFloat,
		"FormatByte":        app.measureConverter.FormatByte,
		"FormatMeasureUnit": app.measureConverter.FormatMeasureUnit,
		"HasKey":            hasKey,
	}

	return
}

func (app *Application) TextTemplateFuncMap() (tm textTemplate.FuncMap) {
	tm = textTemplate.FuncMap{
		"FormatDate":        app.measureConverter.FormatDate,
		"ToUpper":           strings.ToUpper,
		"ToLower":           strings.ToLower,
		"Title":             strings.Title,
		"FormatFloat":       app.measureConverter.FormatFloat,
		"FormatByte":        app.measureConverter.FormatByte,
		"FormatMeasureUnit": app.measureConverter.FormatMeasureUnit,
		"HasKey":            hasKey,
	}

	return
}

func (app *Application) NewTemplateMessage(alerts *Alerts, template string) []*bytes.Buffer {
	buffers := make([]*bytes.Buffer, 0)
	buffers = append(buffers, new(bytes.Buffer))

	currentBufferIndex := 0
	currentBuffer := buffers[currentBufferIndex]

	tmpl, err := textTemplate.New("defaultMessage").Funcs(app.TextTemplateFuncMap()).Parse(template)

	if err != nil {
		log.Fatalf("Problem parsing template messageMini: %v", err)
	}

	if alerts.Status == "firing" {
		currentBuffer.WriteString("<b>Firing</b>🔥\n\n")
	} else {
		currentBuffer.WriteString("<b>" + alerts.Status + "</b>" + "\n\n")
	}

	for _, alert := range alerts.Alerts {
		tempBuffer := new(bytes.Buffer)

		if err := tmpl.Execute(tempBuffer, alert); err != nil {
			log.Fatalf("Problem executing template: %v", err)
		}

		// if currentBuffer Len is reach limit then create new buffer
		if (currentBuffer.Len() + tempBuffer.Len()) > app.config.SplitMessageBytes {
			buffers = append(buffers, new(bytes.Buffer))
			currentBufferIndex = currentBufferIndex + 1
			currentBuffer = buffers[currentBufferIndex]
		}

		// currentBuffer.WriteString("\n")
		currentBuffer.WriteString(tempBuffer.String())
		currentBuffer.WriteString("\n")
	}

	return buffers
}
