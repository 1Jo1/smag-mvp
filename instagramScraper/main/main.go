package main

import (
	"os"
	"test95/instagramScraper"
	"test95/service"
)

func main() {
	kafkaAddress := os.Getenv("KAFKA_ADDRESS")
	if kafkaAddress == "" {
		kafkaAddress = "localhost:9092"
	}
	s := instagramScraper.New(kafkaAddress)
	service.CloseOnSignal(s)
	go s.Run()

	s.WaitUntilClosed()
}
