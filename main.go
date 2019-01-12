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

type PrometheusAlertsView struct {
	PageNumber 	   			int
	PageMessages   			[]*bytes.Buffer
	Alerts  	   			*Alerts // TODO: rename to AlertsJson / AlertsData ?
}

type Application struct {
	config           	*appconfig.Config
	bot              	*tgbotapi.BotAPI
	measureConverter 	*measureconv.Converter
}

func NewApplication() *Application {
	app := new(Application)
	app.config = appconfig.New()
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
		log.Printf("Authorised on account %s", app.bot.Self.UserName)
	}

	if !(app.config.Debug) {
		gin.SetMode(gin.ReleaseMode)
	}

	go app.telegramBot(app.bot)

	router := gin.Default()

	router.POST("/alert/*chatids", app.HTTPAlertHandler)
	router.Run(app.config.Port)

	fmt.Printf("Prometheus Tbot started at port %v", app.config.Port)
	log.Printf("Prometheus Tbot started at port %v", app.config.Port)
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
		//defer func() {
		//	if err := recover(); err != nil {
		//		log.Printf("Panic handled while sending message: %v", err)
		//	}
		//}()

		for chatID := range bufferCh {
			if chatID == 0 {
				if app.config.Debug {
					log.Println("Skip for 0 chatID")
				}

				continue
			}

			// default render values
			selectedLayout := appconfig.SelectedLayout{ Layout: "prometheus", MessageTemplate: "prometheus", GroupByAlertName: true }

			chatIDStr := strconv.FormatInt(chatID, 10)

			if chatLayoutConfig, ok := app.config.ChatsLayouts[chatIDStr]; ok == true {
				if newLayout, ok := chatLayoutConfig["layout"]; ok == true {
					selectedLayout.Layout = newLayout
				}

				if newMessageTemplate, ok := chatLayoutConfig["message_template"]; ok == true {
					selectedLayout.MessageTemplate = newMessageTemplate
				}
			}

			// currently where are 2 rendering types for Prometheus: with and without grouping
			sendBuffers := make([]*bytes.Buffer, 0)

			if selectedLayout.GroupByAlertName == true {
				sendBuffers = app.RenderPrometheusAlertsWithGrouping(alerts, selectedLayout)
			} else {
				sendBuffers = app.RenderPrometheusAlerts(alerts, selectedLayout)
			}

			for idx, buffer := range sendBuffers {
				if buffer.Len() > 0 {
					if idx > 0 {
						time.Sleep(3) // delay before second message
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
		if app.config.Debug {
			log.Println("Sending chat-id", chatID)
		}

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
// TODO: сейчас оно вешает весь процесс при падении здесь
// TODO: just return an error?
func (app *Application) RenderPrometheusAlerts(alerts *Alerts, selectedLayout appconfig.SelectedLayout) []*bytes.Buffer {
	// extract layout templating
	layoutTemplate := textTemplate.New("TelegramMessage").Funcs(app.TextTemplateFuncMap())
	layoutTemplate, err := layoutTemplate.Parse(app.config.Layouts[selectedLayout.Layout])
	if err != nil {
		log.Fatalf("Error while parsing layout %v %v", selectedLayout.Layout, err)
	}

	layoutTemplate, err = layoutTemplate.Parse(appconfig.DefaultPrometheusMessageTemplate())
	if err != nil {
		log.Fatalf("Error while parsing DefaultPrometheusMessageTemplate: %v", appconfig.DefaultPrometheusMessageTemplate(), err)
	}

	// extract message template
	messageTemplate := textTemplate.New("TelegramRowMessage").Funcs(app.TextTemplateFuncMap())
	messageTemplate, err = messageTemplate.Parse(app.config.MessageTemplates[selectedLayout.MessageTemplate])
	if err != nil {
		log.Fatalf("Error while parsing message template %v %v", selectedLayout.MessageTemplate, err)
	}

	// render alerts (separate from template)
	renderedMessages := make([]*bytes.Buffer, 0)

	// TODO: debug messages render index

	for _, alert := range alerts.Alerts {
		tempBuffer := new(bytes.Buffer)

		// render message row partial
		if err := messageTemplate.Execute(tempBuffer, alert); err != nil {
			log.Fatalf("Error while rendering message template: %v", err)
		}

		renderedMessages = append(renderedMessages, tempBuffer)
	}

	// start rendering separated pages (if required)
	renderedPages := make([]*bytes.Buffer, 0)
	renderedPages = append(renderedPages, new(bytes.Buffer))

	currentPage           := 0
	currentPageStartIndex := 0

	// TODO: Not optimal algorithm because i use incremental rendering + len check.
	// Instead calculate len, split to partitions and render in pageNumbers steps.
	for idx, _ := range alerts.Alerts {
		view := new(PrometheusAlertsView)
		view.PageNumber = currentPage
		view.PageMessages = renderedMessages[currentPageStartIndex:idx + 1]
		view.Alerts = alerts

		// render messages according page number and offset
		temp := new(bytes.Buffer)
		if err := layoutTemplate.Execute(temp, view); err != nil {
			log.Fatalf("Error while rendering full template: %v", err)
		}

		if (temp.Len()) <= app.config.SplitMessageBytes {
			renderedPages[currentPage].Reset()
			renderedPages[currentPage].Write(temp.Bytes())
		} else {
			// page is full, create new one
			currentPage += 1
			currentPageStartIndex = idx

			newPageView := new(PrometheusAlertsView)
			newPageView.PageNumber = currentPage
			newPageView.PageMessages = renderedMessages[currentPageStartIndex:idx + 1]
			newPageView.Alerts = alerts

			newPageTemp := new(bytes.Buffer)
			if err := layoutTemplate.Execute(newPageTemp, newPageView); err != nil {
				log.Fatalf("Error while rendering full template: %v", err)
			}

			renderedPages = append(renderedPages, new(bytes.Buffer))
			renderedPages[currentPage].Write(newPageTemp.Bytes())
			//currentPage +
		}
	}

	if app.config.Debug {
		for idx, page := range renderedPages {
			log.Printf("page %v len: %v", idx, page.Len())
		}
	}

	return renderedPages
}

// TODO: сейчас оно вешает весь процесс при падении здесь
// TODO: just return an error?
func (app *Application) RenderPrometheusAlertsWithGrouping(alerts *Alerts, selectedLayout appconfig.SelectedLayout) []*bytes.Buffer {
	// extract layout templating
	layoutTemplate := textTemplate.New("TelegramMessage").Funcs(app.TextTemplateFuncMap())
	layoutTemplate, err := layoutTemplate.Parse(app.config.Layouts[selectedLayout.Layout])
	if err != nil {
		log.Fatalf("Error while parsing layout %v %v", selectedLayout.Layout, err)
	}

	layoutTemplate, err = layoutTemplate.Parse(appconfig.PrometheusMessagesWrapperTemplate())
	if err != nil {
		log.Fatalf("Error while parsing DefaultPrometheusMessageTemplate: %v", appconfig.PrometheusMessagesWrapperTemplate(), err)
	}

	// extract message template
	messageTemplate := textTemplate.New("TelegramRowMessage").Funcs(app.TextTemplateFuncMap())
	//messageTemplate, err = messageTemplate.Parse(app.config.MessageTemplates[selectedLayout.MessageTemplate])
	messageTemplate, err = messageTemplate.Parse(appconfig.DefaultPrometheusGroupedMessageTemplate())
	if err != nil {
		log.Fatalf("Error while parsing message template %v %v", selectedLayout.MessageTemplate, err)
	}

	// group rendered alerts (separate from template)
	groupsWithMessages := make(map[string][]*bytes.Buffer)

	// TODO: debug messages render index

	groupTemplate, err := textTemplate.New("TextTemplate").Parse(appconfig.DefaultPrometheusGroupLabelTemplate())
	if err != nil {
		log.Fatalf("Error while parsinng group label template: %v", err)
	}

	for _, alert := range alerts.Alerts {
		renderedAlert := new(bytes.Buffer)

		// render message row partial
		if err := messageTemplate.Execute(renderedAlert, alert); err != nil {
			log.Fatalf("Error while rendering message template: %v", err)
		}

		// extract group key
		if label, ok := alert.Labels["alertname"]; ok == true {
			if labelStr, ok := label.(string); ok == true {
				// create group if not exist
				if _, ok := groupsWithMessages[labelStr]; ok == false {
					groupsWithMessages[labelStr] = make([]*bytes.Buffer, 0)

					// first row is group label
					renderedGroupLabel := new(bytes.Buffer)
					if err := groupTemplate.Execute(renderedGroupLabel, labelStr); err != nil {
						log.Fatalf("Cannot execute group label template: %v", err)
					}

					groupsWithMessages[labelStr] = append(groupsWithMessages[labelStr], renderedGroupLabel)
				}

				// append message to group
				groupsWithMessages[labelStr] = append(groupsWithMessages[labelStr], renderedAlert)
			} else {
				log.Fatalf("Typecast failed for label %v", label)
			}
		} else {
			log.Fatalf("No alertname provided inside %v", alert.Labels)
		}
	}

	// map groups + messages
	renderedMessages := make([]*bytes.Buffer, 0)
	for _, rows := range groupsWithMessages {
		for _, row := range rows {
			renderedMessages = append(renderedMessages, row)
		}
	}

	// start rendering separated pages (if required)
	renderedPages := make([]*bytes.Buffer, 0)
	renderedPages = append(renderedPages, new(bytes.Buffer))

	currentPage           := 0
	currentPageStartIndex := 0

	// TODO: Not optimal algorithm because i use incremental rendering + len check.
	// Instead calculate len, split to partitions and render in pageNumbers steps.
	for idx, _ := range renderedMessages {
		view := new(PrometheusAlertsView)
		view.PageNumber = currentPage
		view.PageMessages = renderedMessages[currentPageStartIndex:idx + 1]
		view.Alerts = alerts

		// render messages according page number and offset
		temp := new(bytes.Buffer)
		if err := layoutTemplate.Execute(temp, view); err != nil {
			log.Fatalf("Error while rendering full template: %v", err)
		}

		if (temp.Len()) <= app.config.SplitMessageBytes {
			renderedPages[currentPage].Reset()
			renderedPages[currentPage].Write(temp.Bytes())
		} else {
			// page is full, create new one
			currentPage += 1
			currentPageStartIndex = idx

			newPageView := new(PrometheusAlertsView)
			newPageView.PageNumber = currentPage
			newPageView.PageMessages = renderedMessages[currentPageStartIndex:idx + 1]
			newPageView.Alerts = alerts

			newPageTemp := new(bytes.Buffer)
			if err := layoutTemplate.Execute(newPageTemp, newPageView); err != nil {
				log.Fatalf("Error while rendering full template: %v", err)
			}

			renderedPages = append(renderedPages, new(bytes.Buffer))
			renderedPages[currentPage].Write(newPageTemp.Bytes())
			//currentPage +
		}
	}

	if app.config.Debug {
		for idx, page := range renderedPages {
			log.Printf("page %v len: %v", idx, page.Len())
		}
	}

	return renderedPages
}
