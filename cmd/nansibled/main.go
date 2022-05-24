package main

import (
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
)

func main() {
	nc, err := nats.Connect(nats.DefaultURL)
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
