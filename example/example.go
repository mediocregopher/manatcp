package main

import (
	"bufio"
	"bytes"
	"github.com/mediocregopher/manatcp"
	"log"
	"time"
)

////////////////////////////////////////////////////////////////////////////////
// Client definition

type ExClient struct{}

func (_ ExClient) Read(buf *bufio.Reader) (interface{}, error, bool) {
	l, err := buf.ReadBytes('\n')
	if err != nil {
		return nil, err, true
	}
	tl := bytes.TrimRight(l, "\n")
	return tl, nil, false
}

func (_ ExClient) IsPush(li interface{}) bool {
	l := li.([]byte)
	return l[0] != '~'
}

func (_ ExClient) Write(buf *bufio.Writer, item interface{}) (error, bool) {
	if _, err := buf.Write(item.([]byte)); err != nil {
		return err, true
	}
	if _, err := buf.Write([]byte{'\n'}); err != nil {
		return err, true
	}
	return nil, false
}

////////////////////////////////////////////////////////////////////////////////
// Server definition

type ExServer struct{}

func (_ ExServer) Connected(
	lc *manatcp.ListenerConn) (manatcp.ServerClient, bool) {
	log.Println("SERVER: new client")
	go serverClientSpin(lc)
	return &ExServerClient{}, false
}

type ExServerClient struct{}

func (_ ExServerClient) Read(buf *bufio.Reader) (interface{}, bool) {
	l, err := buf.ReadBytes('\n')
	if err != nil {
		return nil, true
	}
	tl := bytes.TrimRight(l, "\n")
	return tl, false
}

func (_ ExServerClient) Write(buf *bufio.Writer, item interface{}) bool {
	if _, err := buf.Write(item.([]byte)); err != nil {
		log.Println(err)
		return true
	}
	if _, err := buf.Write([]byte{'\n'}); err != nil {
		log.Println(err)
		return true
	}
	return false
}

func (_ ExServerClient) HandleCmd(cmd interface{}) (interface{}, bool, bool) {
	log.Printf("SERVER: received '%s' from the client", string(cmd.([]byte)))
	cmdB := cmd.([]byte)
	res := make([]byte, 1, len(cmdB)+1)
	res[0] = '~'
	res = append(res, cmdB...)
	log.Printf("SERVER: sending back '%s'", string(res))
	return res, true, false
}

func (_ ExServerClient) Closing() {
	log.Println("SERVER: closing the client connection")
}

////////////////////////////////////////////////////////////////////////////////
// Behavior

func serverClientSpin(lc *manatcp.ListenerConn) {
	tick := time.Tick(10 * time.Second)
	for {
		select {
		case <-tick:
			log.Println("SERVER: Pushing HI to the client")
			lc.PushCh <- []byte("HI")
		case <-lc.CloseCh:
			return
		}
	}
}

func main() {
	_, err := manatcp.Listen(ExServer{}, ":9000")
	if err != nil {
		log.Fatal(err)
	}

	for {
		log.Println("CLIENT: connecting")
		conn, err := manatcp.Dial(ExClient{}, "localhost:9000")
		if err != nil {
			log.Fatal(err)
		}

	forloop:
		for {
			select {
			case <-time.After(5 * time.Second):
				log.Println("CLIENT: Sending command OHAI")
				res, err, dead := conn.Cmd([]byte("OHAI"))
				log.Printf("CLIENT: Got back: '%s', %v, %v", string(res.([]byte)), err, dead)

			case push, ok := <-conn.PushCh:
				if !ok {
					log.Println("CLIENT: PushCh closed")
					break forloop
				}
				log.Printf("CLIENT: Got push: '%s'", string(push.([]byte)))
			}
		}
		log.Println("CLIENT: closing")
		conn.Close()
	}
}
