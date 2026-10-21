package wyoming

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

var ErrConnectionClosed = errors.New("service closed the connection")

// pastDeadline makes a blocked read or write return at once.
var pastDeadline = time.Unix(1, 0)

type Timeouts struct {
	Connect   time.Duration
	Operation time.Duration
}

// Conn is a connection to one Wyoming service. One goroutine can read while
// another writes. After an error the stream position is unknown, so close the Conn.
type Conn struct {
	conn     net.Conn
	r        *bufio.Reader
	addr     string
	timeouts Timeouts
}

func Dial(ctx context.Context, addr string, timeouts Timeouts) (*Conn, error) {
	d := net.Dialer{Timeout: timeouts.Connect}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	return &Conn{conn: conn, r: bufio.NewReader(conn), addr: addr, timeouts: timeouts}, nil
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) Write(ctx context.Context, ev Event) error {
	err := c.do(ctx, c.conn.SetWriteDeadline, func() error { return WriteEvent(c.conn, ev) })
	if err != nil {
		return fmt.Errorf("write %s to %s: %w", ev.Type, c.addr, err)
	}
	return nil
}

func (c *Conn) Read(ctx context.Context) (Event, error) {
	var ev Event
	err := c.do(ctx, c.conn.SetReadDeadline, func() error {
		var err error
		ev, err = ReadEvent(c.r)
		return err
	})
	if errors.Is(err, io.EOF) {
		err = ErrConnectionClosed
	}
	if err != nil {
		return Event{}, fmt.Errorf("read from %s: %w", c.addr, err)
	}
	return ev, nil
}

// do runs op with a deadline from the operation timeout and ctx, and makes op
// return when ctx ends.
func (c *Conn) do(ctx context.Context, setDeadline func(time.Time) error, op func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var deadline time.Time
	if c.timeouts.Operation > 0 {
		deadline = time.Now().Add(c.timeouts.Operation)
	}
	if d, ok := ctx.Deadline(); ok && (deadline.IsZero() || d.Before(deadline)) {
		deadline = d
	}
	if err := setDeadline(deadline); err != nil {
		return err
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = setDeadline(pastDeadline)
		close(interrupted)
	})
	err := op()
	if !stop() {
		<-interrupted
		if err != nil {
			return ctx.Err()
		}
	}
	return err
}

// Describe asks the service at addr which programs, models and voices it has.
func Describe(ctx context.Context, addr string, timeouts Timeouts) (Info, error) {
	c, err := Dial(ctx, addr, timeouts)
	if err != nil {
		return Info{}, err
	}
	defer c.Close()

	request, err := NewEvent(TypeDescribe, nil, nil)
	if err != nil {
		return Info{}, err
	}
	if err := c.Write(ctx, request); err != nil {
		return Info{}, err
	}
	ev, err := c.Read(ctx)
	if err != nil {
		return Info{}, err
	}
	if err := CheckType(ev, TypeInfo); err != nil {
		return Info{}, fmt.Errorf("describe %s: %w", addr, err)
	}
	var info Info
	return info, ev.Decode(&info)
}
