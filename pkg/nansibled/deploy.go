package nansibled

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
)

type deployState string

var (
	stateNew     = deployState("")
	stateSent    = deployState("sent")
	stateAcked   = deployState("acked")
	stateSuccess = deployState("success")
	stateError   = deployState("error")

	maxDeployTime = 30 * time.Minute
)

type deploy struct {
	ID         string
	StartedAt  time.Time
	FinishedAt time.Time
	State      deployState // new,sent,acked,error,success
	Host       string
	Playbook   string
	SuccessAt  time.Time
	ErrorAt    time.Time
	AckedAt    time.Time
	Error      string

	hst    *host
	pb     *playbook
	ctx    context.Context
	nc     *nats.Conn
	done   chan struct{}
	onSync func(*host, *deploy)
}

func newDeploy(nc *nats.Conn, hst *host, pb *playbook) *deploy {
	d := new(deploy)
	d.ID = "abcd" //uuid.New().String()
	d.hst = hst
	d.pb = pb
	d.Host = hst.Name
	d.Playbook = pb.Name
	d.nc = nc
	d.done = make(chan struct{})
	d.onSync = func(*host, *deploy) {}
	return d
}

// Acked will block until the device acks the deploy payload
func (dpy *deploy) Acked() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for dpy.AckedAt.IsZero() && dpy.ErrorAt.IsZero() {
			time.Sleep(time.Second / 10)
		}
		close(ch)
	}()
	return ch
}

func (dpy *deploy) Done() <-chan struct{} {
	return dpy.done
}

func (dpy *deploy) OnSync(cb func(*host, *deploy)) {
	dpy.onSync = cb
}

func (dpy *deploy) Start(retries int, interval time.Duration) {
	dpy.StartedAt = time.Now()
	defer close(dpy.done)
	defer func() { dpy.FinishedAt = time.Now() }()
	defer dpy.onSync(dpy.hst, dpy)

	dpy.hst.LastDeployedPlaybook = dpy.pb.Name
	dpy.onSync(dpy.hst, dpy)

	for retries > 0 {
		dpy.State = stateSent
		dpy.hst.State = stateSent
		dpy.hst.LastDeployedAt = time.Now()
		dpy.onSync(dpy.hst, dpy)

		nsg := NansibleMessage{}
		nsg.Host = dpy.hst.Name
		nsg.Playbook = dpy.Playbook
		nsg.Payload = dpy.pb.EncryptedString(dpy.hst.Name)
		nsg.Deploy = dpy.ID

		msg, err := dpy.nc.Request("nansible."+dpy.hst.Name+".playbook", nsg.Bytes(), interval)
		if err == nil {
			dpy.State = stateAcked
			dpy.hst.State = stateAcked
			dpy.hst.LastAckedAt = time.Now()
			dpy.hst.LastAckedPlaybook = string(msg.Data)
			break
		}

		// bail out after 1 hour of waiting for ack
		// inifinite retries (retries=0) are not actually infinite
		if time.Since(dpy.StartedAt) > time.Hour {
			dpy.State = stateError
			dpy.hst.State = stateError
			dpy.ErrorAt = time.Now()
			dpy.Error = "abandoned"
			dpy.hst.LastErrorAt = dpy.ErrorAt
			dpy.hst.LastErrorPlaybook = ""
			return
		}

		retries--
	}
	dpy.onSync(dpy.hst, dpy)

	ctx, cancel := context.WithTimeout(context.Background(), maxDeployTime)
	sub1, _ := dpy.nc.Subscribe("nansible."+dpy.hst.Name+".playbook.success", func(msg *nats.Msg) {
		defer cancel()
		dpy.hst.LastSuccessAt = time.Now()
		dpy.hst.LastSuccessPlaybook = string(msg.Data)
		dpy.State = stateSuccess
		dpy.hst.State = stateSuccess
	})
	defer sub1.Unsubscribe()

	sub2, _ := dpy.nc.Subscribe("nansible."+dpy.hst.Name+".playbook.error", func(msg *nats.Msg) {
		defer cancel()
		dpy.hst.LastErrorAt = time.Now()
		dpy.hst.LastErrorPlaybook = string(msg.Data)
		dpy.Error = "host error"
		dpy.State = stateError
		dpy.hst.State = stateError
	})
	defer sub2.Unsubscribe()
	<-ctx.Done()
}

func (dpy deploy) ModelID() string      { return dpy.ID }
func (dpy *deploy) SetModelID(x string) { dpy.ID = x }
