package main // import "github.com/pechorin/prometheus_tbot"

/*
TODO:
— port not working (missing ':')
*/

import (
	"encoding/json"
	"time"

	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gopkg.in/telegram-bot-api.v4"

	"html/template"
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
	template         *template.Template
}

func NewApplication() *Application {
	app := new(Application)
	app.config = appconfig.Setup()
	app.measureConverter = &measureconv.Converter{Config: app.config}

	return app
}

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

// TODO: move to package
func (app *Application) telegramBot(bot *tgbotapi.BotAPI) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatal(err)
	}

	introduce := func(update tgbotapi.Update) {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Chat id is '%d'", update.Message.Chat.ID))
		app.bot.Send(msg)
	}

	for update := range updates {
		if update.Message == nil {
			if app.config.Debug {
				log.Printf("[UNKNOWN_MESSAGE] [%v]", update)
			}
			continue
		} else {
			introduce(update)
		}
	}
}

func main() {
	app := NewApplication()

	bot_tmp, err := tgbotapi.NewBotAPI(app.config.TelegramToken)

	if err != nil {
		fmt.Println("cant start bot with token: " + app.config.TelegramToken)
		log.Fatal(err)
	}

	app.bot = bot_tmp

	if app.config.Debug {
		app.bot.Debug = true
	}
	if app.config.TemplatePath != "" {

		app.template = app.loadTemplate(app.config.TemplatePath)

		if app.config.TimeZone == "" {
			log.Fatalf("You must define time_zone of your bot")
			panic(-1)
		}

	} else {
		app.template = nil
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

func (app *Application) parseMultiParam(s string, c *gin.Context) []int64 {
	chats := strings.Split(s, "/")
	chatIds := make([]int64, len(chats))

	for _, chat := range chats {
		if len(chat) == 0 { // первый элемент слайса почему-то пустая строка
			continue
		}

		if chatId, err := strconv.ParseInt(chat, 10, 64); err != nil {
			log.Printf("Cat't parse chat id: %q", chat)
			return make([]int64, 0)
		} else {
			chatIds = append(chatIds, chatId)
		}
	}

	return chatIds

}

func (app *Application) HTTPAlertHandler(c *gin.Context) {
	chatIds := app.parseMultiParam(c.Param("chatids"), c)

	if len(chatIds) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": "no chats provided",
		})

		return
	}

	chatIdsStrings := make([]string, len(chatIds))

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

	messagesBuffers := app.TestFormatter(alerts)

	for _, chatID := range chatIds {
		str := strconv.FormatInt(chatID, 10)
		chatIdsStrings = append(chatIdsStrings, str)

		for idx, buffer := range messagesBuffers {
			if buffer.Len() > 0 {
				if idx > 0 {
					time.Sleep(3)
				}

				msg := tgbotapi.NewMessage(chatID, buffer.String())
				msg.ParseMode = "HTML"
				app.bot.Send(msg)
			}
		}

	}

	c.String(http.StatusOK, "OK, delivered for", len(chatIds), "chats")
}

// DEPRECATED:
func (app *Application) POST_Handling(c *gin.Context) {
	var msgtext string
	var alerts Alerts

	chatid, err := strconv.ParseInt(c.Param("chatid"), 10, 64)

	log.Printf("Bot alert post: %d", chatid)

	if err != nil {
		log.Printf("Cat't parse chat id: %q", c.Param("chatid"))
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"err": fmt.Sprint(err),
		})
		return
	}

	binding.JSON.Bind(c.Request, &alerts)

	s, err := json.Marshal(alerts)
	if err != nil {
		log.Print(err)
		return
	}

	log.Println("+------------------  A L E R T  J S O N  -------------------+")
	log.Printf("%s", s)
	log.Println("+-----------------------------------------------------------+\n\n")

	// Decide how format Text
	if app.config.TemplatePath == "" {
		msgtext = app.AlertFormatStandard(alerts)
	} else {
		msgtext = app.AlertFormatTemplate(alerts)
	}
	for _, subString := range splitString(msgtext, app.config.SplitMessageBytes) {
		msg := tgbotapi.NewMessage(chatid, subString)
		msg.ParseMode = tgbotapi.ModeHTML

		// Print in Log result message
		log.Println("+---------------  F I N A L   M E S S A G E  ---------------+")
		log.Println(subString)
		log.Println("+-----------------------------------------------------------+")

		msg.DisableWebPagePreview = true

		sendmsg, err := app.bot.Send(msg)
		if err == nil {
			c.String(http.StatusOK, "telegram msg sent.")
		} else {
			log.Printf("Error sending message: %s", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"err":     fmt.Sprint(err),
				"message": sendmsg,
				"srcmsg":  fmt.Sprint(msgtext),
			})
			msg := tgbotapi.NewMessage(chatid, "Error sending message, checkout logs")
			app.bot.Send(msg)
		}
	}

}
