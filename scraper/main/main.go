package main

import (
	"github.com/codeuniversity/smag-mvp/kafka"
	"github.com/codeuniversity/smag-mvp/scraper"
	"github.com/codeuniversity/smag-mvp/service"
)

func main() {
	nameReaderConfig, infoWriterConfig, errWriterConfig := kafka.GetScraperConfig()

	s := scraper.New(
		kafka.NewReader(nameReaderConfig),
		kafka.NewWriter(infoWriterConfig),
		kafka.NewWriter(errWriterConfig),
	)
	service.CloseOnSignal(s)
	go s.Run()

	s.WaitUntilClosed()
}
