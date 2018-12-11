package appconfig

import (
	"fmt"
	configLoader "github.com/micro/go-config"
	yamlEncoder "github.com/micro/go-config/encoder/yaml"
	configSource "github.com/micro/go-config/source"
	configLoaderEnv "github.com/micro/go-config/source/env"
	configLoaderFile "github.com/micro/go-config/source/file"
	configLoaderFlag "github.com/micro/go-config/source/flag"
	"log"
	"strings"
)

/*
	var:                 flag:      env:

	Path                 -c         CONFIG_PATH
	ListenAddr           -l         LISTEN_ADDRESS
	TelegramToken (?)    -t         TELEGRAM_TOKEN
	Debug                -d         DEBUG
*/

// Маппинг полей структуры Config и флагов командной строки
const (
	PathFlag         = "c"
	ListenAddrFlag   = "l"
	DebugFlag        = "d"
	TokenFlag        = "t"
)

// Все environment переменные должны начинаться с EnvPrefix
const (
	EnvPrefix = "TBOT"
)

const ()

// Config является состоянием конфигурации приложения
type Config struct {
	Path              string
	ListenAddr        string            `json:"listen_addr"`
	Debug             bool              `json:"debug"`
	TelegramToken     string            `json:"telegram_token"`
	TimeZone          string            `json:"time_zone"`
	TimeOutFormat     string            `json:"time_outdata"`
	SplitMessageBytes int               `json:"split_msg_byte"`
	Templates         map[string]string `json:"templates"`
	ChatsTemplates    map[string]string `json:"chats_templates"`
}

// New() создает новый сетап конфига и инициализирует значения из:
// 1. флагов
// 2. env переменных
// 3. конфиг файла
func New() *Config {
	app := new(Config)

	// init from args
	config := configLoader.NewConfig()
	flagSrc := configLoaderFlag.NewSource()
	config.Load(flagSrc)

	//log.Println("map->", config.Map())

	app.Path = config.Get(PathFlag).String("")
	app.ListenAddr = config.Get(ListenAddrFlag).String(":9000")
	app.Debug = config.Get(DebugFlag).Bool(false)
	app.TelegramToken = config.Get(TokenFlag).String("")

	//log.Println("debug app:", app)

	// merge from environment
	envConfig := configLoader.NewConfig()
	envSrc := configLoaderEnv.NewSource(configLoaderEnv.WithStrippedPrefix(EnvPrefix))
	envConfig.Load(envSrc)

	if newPath := envConfig.Get("config", "path").String(""); newPath != "" {
		app.Path = newPath
	}

	if newListenAddr := envConfig.Get("listen", "address").String(""); newListenAddr != "" {
		app.ListenAddr = newListenAddr
	}

	if newTelegramToken := envConfig.Get("telegram", "token").String(""); newTelegramToken != "" {
		app.TelegramToken = newTelegramToken
	}

	if newDebug := envConfig.Get("debug").Bool(false); newDebug != false {
		app.Debug = newDebug
	}

	// merge from config file
	yamlConfig := configLoader.NewConfig()
	yamlEncoderInstance := yamlEncoder.NewEncoder()
	fileSrc := configLoaderFile.NewSource(configLoaderFile.WithPath(app.Path), configSource.WithEncoder(yamlEncoderInstance))
	yamlConfig.Load(fileSrc)

	if err := yamlConfig.Scan(app); err != nil {
		fmt.Println("error while scan")
		log.Fatal(err)
	}

	// finalize configuration

	if len(app.Templates) == 0 {
		if app.Templates == nil {
			app.Templates = make(map[string]string)
		}

		app.Templates["default"] = defaultMessageTemplate()
	}

	if !strings.HasPrefix(app.ListenAddr, ":") {
		app.ListenAddr = ":" + app.ListenAddr
	}

	if app.SplitMessageBytes == 0 {
		app.SplitMessageBytes = 4000
	}

	if app.Debug {
		fmt.Printf("Config: %v\n", app)
	}

	return app
}

func defaultMessageTemplate() string {
	return `
    <b>{{ .Annotations.message }}</b>
    <code>{{ .Labels.alertname }}</code> [ {{ .Labels.k8s }} / {{ .Labels.severity }} ]`
}

func main() {}
