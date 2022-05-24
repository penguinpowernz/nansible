package nansibled

import (
	"github.com/albrow/zoom"
)

type db struct {
	hosts     *zoom.Collection
	playbooks *zoom.Collection
	groups    *zoom.Collection
	// reqs      *zoom.Collection
	deploys *zoom.Collection
	keys    *zoom.Collection
}

func newDB(pool *zoom.Pool) *db {
	ignoreErr := func(c *zoom.Collection, err error) *zoom.Collection {
		if err != nil {
			panic(err)
		}
		return c
	}

	return &db{
		hosts:     ignoreErr(pool.NewCollectionWithOptions(new(host), zoom.DefaultCollectionOptions.WithIndex(true))),
		playbooks: ignoreErr(pool.NewCollectionWithOptions(new(playbook), zoom.DefaultCollectionOptions.WithIndex(true))),
		groups:    ignoreErr(pool.NewCollectionWithOptions(new(group), zoom.DefaultCollectionOptions.WithIndex(true))),
		deploys:   ignoreErr(pool.NewCollectionWithOptions(new(deploy), zoom.DefaultCollectionOptions.WithIndex(true))),
		keys:      ignoreErr(pool.NewCollectionWithOptions(new(key), zoom.DefaultCollectionOptions.WithIndex(true))),
		// reqs:      ignoreErr(pool.NewCollectionWithOptions(new(http.Request), zoom.DefaultCollectionOptions.WithIndex(true))),
	}
}
