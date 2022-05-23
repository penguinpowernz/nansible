package main

import (
	"bytes"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/penguinpowernz/nansible/pkg/nansibled"
)

func main() {

	host, _ := os.Hostname()
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		panic(err)
	}

	// listen for pings
	sub1, err := nc.Subscribe("nansible.ping", func(msg *nats.Msg) { nc.Publish("nansible.pong", []byte(host)) })
	if err != nil {
		panic(err)
	}
	defer sub1.Unsubscribe()

	// listen for deployments
	msgs := make(chan *nats.Msg)
	sub2, err := nc.ChanSubscribe("nansible."+host+".playbook", msgs)
	if err != nil {
		panic(err)
	}
	defer sub2.Unsubscribe()

	dp := deployer{}

	for msg := range msgs {
		in, err := nansibled.ParseNanMsg(msg.Data)
		if err != nil {
			nc.Publish(msg.Reply, nil)
			continue
		}

		pb := decryptPlaybook([]byte(in.Playbook))
		sum := md5PB(pb)

		// ack
		nc.Publish(msg.Reply, []byte(sum))

		// do deploy
		out, err := dp.Deploy(pb)

		// nsg := nansibled.NansibleMessage{
		// 	Host: host,
		// }

		// ack success or error
		if err != nil {
			nc.Publish("nansible."+host+".playbook.error", out)
		}

		nc.Publish("nansible."+host+".playbook.success", out)
	}
}

type deployer struct {
	mu   *sync.Mutex
	curr *os.Process
}

func (dp deployer) Cancel() {
	if dp.curr == nil {
		return
	}
	dp.curr.Signal(syscall.SIGTERM)
	dp.curr.Wait()
}

func (dp deployer) Deploy(yml string) ([]byte, error) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	cmd := exec.Command("ansible-playbook", "/etc/nansible/current", "-i", "127.0.0.1,")

	buf := bytes.NewBufferString("")
	cmd.Stdout = buf
	cmd.Stderr = buf

	err := cmd.Start()
	dp.curr = cmd.Process

	err = cmd.Wait()
	return buf.Bytes(), err
}

func md5PB(in string) string {
	return ""
}

func decryptPlaybook(data []byte) string {
	return string(data)
}
