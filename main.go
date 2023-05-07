package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"syscall"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/labstack/echo/v4"
)

var epoller *epoll
var hub map[string]*subscription

type subscription struct {
	conn *net.Conn
	next *subscription
}

func wsstart(c echo.Context) error {
	log.Println("Adding Connection")
	room := c.Param("room")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return err
	}
	conn, _, _, err := ws.UpgradeHTTP(c.Request(), c.Response())
	if err != nil {
		return err
	}
	if err := epoller.Add(conn, room, id); err != nil {
		log.Printf("Failed to add connection %v", err)
		conn.Close()
	}
	fmt.Println(conn)
	return nil
}

func main() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}

	hub = make(map[string]*subscription, 0)

	var err error
	epoller, err = MkEpoll()
	if err != nil {
		panic(err)
	}

	go Start()

	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	e.GET("/ws/:room/:id", wsstart)
	e.Logger.Fatal(e.Start(":6969"))
}

func (s *subscription) send_msg_in_room(op ws.OpCode, msg []byte) {
	err := wsutil.WriteServerMessage(*s.conn, op, msg)
	if err != nil {
		log.Printf("Failed to send %v", err)
	}

	if s.next != nil {
		s.next.send_msg_in_room(op, msg)
	}
}

func Start() {
	for {
		subscriptions, err := epoller.Wait()
		if err != nil {
			log.Printf("Failed to epoll wait %v", err)
			continue
		}
		for _, subs := range subscriptions {
			if subs.connection == nil {
				break
			}
			if msg, op, err := wsutil.ReadClientData(*subs.connection); err != nil {
				fmt.Println("Here!")
				fmt.Println(err)
				if err := epoller.Remove(subs); err != nil {
					log.Printf("Failed to remove %v", err)
				}
				con := *subs.connection
				con.Close()
			} else {
				for _, room := range subs.room {
					if hub[room] != nil {
						hub[room].send_msg_in_room(op, msg)
						log.Printf("msg: %d %s", epoller.fd, string(msg))
					}
				}

			}
		}
	}
}
