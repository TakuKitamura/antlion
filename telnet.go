package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"

	"golang.org/x/term"
)

type Context interface {
}

type Writer interface {
	Write([]byte) (int, error)
}

type Reader interface {
	Read([]byte) (int, error)
}

func LongWrite(w io.Writer, p []byte) (int64, error) {

	numWritten := int64(0)
	for {
		//@TODO: Should check to make sure this doesn't get stuck in an infinite loop writting nothing!
		n, err := w.Write(p)
		numWritten += int64(n)
		if nil != err && io.ErrShortWrite != err {
			return numWritten, err
		}

		if !(n < len(p)) {
			break
		}

		p = p[n:]

		if len(p) < 1 {
			break
		}
	}

	return numWritten, nil
}

// func ServeTELNET(term *term.Terminal) {

// 	for {
// 		// fmt.Println(999)
// 		line, _ := term.ReadLine()
// 		fmt.Println(line)
// 		// if err == io.EOF {
// 		// 	fmt.Print("read eof", "\n")
// 		// 	// return nil
// 		// }
// 		// if err != nil {
// 		// 	fmt.Print("read line failed:", err.Error()+"\n")
// 		// 	// return err
// 		// }
// 		// if line == "" {
// 		// 	fmt.Fprint(term, line, 123)
// 		// 	// fmt.Fprint(logFile, lineLabel+line+"\n")
// 		// 	continue
// 		// }
// 	}

// var buffer [1]byte // Seems like the length of the buffer needs to be small, otherwise will have to wait for buffer to fill up.
// p := buffer[:]

// for {
// 	n, err := r.Read(p)

// 	// fmt.Println(string(p), n)

// 	if n > 0 {
// 		fmt.Println(p, n)
// 		LongWrite(term, p[:n])
// 	}

// 	if nil != err {
// 		break
// 	}
// }
// }

type internalDataWriter struct {
	wrapped io.Writer
}

func newDataWriter(w io.Writer) *internalDataWriter {
	writer := internalDataWriter{
		wrapped: w,
	}

	return &writer
}

type internalDataReader struct {
	wrapped  io.Reader
	buffered *bufio.Reader
}

var iaciac []byte = []byte{255, 255}

func (w *internalDataWriter) write64(data []byte) (n int64, err error) {

	if len(data) <= 0 {
		return 0, nil
	}

	const IAC = 255

	var buffer bytes.Buffer
	for _, datum := range data {

		if IAC == datum {

			if buffer.Len() > 0 {
				var numWritten int64

				numWritten, err = LongWrite(w.wrapped, buffer.Bytes())
				n += numWritten
				if nil != err {
					return n, err
				}
				buffer.Reset()
			}

			var numWritten int64
			//@TODO: Should we worry about "iaciac" potentially being modified by the .Write()?
			numWritten, err = LongWrite(w.wrapped, iaciac)
			if int64(len(iaciac)) != numWritten {
				//@TODO: Do we really want to panic() here?
				panic("errPartialIACIACWrite")
			}
			n += 1
			if nil != err {
				return n, err
			}
		} else {
			buffer.WriteByte(datum) // The returned error is always nil, so we ignore it.
		}
	}

	if buffer.Len() > 0 {
		var numWritten int64
		numWritten, err = LongWrite(w.wrapped, buffer.Bytes())
		n += numWritten
	}

	return n, err
}

func (w *internalDataWriter) Write(data []byte) (n int, err error) {
	var n64 int64

	n64, err = w.write64(data)
	n = int(n64)
	if int64(n) != n64 {
		panic("errOverflow")
	}

	return n, err
}

func newDataReader(r io.Reader) *internalDataReader {
	buffered := bufio.NewReader(r)

	reader := internalDataReader{
		wrapped:  r,
		buffered: buffered,
	}

	return &reader
}

func (r *internalDataReader) Read(data []byte) (n int, err error) {

	const IAC = 255

	const SB = 250
	const SE = 240

	const WILL = 251
	const WONT = 252
	const DO = 253
	const DONT = 254

	p := data

	for len(p) > 0 {
		var b byte

		b, err = r.buffered.ReadByte()
		if nil != err {
			return n, err
		}

		if IAC == b {
			var peeked []byte

			peeked, err = r.buffered.Peek(1)
			if nil != err {
				return n, err
			}

			switch peeked[0] {
			case WILL, WONT, DO, DONT:
				_, err = r.buffered.Discard(2)
				if nil != err {
					return n, err
				}
			case IAC:
				p[0] = IAC
				n++
				p = p[1:]

				_, err = r.buffered.Discard(1)
				if nil != err {
					return n, err
				}
			case SB:
				for {
					var b2 byte
					b2, err = r.buffered.ReadByte()
					if nil != err {
						return n, err
					}

					if IAC == b2 {
						peeked, err = r.buffered.Peek(1)
						if nil != err {
							return n, err
						}

						if IAC == peeked[0] {
							_, err = r.buffered.Discard(1)
							if nil != err {
								return n, err
							}
						}

						if SE == peeked[0] {
							_, err = r.buffered.Discard(1)
							if nil != err {
								return n, err
							}
							break
						}
					}
				}
			case SE:
				_, err = r.buffered.Discard(1)
				if nil != err {
					return n, err
				}
			default:
				// If we get in here, this is not following the TELNET protocol.
				//@TODO: Make a better error.
				err = nil
				return n, err
			}
		} else {

			p[0] = b
			n++
			p = p[1:]
		}
	}

	return n, nil
}

func ServeTELNET(term *term.Terminal) {

	for {
		// fmt.Println(999)
		x, err := term.ReadLine()
		// term.Write([]byte("ok!\n"))
		fmt.Println(x, err)
		// term.Write([]byte(line + "XXX"))

	}
}

func handle(c net.Conn) {
	defer c.Close()

	// var w Writer = newDataWriter(c)
	// var r Reader = newDataReader(c)

	rw := bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c))

	// var x = abc{}

	term := term.NewTerminal(rw, "")

	// lineLabel := "> "

	// term.SetPrompt(lineLabel + string(term.Escape.Reset))

	// fmt.Fprint(term, "Hello!\n")

	ServeTELNET(term)
	c.Close()
}

type ReadWriter struct {
	io.Reader
	io.Writer
}

// NewReadWriter allocates a new ReadWriter that dispatches to r and w.
func NewReadWriter(r io.Reader, w io.Writer) *ReadWriter {
	return &ReadWriter{r, w}
}

type terminal struct {
	write  *bufio.Writer
	reader *bufio.Reader
}

type RR interface {
	Read(p []byte) (n int, err error)
}

// type internalDataWriter struct {
// 	wrapped io.Writer
// }

// func newDataWriter(w io.Writer) *internalDataWriter {
// 	writer := internalDataWriter{
// 		wrapped: w,
// 	}

// 	return &writer
// }

func main() {
	tcpListener, err := net.Listen("tcp", "0.0.0.0:23")
	if err != nil {
		log.Fatalf("failed to listen on 23 (%s)", err)
	}
	defer tcpListener.Close()

	conn, _ := tcpListener.Accept()
	defer conn.Close()

	conn.Write([]byte{0xFF, 0xFD, 0x18, 0xFF, 0xFD, 0x20, 0xFF, 0xFD, 0x23, 0xFF, 0xFD, 0x27})
	// conn.Write([]byte{0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x3a, 0x20})
	// conn.Write([]byte{0x50, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x3a})
	// conn.Write([]byte("\r\n"))

	r := newDataReader(conn)
	w := newDataWriter(conn)

	readBuff := []byte{0}

	commandBuff := []byte{}

	w.Write([]byte("login: "))
	w.Write([]byte("\n"))
	w.Write([]byte("password: "))
	w.Write([]byte("\n"))
	w.Write([]byte("> "))

	lfCount := 0

	for {

		_, err := r.Read(readBuff)

		if readBuff[0] == 0x0a { // cr lf を lfの扱いに統一
			readBuff[0] = 0x0d
		}

		readOneByte := readBuff[0]

		if readOneByte == 0x0d {

			fmt.Println(commandBuff, readBuff)

			if len(commandBuff) == 0 { // lf の処理
				lfCount += 1
				if lfCount == 2 {
					w.Write([]byte("> "))
					lfCount = 0
				}

				continue
			}

			lfCount = 0

			command := string(commandBuff)
			commandBuff = []byte{}

			w.Write([]byte(command + "\n"))
			// w.Write([]byte("your command is " + command + "\n"))
			if command == "exit" {
				// break
			}
			w.Write([]byte("> "))
		} else {
			commandBuff = append(commandBuff, readOneByte)
		}

		if nil != err {
			break
		}
	}

}
