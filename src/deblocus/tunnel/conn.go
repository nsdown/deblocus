package tunnel

import (
	"crypto/sha1"
	log "golang/glog"
	"hash"
	"io"
	"net"
	"strings"
	"time"
	//"time"
)

type Conn struct {
	net.Conn
	cipher     *Cipher
	rHash      hash.Hash
	wHash      hash.Hash
	identifier string
}

func NewConnWithHash(conn *net.TCPConn) *Conn {
	return &Conn{
		Conn:  conn,
		rHash: sha1.New(),
		wHash: sha1.New(),
	}
}

func NewConn(conn *net.TCPConn, cipher *Cipher) *Conn {
	return &Conn{
		Conn:   conn,
		cipher: cipher,
	}
}

func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if c.rHash != nil && n > 0 {
		c.rHash.Write(b[:n])
	}
	if n > 0 && c.cipher != nil {
		c.cipher.decrypt(b[:n], b[:n])
	}
	return n, err
}

func (c *Conn) Write(b []byte) (int, error) {
	if c.cipher != nil {
		c.cipher.encrypt(b, b)
	}
	if c.wHash != nil {
		c.wHash.Write(b)
	}
	return c.Conn.Write(b)
}

func (c *Conn) Close() error {
	return c.Conn.Close()
}

func (c *Conn) CloseRead() {
	if conn, ok := c.Conn.(*net.TCPConn); ok {
		conn.CloseRead()
	}
}

func (c *Conn) CloseWrite() {
	if conn, ok := c.Conn.(*net.TCPConn); ok {
		conn.CloseWrite()
	}
}

func (c *Conn) SetSockOpt(disableDeadline, keepAlive, noDelay int8) {
	if disableDeadline > 0 {
		c.Conn.SetDeadline(ZERO_TIME)
	}
	if t, y := c.Conn.(*net.TCPConn); y {
		// SetKeepAlivePeriod(d time.Duration) error
		if keepAlive >= 0 {
			t.SetKeepAlive(keepAlive > 0)
		}
		if noDelay >= 0 {
			t.SetNoDelay(noDelay > 0)
		}
	}
}

func (c *Conn) FreeHash() {
	c.rHash = nil
	c.wHash = nil
}

func (c *Conn) RHashSum() []byte {
	hash := c.rHash.Sum(nil)
	c.rHash.Reset()
	c.rHash = nil
	return hash
}

func (c *Conn) WHashSum() []byte {
	hash := c.wHash.Sum(nil)
	c.wHash.Reset()
	c.wHash = nil
	return hash
}

func GetConnIdentifier(con net.Conn) string {
	if c, y := con.(*Conn); y && c.identifier != NULL {
		return c.identifier
	}
	return ipAddr(con.RemoteAddr())
}

func Pipe(dst, src net.Conn, sid int32, ctl *CtlThread) {
	defer dst.Close()
	src.SetReadDeadline(ZERO_TIME)
	dst.SetWriteDeadline(ZERO_TIME)
	var (
		written  int64
		err      error
		buf      = make([]byte, 16*1024)
		nr, nw   int
		er, ew   error
		now      int64
		lastTime = time.Now().Unix()
	)
	for {
		nr, er = src.Read(buf)
		if nr > 0 {
			nw, ew = dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		// prevent calling too often, especially during high speed transmitting.
		if now = time.Now().Unix(); (now-lastTime) > 2 && (er == nil || er == io.EOF) {
			lastTime = now
			ctl.active(now)
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	if log.V(2) {
		sAddr := GetConnIdentifier(src)
		dAddr := GetConnIdentifier(dst)
		// use of closed...err may be normal error-obj that named `errClosing` at /src/net.go:284
		// OR may be net.OpError caused by syscall.
		// so we have to scan error string msg, where is better way ?
		if err == nil || strings.Contains(err.Error(), "closed") {
			log.Infof("SID#%X  %s --- %s --> %s\n", sid, sAddr, i64HumanSize(written), dAddr)
		} else {
			log.Infof("SID#%X  %s --- %s --> %s Error=%v\n", sid, sAddr, i64HumanSize(written), dAddr, err)
		}
	}
	return
}
