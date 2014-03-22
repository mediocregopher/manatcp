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
	// ListenerConn passed in is unique to this connection as well.
	Connected(*ListenerConn) ServerClient
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

	// Once a command is read off the connection, it is handled with this
	// method. Takes in the command read off the connection, returns a response
	// (which will be written with Write). Also returns whether or not the
	// connection should be closed (true if so). If the connection should be
	// closed and the response is NOT `nil` that response will attempt to be
	// written to the connection before closing.
	HandleCmd(interface{}) (interface{}, bool)

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
		lc.serverClient = l.server.Connected(&lc)
		go lc.spin()
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
	PushCh       chan interface{}
}

func (lc *ListenerConn) read(readCh chan *readWrap) {
	item, die := lc.serverClient.Read(lc.buf.Reader)
	readCh <- &readWrap{item, nil, die}
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
			res, die := lc.serverClient.HandleCmd(rw.item)
			if die {
				if res != nil {
					lc.write(res)
				}
				break spinloop
			}
			if die = lc.write(res); die {
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

}

// Force close the client connection. This method can be called by any number of
// go-routines safely.
func (lc *ListenerConn) Close() {
	close(lc.CloseCh)
}
