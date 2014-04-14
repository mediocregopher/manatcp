package manatcp

import (
	"bufio"
	"net"
)

// Interface representing a tcp server which can accept new client connections.
// Unlike Client, Server is interacted with in a single-threaded manner, so
// state can be preserved if required
type Server interface {

	// Called when a client initiates a connection. Returns an initialized
	// instance of a ServerClient which is unique to this new connection. The
	// ListenerConn passed in is unique to this connection as well. Also returns
	// a boolean indicating whether the client should be closed immediately
	// (true if so)
	Connected(*ListenerConn) (ServerClient, bool)
}

// A ServerClient is an interface over a single client connection to a Server.
// It is primarily run in a single-threaded manner (caveat: the Read method), so
// it may hold and modify its own state. The ServerClient's methods are meant to
// be called within manatcp and should not be called outside of it.
type ServerClient interface {

	// Read a single command off of the connection, and do whatever parsing
	// which needs to be done. Returns the parsed item, and whether or not the
	// connection should be closed (true if so). NOTE: This method is executed
	// in a separate go-routine from the other methods on the ServerClient, so
	// be careful if it needs to modify state.
	Read(*bufio.Reader) (interface{}, bool)

	// Writes a command response or push message to the connection, returning
	// whether or not the connection should be closed (true if so).
	Write(*bufio.Writer, interface{}) bool

	// Once a command is read off the connection it is handled with this method.
	// The return interface{} is what should be sent back to the client. The
	// First boolean return indicates whether or not to actually send something
	// (true to send the interface{} back). The final boolean return indicates
	// whether or not to close the connection (true to close). If both the
	// boolean returns are true it will attempt to send back the given
	// interface{} before closing the connection
	HandleCmd(interface{}) (interface{}, bool, bool)

	// Called when the connection is closed for any reason, either by the client
	// or server. This is called just before Close is called on the net.Conn
	// stored internally.
	Closing()
}

// A tcp server which can accept new client connections, and handle them through
// a given Server interface.
type Listener struct {
	listen net.Listener
	server Server
	ErrCh  chan error
}

// Begins listening for client connections on the given address, using the given
// Server interface.
func Listen(s Server, laddr string) (*Listener, error) {
	ln, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}

	l := Listener{
		listen: ln,
		server: s,
		ErrCh:  make(chan error),
	}
	go l.spin()
	return &l, nil
}

func (l *Listener) err(err error) {
	select {
	case l.ErrCh <- err:
	default:
	}
}

func (l *Listener) spin() {
	for {
		conn, err := l.listen.Accept()
		if err != nil {
			l.err(err)
			continue
		}
		rbuf := bufio.NewReader(conn)
		wbuf := bufio.NewWriter(conn)
		lc := ListenerConn{
			conn:    conn,
			buf:     bufio.NewReadWriter(rbuf, wbuf),
			CloseCh: make(chan struct{}),
			PushCh:  make(chan interface{}),
		}
		var die bool
		lc.serverClient, die = l.server.Connected(&lc)
		if !die {
			go lc.spin()
		} else {
			conn.Close()
		}
	}
}

// A wrapper around the net.Conn for a client connection.
type ListenerConn struct {
	conn         net.Conn
	buf          *bufio.ReadWriter
	serverClient ServerClient
	CloseCh      chan struct{}

	// Anything pushed to this channel will subsequently be pushed to the
	// client. This channel can be written to by any number of go-routines
	// safely, up until Closing is called on the associated ServerClient. As
	// soon as that method returns pushing to this channel will cause a panic.
	PushCh chan interface{}
}

func (lc *ListenerConn) read(readCh chan *readWrap) {
	item, die := lc.serverClient.Read(lc.buf.Reader)
	select {
	case readCh <- &readWrap{item, nil, die}:
	case <-lc.CloseCh:
	}
}

func (lc *ListenerConn) groundPushCh(done chan struct{}) {
	for {
		select {
		case <-lc.PushCh:
		case <-done:
			return
		}
	}
}

func (lc *ListenerConn) write(item interface{}) bool {
	if die := lc.serverClient.Write(lc.buf.Writer, item); die {
		return die
	}
	if err := lc.buf.Writer.Flush(); err != nil {
		return true
	}
	return false
}

func (lc *ListenerConn) spin() {
	readCh := make(chan *readWrap)
	needsRead := true

spinloop:
	for {
		if needsRead {
			go lc.read(readCh)
			needsRead = false
		}

		select {
		case rw := <-readCh:
			needsRead = true
			if rw.die {
				break spinloop
			}
			var dieCmd, dieWrite bool
			res, sendback, dieCmd := lc.serverClient.HandleCmd(rw.item)
			if sendback {
				dieWrite = lc.write(res)
			}
			if dieCmd || dieWrite {
				break spinloop
			}

		case item := <-lc.PushCh:
			if die := lc.write(item); die {
				break spinloop
			}

		case <-lc.CloseCh:
			break spinloop
		}
	}

	doneClosing := make(chan struct{})
	go lc.groundPushCh(doneClosing)
	lc.serverClient.Closing()
	lc.conn.Close()
	close(doneClosing)
	close(lc.PushCh)
	close(lc.CloseCh)
	lc.serverClient = nil

}

// Force close the client connection. This method can be called by any number of
// go-routines safely.
func (lc *ListenerConn) Close() {
	close(lc.CloseCh)
}
