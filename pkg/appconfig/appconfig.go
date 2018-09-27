package appconfig

import (
	"fmt"
	"log"

	configLoader "github.com/micro/go-config"
	yamlEncoder "github.com/micro/go-config/encoder/yaml"
	configSource "github.com/micro/go-config/source"
	configLoaderEnv "github.com/micro/go-config/source/env"
	configLoaderFile "github.com/micro/go-config/source/file"
	configLoaderFlag "github.com/micro/go-config/source/flag"
)

/*
	var:                 flag:      env:

	Path                 -c         CONFIG_PATH
	ListenAddr           -l         LISTEN_ADDRESS
	TemplatePath (?)     -t         TEMPLATE_PATH
	Debug                -d         DEBUG
*/

// Маппинг полей структуры Config и флагов командной строки
const (
	PathFlag         = "c"
	ListenAddrFlag   = "l"
	TemplatePathFlag = "t"
	DebugFlag        = "d"
)

// Все environment переменные должны начинаться с EnvPrefix
const (
	EnvPrefix = "RT_BOT"
)

const ()

// Config является состоянием конфигурации приложения
type Config struct {
	Path              string
	ListenAddr        string `json:"listen_addr"`
	Debug             bool   `json:"debug"`
	TelegramToken     string `json:"telegram_token"`
	TemplatePath      string `json:"template_path"`
	TimeZone          string `json:"time_zone"`
	TimeOutFormat     string `json:"time_outdata"`
	SplitChart        string `json:"split_token"`
	SplitMessageBytes int    `json:"split_msg_byte"`
}

// New создает конфиг с дефолтными значенииями
func New() *Config {
	cfg := new(Config)
	cfg.SplitMessageBytes = 4000

	return cfg
}

// Setup не только создает, но и инициализирует значения из:
// 1. флагов
// 2. env переменных
// 3. конфиг файла
func Setup() *Config {
	config := configLoader.NewConfig()
	src := configLoaderFlag.NewSource()
	config.Load(src)

	app := New()

	app.Path = config.Get(PathFlag).String("")
	app.ListenAddr = config.Get(ListenAddrFlag).String(":9000")
	app.TemplatePath = config.Get(TemplatePathFlag).String("")
	app.Debug = config.Get(ListenAddrFlag).Bool(false)

	// теперь сверху накладываем ENV переменные
	envConfig := configLoader.NewConfig()
	envSrc := configLoaderEnv.NewSource(configLoaderEnv.WithStrippedPrefix(EnvPrefix))
	envConfig.Load(envSrc)

	if newPath := envConfig.Get("config", "path").String(""); newPath != "" {
		fmt.Println("load config from ENV -> " + newPath)
		app.Path = newPath
	}

	if newListenAddr := envConfig.Get("listen", "addr").String(""); newListenAddr != "" {
		app.ListenAddr = newListenAddr
	}

	if newTemplatePath := envConfig.Get("template", "path").String(""); newTemplatePath != "" {
		app.TemplatePath = newTemplatePath
	}

	if newDebug := envConfig.Get("debug").Bool(false); newDebug != false {
		app.Debug = newDebug
	}

	yamlConfig := configLoader.NewConfig()
	yamlEncoderInstance := yamlEncoder.NewEncoder()
	fileSrc := configLoaderFile.NewSource(configLoaderFile.WithPath(app.Path), configSource.WithEncoder(yamlEncoderInstance))
	yamlConfig.Load(fileSrc)

	if err := yamlConfig.Scan(app); err != nil {
		fmt.Println("error while scan")
		log.Fatal(err)
	}

	fmt.Printf("APP after all steps -> %v\n", app)

	return app
}

func main() {}
