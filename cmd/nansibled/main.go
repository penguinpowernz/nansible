package main

import (
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
)

func main() {
	nc, err := nats.Connect(nats.DefaultURL)

	api := gin.Default()
	svr := nansibled.NewServer(nc)
	svr.SetupRoutes(api)

	api.Run(":8090")
}
