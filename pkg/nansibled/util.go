package nansibled

import (
	"encoding/json"
	"errors"

	"github.com/Jeffail/gabs"
	"github.com/albrow/zoom"
	"github.com/gin-gonic/gin"
)

type findAller interface{ FindAll(interface{}) error }

func updateAttributeHandler(db *zoom.Collection, model interface{}, param, attr string) func(*gin.Context) {
	return func(c *gin.Context) {
		id := c.Param("name")

		var err error
		switch v := model.(type) {
		case *group:
			err = db.Find(id, v)
		}

		if err != nil {
			abortWithError(c, 500, err)
			return
		}

		v := c.Param(param)

		x, err := gabs.Consume(model)
		if err != nil {
			abortWithError(c, 500, err)
			return
		}

		x.SetP(v, attr)
		data := x.Bytes()
		if err = json.Unmarshal(data, model); err != nil {
			switch v := model.(type) {
			case *group:
				err = db.Save(v)
			default:
				err = errors.New("unknown object type")
			}
		}

		if err != nil {
			abortWithError(c, 500, err)
			return
		}

		c.JSON(200, data)
	}
}

func findAllModels(c *gin.Context, db findAller, models interface{}) {
	if err := db.FindAll(models); err != nil {
		abortWithError(c, 500, err)
		return
	}
	switch v := models.(type) {
	case *[]*host:
		if len(*v) == 0 {
			models = []string{}
		}
	case *[]*deploy:
		if len(*v) == 0 {
			models = []string{}
		}
	case *[]*playbook:
		if len(*v) == 0 {
			models = []string{}
		}
	case *[]*group:
		if len(*v) == 0 {
			models = []string{}
		}
	}
	c.JSON(200, models)
}

func findAllModelsHandler(db findAller, models interface{}) func(c *gin.Context) {
	return func(c *gin.Context) {
		findAllModels(c, db, models)
	}
}

func deleteModelHandler(db *zoom.Collection) func(c *gin.Context) {
	return func(c *gin.Context) {
		id := c.Param("name")

		ok, err := db.Delete(id)
		if err != nil {
			abortWithError(c, 500, err)
			return
		}

		if !ok {
			c.AbortWithStatus(404)
			return
		}

		c.Status(204)
	}
}

func encryptWithSalt(salt, playbook string) string {
	return playbook
}

func abortWithError(c *gin.Context, code int, err error) {
	c.Error(err)
	c.AbortWithStatusJSON(code, map[string]string{"error": err.Error()})
}

func findModelHandler(find func(string, zoom.Model) error, model interface{}, param string) func(c *gin.Context) {
	return func(c *gin.Context) {
		id := c.Param(param)
		var err error
		switch v := model.(type) {
		case *playbook:
			err = find(id, v)
		case *host:
			err = find(id, v)
		case *deploy:
			err = find(id, v)
		case *group:
			err = find(id, v)
		}

		if err != nil {
			abortWithError(c, 500, err)
			return
		}

		c.JSON(200, model)
	}
}
