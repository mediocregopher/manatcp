package manatcp

import (
	"bufio"
	"net"
	"time"
)

// Interface which essentially acts as a set of callbacks defining the behavior
// of the manatcp client. The actual value the methods are being called on
// should NOT be modified, as these are not called in a single-threaded manner.
type Client interface {

	// Read a single command response or push message off of the the Reader, and
	// do whatever parsing on it needs to be done. Returns the parsed item,
	// potentially an error (either from the reading or parsing), and whether or
	// not the connection should be closed. If there was an error reading then
	// the third return value should be `true`
	Read(*bufio.Reader) (interface{}, error, bool)

	// Simply returns whether or not the given item is a push message (as
	// opposed to a command response)
	IsPush(interface{}) bool

	// Writes the given item to the given Writer. Returns an error either from
	// marshalling the item into whatever format, or from the writing itself. If
	// the error is from writing then the second return value should be `true`
	// to indicate the connection is closed.
	Write(*bufio.Writer, interface{}) (error, bool)
}

type readWrap struct {
	item interface{}
	err  error
	die  bool
}

// Conn handles the command/response sequence as well as putting push messages
// from the server into the PushCh. It is meant to be interacted with in a
// single-threaded manner (with the exception of PushCh, which can be read from
// in a separate go-routine).
type Conn struct {
	conn   net.Conn
	buf    *bufio.ReadWriter
	client Client
	readCh chan *readWrap

	// Channel onto-which all push messages are put. This must be read from at
	// all times or execution inside Conn will be blocked. This channel is
	// closed when either the client or server terminate the connection.
	PushCh chan interface{}
}

// Connects to a server over tcp and initializes a Conn if successful.
func Dial(c Client, address string) (*Conn, error) {
	tconn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	rbuf := bufio.NewReader(tconn)
	wbuf := bufio.NewWriter(tconn)
	conn := Conn{
		conn:   tconn,
		buf:    bufio.NewReadWriter(rbuf, wbuf),
		client: c,
		readCh: make(chan *readWrap),
		PushCh: make(chan interface{}),
	}
	go conn.spin()
	return &conn, nil
}

func (conn *Conn) spin() {
	for {
		item, err, die := conn.client.Read(conn.buf.Reader)
		if err != nil || !conn.client.IsPush(item) {
			select {
			case conn.readCh <- &readWrap{item, err, die}:
			case <-time.After(2 * time.Second):
			}
			if die {
				break
			}
		} else {
			conn.PushCh <- item
		}
	}
	conn.conn.Close()
	close(conn.PushCh)
}

func (conn *Conn) cmd(cmd interface{}, bg bool) (interface{}, error, bool) {
	err, die := conn.client.Write(conn.buf.Writer, cmd)
	if err != nil {
		return nil, err, die
	} else if bg {
		return nil, nil, false
	}

	if err = conn.buf.Writer.Flush(); err != nil {
		return nil, err, true
	}

	select {
	case rw := <-conn.readCh:
		return rw.item, rw.err, rw.die
	}
}

// Sends a cmd to the connection (to be processed by Write) and waits for a
// response (processed by Read). Returns the response, an error (either from
// reading/writing or marshalling/unmarshalling the command/response), and
// whether or not the connection has been closed.
func (conn *Conn) Cmd(cmd interface{}) (interface{}, error, bool) {
	return conn.cmd(cmd, false)
}

// Similar to Cmd, but doesn't wait for a response. This will still block to
// find out if there are any errors when reading/writing/marshalling.
func (conn *Conn) CmdBg(cmd interface{}) (error, bool) {
	_, err, die := conn.cmd(cmd, true)
	return err, die
}

// Closes the backing tcp connection, causing Conn to cleanup its resources.
func (conn *Conn) Close() error {
	// This will cause Read to fail and cleanup on its own.
	return conn.conn.Close()
}
