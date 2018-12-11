package appconfig

import (
	"flag"
	"fmt"
	configLoader "github.com/micro/go-config"
	yamlEncoder "github.com/micro/go-config/encoder/yaml"
	configSource "github.com/micro/go-config/source"
	configLoaderEnv "github.com/micro/go-config/source/env"
	configLoaderFile "github.com/micro/go-config/source/file"
	"log"
	"strings"
)

/*
	var:                 flag:      env:

	ConfigPath           -c         CONFIG_PATH
	Port                 -p         PORT
	TelegramToken (?)    -t         TELEGRAM_TOKEN
	Debug                -d         DEBUG
*/

// Маппинг полей структуры Config и флагов командной строки
const (
	ConfigPathFlag    = "c"
	PortFlag          = "p"
	DebugFlag         = "d"
	TelegramTokenFlag = "t"
)

// Все environment переменные должны начинаться с EnvPrefix
const (
	EnvPrefix = "TBOT"
)

const ()

// Config является состоянием конфигурации приложения
type Config struct {
	ConfigPath        string
	Port              string            `json:"port"`
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
	tmpConfigPath    := flag.String(ConfigPathFlag, "", "Path to config file")
	tmpPort          := flag.String(PortFlag, "9000", "Web server listen address") // rename to port?
	tmpTelegramToken := flag.String(TelegramTokenFlag, "", "Telegram token")
	tmpDebug         := flag.Bool(DebugFlag, false, "Debug mode")

	flag.Parse()

	app.ConfigPath    = *tmpConfigPath
	app.Port          = *tmpPort
	app.TelegramToken = *tmpTelegramToken
	app.Debug         = *tmpDebug

	if app.Debug {
		log.Println("Config after flags init", app)
	}

	// merge from environment
	envConfig := configLoader.NewConfig()
	envSrc := configLoaderEnv.NewSource(configLoaderEnv.WithStrippedPrefix(EnvPrefix))
	envConfig.Load(envSrc)

	if newConfigPath := envConfig.Get("config", "path").String(""); newConfigPath != "" {
		app.ConfigPath = newConfigPath
	}

	if newPort := envConfig.Get("port").String(""); newPort != "" {
		app.Port = newPort
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
	fileSrc := configLoaderFile.NewSource(configLoaderFile.WithPath(app.ConfigPath), configSource.WithEncoder(yamlEncoderInstance))
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

	if !strings.HasPrefix(app.Port, ":") {
		app.Port = ":" + app.Port
	}

	if app.SplitMessageBytes == 0 {
		app.SplitMessageBytes = 4000
	}

	if app.Debug {
		fmt.Printf("Config: %v\n", app)
	}

	if app.TelegramToken == "" {
		log.Fatalln("No Telegram token provided")
	}

	return app
}

func defaultMessageTemplate() string {
	return `
<b>{{ .Annotations.message }}</b>
<code>{{ .Labels.alertname }}</code> [ {{ .Labels.k8s }} / {{ .Labels.severity }} ]`
}

func main() {}
