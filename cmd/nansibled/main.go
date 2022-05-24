package main

import (
	"flag"
	"os"

	"github.com/albrow/zoom"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"github.com/penguinpowernz/nansible/pkg/nansibled"
)

func main() {
	var createKey, redisURL, natsURL string
	flag.StringVar(&createKey, "create-key", "", "create a new key to access the API with")
	flag.StringVar(&redisURL, "r", os.Getenv("REDIS_URL"), "the redis URL to use")
	flag.StringVar(&natsURL, "n", os.Getenv("NATS_URL"), "the NATS URL to use")
	flag.Parse()

	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	if redisURL == "" {
		redisURL = "127.0.0.1:6379"
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		panic(err)
	}

	pool := zoom.NewPoolWithOptions(zoom.DefaultPoolOptions.WithAddress(redisURL).WithDatabase(8))

	svr := nansibled.NewServer(nc, pool)

	switch {
	case createKey != "":
		svr.CreateKey(createKey)
		return
	}

	api := gin.Default()
	svr.SetupRoutes(api)

	api.Run(":8090")
}
