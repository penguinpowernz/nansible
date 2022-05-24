package nansibled

import (
	"github.com/albrow/zoom"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
)

type Server struct {
	nc *nats.Conn
	db *db

	running []*deploy
}

func NewServer(nc *nats.Conn, pool *zoom.Pool) *Server {
	svr := &Server{nc: nc}
	svr.db = newDB(pool)

	go svr.identifyHosts()

	return svr
}

func (svr *Server) SetupRoutes(api gin.IRouter) {
	api.Use(svr.requestAuthorizer)

	api.GET("/playbooks/", findAllModelsHandler(svr.db.playbooks, new([]*playbook)))
	api.GET("/playbooks/:name", findModelHandler(svr.db.playbooks.Find, new(playbook), "name"))
	api.DELETE("/playbooks/:name", deleteModelHandler(svr.db.playbooks))
	api.POST("/playbooks")
	api.POST("/playbooks/:name/group/:group")
	api.DELETE("/playbooks/:name/group/:group")

	api.GET("/hosts", findAllModelsHandler(svr.db.hosts, new([]*host)))
	api.PUT("/hosts/:host", findModelHandler(svr.db.hosts.Find, new(host), "name"))
	api.PUT("/hosts/:host/deploy/:playbook", svr.handleHostDeploy)
	api.POST("/hosts/:host/group/:group", svr.handleAddHostToGroup)
	api.DELETE("/hosts/:host/group/:group", svr.handleRmHostFromGroup)

	api.GET("/groups", findAllModelsHandler(svr.db.groups, new([]*group)))
	api.GET("/groups/:name", findModelHandler(svr.db.groups.Find, new(group), "name"))
	api.POST("/groups", svr.handleCreateNewGroup)
	api.DELETE("/groups/:name", deleteModelHandler(svr.db.groups))
	api.PUT("/groups/:name")
	api.POST("/groups/:name/host/:host", svr.handleAddHostToGroup)
	api.DELETE("/groups/:name/host/:host", svr.handleRmHostFromGroup)
	api.PUT("/groups/:name/playbook/:playbook", updateAttributeHandler(svr.db.groups, new(group), "playbook", "playbook"))
	api.PUT("/groups/:name/deploy", svr.handleDeployGroup)

	// api.GET("/requests", findAllModelsHandler(svr.db.reqs, new([]*http.Request)))
	api.GET("/deploys", findAllModelsHandler(svr.db.deploys, new([]*deploy)))
	api.GET("/deploys/:name", findModelHandler(svr.db.deploys.Find, new(deploy), "name"))
	api.GET("/deploys/:name/running", svr.handleRunningDeploys)
}
