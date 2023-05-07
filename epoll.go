package main

import (
	"fmt"
	"net"
	"reflect"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

type subscriber struct {
	connection *net.Conn
	room       []string
}

type epoll struct {
	fd          int
	connections map[int]subscriber
	lock        *sync.RWMutex
}

func MkEpoll() (*epoll, error) {
	fd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	return &epoll{
		fd:          fd,
		lock:        &sync.RWMutex{},
		connections: make(map[int]subscriber),
	}, nil
}

func (s *subscription) add_next(new_sub *subscription) {
	if s.next == nil {
		s.next = new_sub
	} else {
		s.next.add_next(new_sub)
	}
}

func (s *subscription) remove_next(conn *net.Conn) {
	if s.next.conn == conn {
		s.next = s.next.next
	} else {
		s.next.remove_next(conn)
	}
}

func (e *epoll) Add(conn net.Conn, room string) error {
	fd := websocketFD(conn)
	err := unix.EpollCtl(e.fd, syscall.EPOLL_CTL_ADD, fd, &unix.EpollEvent{Events: unix.POLLIN | unix.POLLHUP, Fd: int32(fd)})
	if err != nil {
		return err
	}
	e.lock.Lock()
	defer e.lock.Unlock()

	e.connections[fd] = subscriber{
		&conn,
		[]string{room},
	}
	fmt.Println("Before")
	fmt.Println(room)

	subs := subscription{&conn, nil}
	if hub[room] == nil {
		hub[room] = &subs
	} else {
		hub[room].add_next(&subs)
	}

	fmt.Println(room)
	fmt.Println(hub[room])
	return nil
}

func (e *epoll) Remove(sub subscriber) error {
	fd := websocketFD(*sub.connection)
	err := unix.EpollCtl(e.fd, syscall.EPOLL_CTL_DEL, fd, nil)
	if err != nil {
		return err
	}
	e.lock.Lock()
	defer e.lock.Unlock()
	for _, room := range sub.room {
		hub[room].remove_next(sub.connection)

		fmt.Println(hub[room])
	}
	delete(e.connections, fd)

	return nil
}

func (e *epoll) Wait() ([]subscriber, error) {
	events := make([]unix.EpollEvent, 100)
	n, err := unix.EpollWait(e.fd, events, 100)
	if err != nil {
		return nil, err
	}
	e.lock.RLock()
	defer e.lock.RUnlock()
	var connections []subscriber
	for i := 0; i < n; i++ {
		conn := e.connections[int(events[i].Fd)]
		connections = append(connections, conn)
	}
	return connections, nil
}

func websocketFD(conn net.Conn) int {
	tcpConn := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn")
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")

	return int(pfdVal.FieldByName("Sysfd").Int())
}
