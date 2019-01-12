# prometheus_tbot

rewrite-in-progress prometheus telegram bot

## Features
- multi-chat sending
- ability to define separate template for each chat

## Usage

1. Create Telegram bot with [BotFather](https://t.me/BotFather), it will return your bot token

2. Create configuration `config.yaml`:

```yaml
  telegram_token: "token goes here"

  # templates definitions
  # default template used for all chats by default
  templates:
    default:
      |
        <b>{{ .Annotations.message }}</b>
        <code>{{ .Labels.alertname }}</code> [ {{ .Labels.k8s }} / {{ .Labels.severity }} ]
    only_alert_message:
      |
        <b>{{ .Annotations.message }}</b>

  # assign template for specific chat 
  # (default template will be used for all other cases)
  chats_templates:
    # "chatID": custom_template_name
    "-chat1": only_alert_message
    "chat2": default
```

3. Run ```telegram_tbot``` with command lines options or env variables described in section below

4. Write `/chatid` command in any chat with tbot and receive ChatId

### Command lines options & environment variables

Any command line argument can be set through ENV variables, equality table below:

```
var:                 flag:      env:

Config Path          -c        TBOT_CONFIG_PATH
Port                 -p        TBOT_PORT
Telegram Token       -t        TBOT_TELEGRAM_TOKEN
Debug                -d        TBOT_DEBUG
```

***Examples***:

run with arguments
```
./prometheus_tbot -c path/to/config.yml -p 9000 -t TOKEN
```

run with environment variables (all vars prefixed with `TBOT`)
```
TBOT_CONFIG_PATH=path/to/config.yml TBOT_PORT=9000 TBOT_TELEGRAM_TOKEN=TOKEN ./prometheus_tbot
```

run with proxy
```
HTTP_PROXY=socks5://telegram:login@server:8080 ./prometheus_tbot -c path/to/config.yml -p 9000 -t TOKEN
```

### Configuring alert manager

Alert manager configuration file:

```yml
- name: 'admins'
  webhook_configs:
  - send_resolved: True
    url: http://127.0.0.1:9087/alert/-chat_id_1/-chat_id_2/-chat_id_n
```

## TODO:
- better crash reports
- [?] notify panic's with `honeybadger`
- [readme] templates
- write tests
- [late] add ability to test templates
