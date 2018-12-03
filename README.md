# prometheus_bot

rewrite-in-progress prometheus telegram bot

## Usage

1. Create Telegram bot with [BotFather](https://t.me/BotFather), it will return your bot token

2. Create configuration `config.yaml`:

```yml
telegram_token: "token goes here"

# all templates should be defined here
templates:
  default:
    |
      <b>{{ .Annotations.message }}</b>
      <code>{{ .Labels.alertname }}</code> [ {{ .Labels.k8s }} / {{ .Labels.severity }} ]
  only_message:
    |
      <b>{{ .Annotations.message }}</b>

# do not add blank line after each alert
# NOT IMPLEMENTED: нужно это кому-то вообще?
# disable_message_line_separator:
#   - default
#   - only_message

# (chats -> template) mapping configuration
chats_templates:
  # "chatID": custom_template_name
  "-228572021": only_message
  "46733847": default

time_zone: "Europe/Rome"
split_token: "|"    
split_msg_byte: 4000
```

3. Run ```telegram_tbot```. See ```prometheus_tbot --help``` for command line options
4. Write `/chatid` command and receive ChatId

### Configuring alert manager

Alert manager configuration file:

```yml
- name: 'admins'
  webhook_configs:
  - send_resolved: True
    url: http://127.0.0.1:9087/alert/-chat_id_1/-chat_id_2/-chat_id_n
```

## TODO:
- [readme] templates
- tests