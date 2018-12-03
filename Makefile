TARGET=prometheus_tbot

all: main.go
	go build -o $(TARGET)
test: all
	prove -v
clean:
	go clean
	rm -f $(TARGET)
	rm -f bot.log
