package nansibled

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (svr *Server) requestAuthorizer(c *gin.Context) {
	var token string
	setToken := func(t string) (wasSet bool) {
		if t != "" {
			token = t
			wasSet = true
		}
		return
	}

	switch {
	case setToken(c.GetHeader("Authorization")):
	case setToken(c.GetHeader("X-Api-Key")):
	case setToken(c.GetHeader("X-API-KEY")):
	case setToken(c.Query("api_key")):
	case setToken(c.Query("apikey")):
	case setToken(c.Query("token")):
	}

	token = strings.ReplaceAll(token, "Bearer ", "")

	if token == "" {
		c.AbortWithError(401, errors.New("token not found in request"))
		return
	}

	var k key
	if err := svr.db.keys.Find(token, &k); err != nil {
		c.AbortWithError(401, err)
		return
	}

	c.Set("user", k.Name)
}

func (svr *Server) handleRmHostFromGroup(c *gin.Context) { c.AbortWithStatus(501) }

func (svr *Server) handleHostDeploy(c *gin.Context) {
	var h *host
	if err := svr.db.hosts.Find(c.Param("name"), h); err != nil {
		abortWithError(c, 500, err)
		return
	}

	var pb *playbook
	if err := svr.db.playbooks.Find(c.Param("playbook"), h); err != nil {
		abortWithError(c, 500, err)
		return
	}

	dply := newDeploy(svr.nc, h, pb)
	if err := svr.db.deploys.Save(dply); err != nil {
		abortWithError(c, 500, err)
		return
	}

	go dply.Start(5, time.Second*5) // 25 sec timeout
	<-dply.Acked()
	if !dply.ErrorAt.IsZero() {
		abortWithError(c, 504, errors.New(dply.Error))
		return
	}

	dply.OnSync(func(hst *host, dply *deploy) {
		svr.db.hosts.Save(dply.hst)
		svr.db.deploys.Save(dply)
	})

	c.JSON(202, map[string]string{"id": dply.ID})
}

func (svr *Server) handleAddHostToGroup(c *gin.Context) {
	name := c.Param("group")
	host := c.Param("host")

	var g group
	if err := svr.db.groups.Find(name, &g); err != nil {
		c.AbortWithError(500, err)
		return
	}

	g.Hosts = append(g.Hosts, host)
}

func (svr *Server) handleCreateNewGroup(c *gin.Context) {
	var g *group
	if err := c.BindJSON(g); err != nil {
		abortWithError(c, 400, err)
		return
	}

	if g.Name == "" {
		c.AbortWithStatus(400)
		return
	}

	if err := svr.db.groups.Save(g); err != nil {
		abortWithError(c, 500, err)
		return
	}

	c.JSON(201, g)
}

func (svr *Server) handleDeployGroup(c *gin.Context) {
	name := c.Param("name")
	var g *group
	if err := svr.db.groups.Find(name, g); err != nil {
		abortWithError(c, 500, err)
		return
	}

	if g.Playbook == "" {
		abortWithError(c, 400, errors.New("group does not have a playbook assigned"))
		return
	}

	var pb *playbook
	if err := svr.db.playbooks.Find(g.Playbook, pb); err != nil {
		abortWithError(c, 500, err)
		return
	}

	res := map[string]map[string]string{}
	res["errors"] = map[string]string{}
	res["started"] = map[string]string{}
	for _, hostname := range g.Hosts {
		var h *host
		if err := svr.db.hosts.Find(hostname, h); err != nil {
			// TODO: log
			res["errors"][hostname] = err.Error()
			continue
		}

		dply := newDeploy(svr.nc, h, pb)
		if err := svr.db.deploys.Save(dply); err != nil {
			res["errors"][hostname] = err.Error()
			continue
		}

		dply.OnSync(func(hst *host, dply *deploy) {
			svr.db.hosts.Save(dply.hst)
			svr.db.deploys.Save(dply)
		})

		go dply.Start(5, time.Second*5)
		svr.running = append(svr.running, dply)
		res["started"][hostname] = dply.ID
	}

	code := 202
	if len(res["started"]) == 0 {
		code = 500
	}
	c.JSON(code, res)
}

func (svr *Server) handleRunningDeploys(c *gin.Context) {
	ids := []string{}
	for _, dpy := range svr.running {
		if dpy.FinishedAt.IsZero() {
			ids = append(ids, dpy.ID)
		}
	}
	c.JSON(200, ids)
}
