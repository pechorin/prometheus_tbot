package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"path"
	"sort"
	"strings"
	textTemplate "text/template"
)

func (app *Application) loadTemplate(tmplPath string) *template.Template {
	// let's read template
	tmpH, err := template.New(path.Base(tmplPath)).Funcs(app.TemplateFuncMap()).ParseFiles(tmplPath)

	if err != nil {
		log.Fatalf("Problem reading parsing template file: %v", err)
	} else {
		log.Printf("Load template file:%s", tmplPath)
	}

	return tmpH
}

// Create alert message from template
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

// DEPRECATED:
// Standatd formatter from old prometheus_bot
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

// TODO: не падать, а возвращать ошибку?
func (app *Application) TestFormatter(alerts *Alerts) []*bytes.Buffer {
	buffers := make([]*bytes.Buffer, 0)
	buffers = append(buffers, new(bytes.Buffer))

	currentBufferIndex := 0
	currentBuffer := buffers[currentBufferIndex]

	tmpl, err := textTemplate.New("defaultMessage").Funcs(app.TextTemplateFuncMap()).Parse(app.config.MessageTemplate)

	if err != nil {
		log.Fatalf("Problem parsing template messageMini: %v", err)
	}

	if alerts.Status == "firing" {
		currentBuffer.WriteString("Firing🔥\n")
	} else {
		currentBuffer.WriteString(alerts.Status + "\n")
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

		currentBuffer.WriteString(tempBuffer.String())
	}

	return buffers
}

// DEPRECATED:
func splitString(s string, n int) []string {
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
