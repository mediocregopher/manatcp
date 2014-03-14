package main

import (
	"github.com/mediocregopher/manatcp"
	"bufio"
	"time"
	"fmt"
)

type RawTcp struct{}

func (_ RawTcp) Read(buf *bufio.Reader) (interface{}, error, bool) {
	l, err := buf.ReadBytes('\n')
	if err != nil {
		return nil, err, true
	}
	return l, nil, false
}

func (_ RawTcp) IsPush(li interface{}) bool {
	l := li.([]byte)
	return l[0] != '~'
}

func (_ RawTcp) Write(buf *bufio.Writer, item interface{}) (error, bool) {
	if _, err := buf.Write(item.([]byte)); err != nil {
		return err, true
	}
	if _, err := buf.Write([]byte{'\n'}); err != nil {
		return err, true
	}
	return nil, false
}

func main() {
	conn, err := manatcp.Dial(RawTcp{}, "tcp", "localhost:9000")
	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-time.After(5 * time.Second):
			fmt.Println(conn.Cmd([]byte("OHAI")))
		case push, ok := <-conn.PushCh:
			if !ok {
				fmt.Println("PushCh closed")
				return
			}
			fmt.Println(string(push.([]byte)))
		}
	}
}
