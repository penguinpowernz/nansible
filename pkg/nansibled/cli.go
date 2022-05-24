package nansibled

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"time"
)

func (svr *Server) CreateKey(name string) {
	k := key{Name: name, Token: makeToken(), CreatedAt: time.Now(), CreatedBy: os.Getenv("USER") + "@localhost"}
	if err := svr.db.keys.Save(&k); err != nil {
		log.Println("ERROR:", err)
		return
	}
	fmt.Printf("New token for %s is: %s\n", name, k.Token)
}

func (svr *Server) DeleteKey(name string) {
	var k key
	if err := svr.db.keys.NewQuery().Filter("Name =", name).RunOne(&k); err != nil {
		log.Println("ERROR:", err)
		return
	}
	ok, err := svr.db.keys.Delete(k.Token)
	if err != nil {
		log.Println("ERROR:", err)
		return
	}
	switch ok {
	case true:
		fmt.Println("Deleted")
	default:
		fmt.Printf("Key not found with name '%s'\n", name)
	}
}

func (svr *Server) ListKeys(name string) {
	var keys []*key
	if err := svr.db.keys.FindAll(&keys); err != nil {
		log.Println("ERROR:", err)
	}

	for _, k := range keys {
		fmt.Printf("%10s %s\n", k.CreatedAt, k.Name)
	}
}

func (svr *Server) RotateKey(name string) {
	var k key
	if err := svr.db.keys.NewQuery().Filter("Name =", name).RunOne(&k); err != nil {
		log.Println("ERROR:", err)
		return
	}
	k.Token = makeToken()
	if err := svr.db.keys.Save(&k); err != nil {
		log.Println("ERROR:", err)
		return
	}

	fmt.Printf("New token for %s is: %s\n", name, k.Token)
}

func makeToken() string {
	t := make([]byte, 32)
	rand.Read(t)
	return fmt.Sprintf("%x", t)
}
