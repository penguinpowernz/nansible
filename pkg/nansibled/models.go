package nansibled

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"time"
)

type playbook struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Data string `json:"data,omitempty"`
}

func (pb *playbook) EncryptedString(salt string) string {
	return encryptWithSalt(salt, pb.Data)
}

func (pb *playbook) Bytes() []byte {
	return []byte(pb.Data)
}

func (pb *playbook) MD5SUM() string {
	return fmt.Sprintf("%x", md5.Sum(pb.Bytes()))
}

func (pb playbook) ModelID() string      { return pb.ID }
func (pb *playbook) SetModelID(x string) { pb.ID = x }

type group struct {
	Name     string   `json:"name,omitempty"`
	Playbook string   `json:"playbook,omitempty" zoom:"index"`
	Hosts    []string `json:"hosts,omitempty"`
}

func (g group) ModelID() string      { return g.Name }
func (g *group) SetModelID(x string) { g.Name = x }

type host struct {
	Name                 string      `json:"name"`
	State                deployState `json:"state" zoom:"index"`
	LastDeployedAt       time.Time   `json:"last_deployed_at"`
	LastDeployedPlaybook string      `json:"last_deployed_playbook"`
	LastAckedPlaybook    string      `json:"last_acked_playbook"`
	LastAckedAt          time.Time   `json:"last_acked_at"`
	LastSuccessPlaybook  string      `json:"last_success_playbook"`
	LastSuccessAt        time.Time   `json:"last_success_at"`
	LastErrorPlaybook    string      `json:"last_error_playbook"`
	LastErrorAt          time.Time   `json:"last_error_at"`
	LastSeenAt           time.Time   `json:"last_seen_at"`
}

func (h host) ModelID() string      { return h.Name }
func (h *host) SetModelID(x string) { h.Name = x }

type NansibleMessage struct {
	Host     string `json:"host,omitempty"`
	Playbook string `json:"playbook,omitempty"`
	Payload  string `json:"payload,omitempty"`
	Deploy   string `json:"deploy,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (nsg NansibleMessage) Bytes() []byte {
	data, _ := json.Marshal(nsg)
	return data
}

func newNanMsg(host string, data []byte) NansibleMessage {
	return NansibleMessage{
		Host:    host,
		Payload: string(data),
	}
}

func ParseNanMsg(data []byte) (NansibleMessage, error) {
	m := NansibleMessage{}
	err := json.Unmarshal(data, &m)
	return m, err
}

type key struct {
	Name      string `zoom:"index"`
	Token     string
	CreatedAt time.Time
	CreatedBy string
}

func (k key) ModelID() string      { return k.Token }
func (k *key) SetModelID(x string) { k.Token = x }

// type req struct {
// 	ID string
// 	http.Request
// }

// func (k key) ModelID() string      { return k.Token }
// func (k *key) SetModelID(x string) { k.Token = x }
