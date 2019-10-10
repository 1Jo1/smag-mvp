package main

import (
	"github.com/codeuniversity/smag-mvp/insta-posts-inserter"
	"github.com/codeuniversity/smag-mvp/service"
	"github.com/codeuniversity/smag-mvp/utils"
)

func main() {
	postgresHost := utils.GetStringFromEnvWithDefault("POSTGRES_HOST", "127.0.0.1")
	kafkaAddress := utils.GetStringFromEnvWithDefault("KAFKA_ADDRESS", "52.58.171.160:9092")

	i := insta_posts_inserter.New(kafkaAddress, postgresHost)

	service.CloseOnSignal(i)
	go i.Run()

	i.WaitUntilClosed()
}
