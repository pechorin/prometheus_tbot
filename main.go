package main // import "github.com/pechorin/prometheus_tbot"

/*
TODO:
— port not working (missing ':')
*/

import (
	"bytes"
	"encoding/json"

	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gopkg.in/telegram-bot-api.v4"

	"html/template"

	"github.com/pechorin/prometheus_bot/pkg/appconfig"
	"github.com/pechorin/prometheus_bot/pkg/measureconv"
)

type Alerts struct {
	Alerts            []Alert                `json:"alerts"`
	CommonAnnotations map[string]interface{} `json:"commonAnnotations"`
	CommonLabels      map[string]interface{} `json:"commonLabels"`
	ExternalURL       string                 `json:"externalURL"`
	GroupKey          int                    `json:"groupKey"`
	GroupLabels       map[string]interface{} `json:"groupLabels"`
	Receiver          string                 `json:"receiver"`
	Status            string                 `json:"status"`
	Version           int                    `json:"version"`
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
		}

		if update.Message.NewChatMembers != nil && len(*update.Message.NewChatMembers) > 0 {
			for _, member := range *update.Message.NewChatMembers {
				if member.UserName == app.bot.Self.UserName && update.Message.Chat.Type == "group" {
					introduce(update)
				}
			}
		} else if update.Message != nil && update.Message.Text != "" {
			introduce(update)
		}
	}
}

func (app *Application) loadTemplate(tmplPath string) *template.Template {
	// let's read template
	tmpH, err := template.New(path.Base(tmplPath)).Funcs(app.TemplateFuncMap()).ParseFiles(app.config.TemplatePath)

	if err != nil {
		log.Fatalf("Problem reading parsing template file: %v", err)
	} else {
		log.Printf("Load template file:%s", tmplPath)
	}

	return tmpH
}

// TODO: make private
func SplitString(s string, n int) []string {
	sub := ""
	subs := []string{}

	runes := bytes.Runes([]byte(s))
	l := len(runes)
	for i, r := range runes {
		sub = sub + string(r)
		if (i+1)%n == 0 {
			subs = append(subs, sub)
			sub = ""
		} else if (i + 1) == l {
			subs = append(subs, sub)
		}
	}

	return subs
}

func main() {
	app := NewApplication()

	bot_tmp, err := tgbotapi.NewBotAPI(app.config.TelegramToken)

	if err != nil {
		fmt.Println("cant start bot with token: " + app.config.TelegramToken)
		log.Fatal(err)
	}

	app.bot = bot_tmp

	// app.bot.Client

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

	router.GET("/ping/:chatid", app.HTTPPingHandler)
	router.GET("/alert/*chatids", app.HTTPAlertHandler)
	router.POST("/alert/:chatid", app.POST_Handling) // TODO: legacy
	router.Run(app.config.ListenAddr)
}

func (app *Application) HTTPPingHandler(c *gin.Context) {
	log.Printf("Received GET")
	chatid, err := strconv.ParseInt(c.Param("chatid"), 10, 64)
	if err != nil {
		log.Printf("Cat't parse chat id: %q", c.Param("chatid"))
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"err": fmt.Sprint(err),
		})
		return
	}

	log.Printf("Bot test: %d", chatid)
	msgtext := fmt.Sprintf("Some HTTP triggered notification by prometheus bot... %d", chatid)
	msg := tgbotapi.NewMessage(chatid, msgtext)
	sendmsg, err := app.bot.Send(msg)
	if err == nil {
		c.String(http.StatusOK, msgtext)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"err":     fmt.Sprint(err),
			"message": sendmsg,
		})
	}
}

func (app *Application) parseMultiParam(s string, c *gin.Context) []int64 {
	chats := strings.Split(s, "/")
	chatIds := make([]int64, len(chats))

	for _, chat := range chats {
		log.Println("processing CHAT id -> ", chat)

		if len(chat) == 0 { // а почему такое бывает в начале слайса? оО
			continue
		}
		if chatId, err := strconv.ParseInt(chat, 10, 64); err != nil {
			log.Printf("Cat't parse chat id: %q", s)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"err": fmt.Sprint(err),
			})

			return make([]int64, 0)
		} else {
			chatIds = append(chatIds, chatId)
		}
	}

	return chatIds

}

func (app *Application) HTTPAlertHandler(c *gin.Context) {
	chatIds := app.parseMultiParam(c.Param("chatids"), c)
	chatIdsStrings := make([]string, len(chatIds))

	for _, chatID := range chatIds {
		str := strconv.FormatInt(chatID, 10)
		chatIdsStrings = append(chatIdsStrings, str)
	}

	formatted := strings.Join(chatIdsStrings, ",")

	// for _, chatId := range chatIds {
	// 	app.NewChatAlertHandler(chatId, c)
	// }

	c.String(http.StatusOK, "vse ok: ["+formatted+"] and len is "+strconv.Itoa(len(chatIds)))
}

func (app *Application) NewChatAlertHandler(chatid int64, c *gin.Context) {
	msgtext := fmt.Sprintf("Some HTTP triggered notification by prometheus bot... %d", chatid)
	msg := tgbotapi.NewMessage(chatid, msgtext)
	sendmsg, err := app.bot.Send(msg)
	if err == nil {
		c.String(http.StatusOK, msgtext)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"err":     fmt.Sprint(err),
			"message": sendmsg,
		})
	}
}

func (*Application) AlertFormatStandard(alerts Alerts) string {
	keys := make([]string, 0, len(alerts.GroupLabels))
	for k := range alerts.GroupLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	groupLabels := make([]string, 0, len(alerts.GroupLabels))
	for _, k := range keys {
		groupLabels = append(groupLabels, fmt.Sprintf("%s=<code>%s</code>", k, alerts.GroupLabels[k]))
	}

	keys = make([]string, 0, len(alerts.CommonLabels))
	for k := range alerts.CommonLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	commonLabels := make([]string, 0, len(alerts.CommonLabels))
	for _, k := range keys {
		if _, ok := alerts.GroupLabels[k]; !ok {
			commonLabels = append(commonLabels, fmt.Sprintf("%s=<code>%s</code>", k, alerts.CommonLabels[k]))
		}
	}

	keys = make([]string, 0, len(alerts.CommonAnnotations))
	for k := range alerts.CommonAnnotations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	commonAnnotations := make([]string, 0, len(alerts.CommonAnnotations))
	for _, k := range keys {
		commonAnnotations = append(commonAnnotations, fmt.Sprintf("\n%s: <code>%s</code>", k, alerts.CommonAnnotations[k]))
	}

	alertDetails := make([]string, len(alerts.Alerts))
	for i, a := range alerts.Alerts {
		if instance, ok := a.Labels["instance"]; ok {
			instanceString, _ := instance.(string)
			alertDetails[i] += strings.Split(instanceString, ":")[0]
		}
		if job, ok := a.Labels["job"]; ok {
			alertDetails[i] += fmt.Sprintf("[%s]", job)
		}
		if a.GeneratorURL != "" {
			alertDetails[i] = fmt.Sprintf("<a href='%s'>%s</a>", a.GeneratorURL, alertDetails[i])
		}
	}
	return fmt.Sprintf(
		"<a href='%s/#/alerts?receiver=%s'>[%s:%d]</a>\ngrouped by: %s\nlabels: %s%s\n%s",
		alerts.ExternalURL,
		alerts.Receiver,
		strings.ToUpper(alerts.Status),
		len(alerts.Alerts),
		strings.Join(groupLabels, ", "),
		strings.Join(commonLabels, ", "),
		strings.Join(commonAnnotations, ""),
		strings.Join(alertDetails, ", "),
	)
}

func (app *Application) AlertFormatTemplate(alerts Alerts) string {
	var bytesBuff bytes.Buffer
	var err error

	writer := io.Writer(&bytesBuff)

	if app.config.Debug {
		log.Printf("Reloading Template\n")
		// reload template bacause we in debug mode
		app.template = app.loadTemplate(app.config.TemplatePath)
	}

	app.template.Funcs(app.TemplateFuncMap())
	err = app.template.Execute(writer, alerts)

	if err != nil {
		log.Fatalf("Problem with template execution: %v", err)
		panic(err)
	}

	return bytesBuff.String()
}

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
	for _, subString := range SplitString(msgtext, app.config.SplitMessageBytes) {
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
