package proto

// Copyright (c) 2016 Charles Iliya Krempeaux <charles@reptile.ca> :: http://changelog.ca/

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
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
				log.Print("errPartialIACIACWrite")
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
		log.Print("errOverflow")
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

type ReadWriter struct {
	io.Reader
	io.Writer
}

func StartTelnetServer() {
	tcpListener, err := net.Listen("tcp", "0.0.0.0:5555")
	if err != nil {
		log.Fatalf("failed to listen on 5555 (%s)", err)
	}
	defer tcpListener.Close()

	log.Print("listening on 5555 port")

	if _, err := os.Stat("./telnet-log"); os.IsNotExist(err) {
		os.Mkdir("./telnet-log", 0766)
	}

	timeoutSec := 30

	log.Print("ssh timeout is ", timeoutSec, "s")

	for {

		telnetConn, err := tcpListener.Accept()
		if err != nil {
			log.Fatalf("failed to listen on 5555 (%s)", err)
		}
		defer telnetConn.Close()

		go func() {

			go func() {
				time.Sleep(time.Duration(timeoutSec) * time.Second)
				log.Println("timeout")
				err = telnetConn.Close()
				if err != nil {
					log.Println(err)
				}
			}()

			utcTime := time.Now().UTC().Format(time.RFC3339Nano)

			remoteIP := strings.Split(telnetConn.RemoteAddr().String(), ":")[0]

			if _, err := os.Stat(remoteIP); os.IsNotExist(err) {
				os.Mkdir("./telnet-log/"+remoteIP, 0766)
			}

			logFileName := "./telnet-log/" + remoteIP + "/" + utcTime + ".txt"

			logFile, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_CREATE, 0666)
			if err != nil {
				log.Fatal("failed open log file:", err)
			}
			log.Print("new ssh connection from " + telnetConn.RemoteAddr().String() + "\n")
			fmt.Fprint(logFile, "RemoteAddr:"+telnetConn.RemoteAddr().String()+"\n")
			fmt.Fprint(logFile, "Time:"+utcTime+"\n")

			telnetConn.Write([]byte{0xFF, 0xFD, 0x18, 0xFF, 0xFD, 0x20, 0xFF, 0xFD, 0x23, 0xFF, 0xFD, 0x27})
			// telnetConn.Write([]byte{0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x3a, 0x20})
			// telnetConn.Write([]byte{0x50, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x3a})
			// telnetConn.Write([]byte("\r\r\n"))

			r := newDataReader(telnetConn)
			w := newDataWriter(telnetConn)

			readBuff := []byte{0}

			commandBuff := []byte{}

			w.Write([]byte("login: "))

			lfCount := 0

			commandCount := 0

			for {

				_, err := r.Read(readBuff)
				if nil != err {
					log.Print(err)
					break
				}

				if readBuff[0] == 0x0a { // cr lf を lfの扱いに統一
					readBuff[0] = 0x0d
				}

				readOneByte := readBuff[0]

				if readOneByte == 0x0d {

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

					commandCount += 1

					if commandCount == 1 { // userID
						fmt.Println("your user is", command)
						fmt.Fprint(logFile, "User:"+command+"\n")

						w.Write([]byte("password: "))
					} else if commandCount == 2 { // password
						fmt.Println("your password is", command)
						fmt.Fprint(logFile, "Password:"+command+"\n")
						fmt.Fprint(logFile, "RequestTyped:Shell"+"\n-----\n")

						w.Write([]byte("> "))
					} else {
						// w.Write([]byte(command + "\r\n"))
						fmt.Println(">", command)
						w.Write([]byte(command + "\r\n"))

						fmt.Fprint(logFile, "$ "+string(command)+"\n")
						if command == "exit" {
							telnetConn.Close()
							break
						}
						w.Write([]byte("> "))
					}
				} else {
					commandBuff = append(commandBuff, readOneByte)
				}
			}
		}()
	}
}
