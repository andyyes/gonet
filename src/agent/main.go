package main

import (
	"encoding/binary"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"time"
)

import (
	"agent/event_client"
	"agent/hub_client"
	"agent/stats_client"
	"cfg"
	"inspect"
)

func init() {
	if !flag.Parsed() {
		flag.Parse()
	}
}

const (
	TCP_TIMEOUT  = 120
	MAX_DELAY_IN = 60
)

//----------------------------------------------- Game Server Start
func main() {
	// start logger
	config := cfg.Get()
	if config["gs_log"] != "" {
		cfg.StartLogger(config["gs_log"])
	}

	// inspector
	go inspect.StartInspect()

	// dial HUB
	hub_client.DialHub()
	event_client.DialEvent()
	stats_client.DialStats()

	log.Println("Starting the server.")

	// Signal Handler
	go SignalProc()

	// SYS ROUTINE for this game server
	go SysRoutine()

	// Listen
	service := ":8080"
	if config["service"] != "" {
		service = config["service"]
	}

	log.Println("Service:", service)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	checkError(err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)

	log.Println("Game Server OK.")

	for {
		// Accept and go!
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		// test whether this IP is banned
		IP := net.ParseIP(conn.RemoteAddr().String())
		if !IsBanned(IP) {
			go handleClient(conn)
		} else {
			conn.Close()
		}
	}
}

//----------------------------------------------- start a goroutine when a new connection is accepted
func handleClient(conn net.Conn) {
	defer conn.Close()

	header := make([]byte, 2)
	ch := make(chan []byte, 10)

	go StartAgent(ch, conn)

	for {
		// read header : 2-bytes
		conn.SetReadDeadline(time.Now().Add(TCP_TIMEOUT * time.Second))
		n, err := io.ReadFull(conn, header)
		if n == 0 && err == io.EOF {
			break
		} else if err != nil {
			log.Println("error receiving header:", err)
			break
		}

		// read payload, the size of the payload is given by header
		size := binary.BigEndian.Uint16(header)
		data := make([]byte, size)
		n, err = io.ReadFull(conn, data)
		if err != nil {
			log.Println("error receiving payload:", err)
			break
		}

		// NOTICE: slice is passed by reference; don't re-use a single buffer.
		select {
		case ch <- data:
		case <-time.After(MAX_DELAY_IN * time.Second):
			log.Println("server busy or agent closed")
			return
		}
	}

	// close the channel, agent will notified by close
	close(ch)
}

func checkError(err error) {
	if err != nil {
		log.Println("Fatal error: %v", err)
		os.Exit(-1)
	}
}
