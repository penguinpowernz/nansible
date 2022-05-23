package nansibled

import (
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

func (svr *Server) identifyHosts() {
	for {
		svr.identifyAndSave()
		time.Sleep(time.Minute)
	}
}

func (svr *Server) identifyAndSave() {
	go func() {
		time.Sleep(time.Second / 2)
		svr.nc.Publish("nansible.ping", nil)
	}()

	msgs := make(chan *nats.Msg, 1000)
	sub, _ := svr.nc.ChanSubscribe("nansible.pong", msgs)
	defer func() {
		sub.Unsubscribe()
		close(msgs)
	}()

	var hosts []string
	func() {
		deadline := time.After(2 * time.Second)
		for {
			select {
			case <-deadline:
				return
			case msg := <-msgs:
				hosts = append(hosts, string(msg.Data))
			}
		}
	}()

	for _, h := range hosts {
		found, err := svr.db.hosts.Exists(h)
		if err != nil {
			log.Println("ERROR: identifyAndSave(): ", err)
			continue
		}

		if found {
			if err := svr.db.hosts.SaveFields([]string{"LastSeenAt"}, &host{Name: h, LastSeenAt: time.Now()}); err != nil {
				log.Println("ERROR: identifyAndSave(): ", err)
			}
			continue
		}

		// create any that don't exist
		hst := host{
			Name:       h,
			State:      stateNew,
			LastSeenAt: time.Now(),
		}

		if err := svr.db.hosts.Save(&hst); err != nil {
			log.Println("ERROR: identifyAndSave(): ", err)
		}
	}
}
